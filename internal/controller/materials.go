package controller

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/liyiligang/loom/internal/materials"
	"github.com/liyiligang/loom/internal/workspace"
)

type MaterialInput struct {
	Kind         string `json:"kind"`
	Name         string `json:"name"`
	Content      string `json:"content"`
	ExpectedHash string `json:"expectedHash"`
	ScopeKey     string `json:"scopeKey"`
}
type MaterialLinks struct {
	ScopeKey   string   `json:"scopeKey"`
	RelatedIDs []string `json:"relatedIds"`
}

// Holding a scope blocks requirement branch replacement while filesystem IO is in progress.
func (s *Service) materialScope(ctx context.Context, id string, write bool) (materials.Scope, func(), error) {
	value, _ := s.materialLocks.LoadOrStore(id, make(chan struct{}, 1))
	gate := value.(chan struct{})
	select {
	case gate <- struct{}{}:
	case <-ctx.Done():
		return materials.Scope{}, nil, ctx.Err()
	}
	releaseGate := func() { <-gate }
	s.mu.Lock()
	requirement := s.requirement(id)
	if requirement == nil {
		s.mu.Unlock()
		releaseGate()
		return materials.Scope{}, nil, problem("requirement_not_found", "Requirement not found.", 404)
	}
	project := s.project(requirement.ProjectID)
	if !project.Local {
		s.mu.Unlock()
		releaseGate()
		return materials.Scope{}, nil, problem("materials_remote", "Requirement files are currently available for local projects.", 409)
	}
	if !s.data.LocalSetup.Confirmed {
		s.mu.Unlock()
		releaseGate()
		return materials.Scope{}, nil, problem("local_unconfirmed", "Confirm local agents before browsing project directories.", 409)
	}
	directory, projectID := project.Cwd, project.ID
	var binding *workspace.Binding
	if source := requirement.MaterialWorkspace(); source != nil {
		if source.State != "ready" {
			s.mu.Unlock()
			releaseGate()
			return materials.Scope{}, nil, problem("workspace_not_ready", "Prepare the requirement worktree before accessing its files.", 409)
		}
		copy := *source
		binding = &copy
		directory = copy.Directory
	}
	if write {
		if err := s.directoryWriterAvailableLocked(directory, ""); err != nil {
			s.mu.Unlock()
			releaseGate()
			return materials.Scope{}, nil, err
		}
		s.materialWrites[projectDirectory(directory, true)]++
	}
	s.creatingSessions[id]++
	s.mu.Unlock()
	release := func() {
		s.mu.Lock()
		s.creatingSessions[id]--
		if write {
			s.materialWrites[projectDirectory(directory, true)]--
		}
		s.mu.Unlock()
		releaseGate()
	}
	if binding != nil {
		if err := workspace.Verify(ctx, *binding); err != nil {
			release()
			return materials.Scope{}, nil, problem("workspace_unavailable", err.Error(), 409)
		}
	}
	branch, commit, err := workspace.Version(ctx, directory)
	if err != nil {
		release()
		return materials.Scope{}, nil, problem("materials_unavailable", materials.ErrorMessage(err), 409)
	}
	return materials.NewScope(projectID, id, projectDirectory(directory, true), branch, commit), release, nil
}
func (s *Service) scanMaterials(ctx context.Context, scope materials.Scope) (materials.Catalog, error) {
	catalog, err := materials.Scan(ctx, scope)
	if err != nil {
		return catalog, problem("materials_unavailable", materials.ErrorMessage(err), 409)
	}
	if err = s.store.SyncMaterials(&catalog); err != nil {
		return catalog, problem("materials_index", "Could not update the requirement file index. Files remain on disk; refresh to retry.", 500)
	}
	return catalog, nil
}
func (s *Service) RequirementMaterials(ctx context.Context, id string) (materials.Catalog, error) {
	scope, release, err := s.materialScope(ctx, id, false)
	if err != nil {
		return materials.Catalog{}, err
	}
	defer release()
	return s.scanMaterials(ctx, scope)
}
func (s *Service) RequirementMaterial(ctx context.Context, id, fileID string) (materials.Detail, error) {
	scope, release, err := s.materialScope(ctx, id, false)
	if err != nil {
		return materials.Detail{}, err
	}
	defer release()
	catalog, err := s.scanMaterials(ctx, scope)
	if err != nil {
		return materials.Detail{}, err
	}
	file, err := materials.Find(catalog, fileID)
	if err != nil {
		return materials.Detail{}, problem("material_not_found", "Requirement file not found in the current branch.", 404)
	}
	result, err := materials.Read(scope, file)
	if err != nil {
		return result, problem("materials_unavailable", materials.ErrorMessage(err), 409)
	}
	return result, nil
}
func (s *Service) SaveRequirementMaterial(ctx context.Context, id, fileID string, input MaterialInput) (materials.Detail, error) {
	if err := materials.Validate(input.Kind, input.Name, input.Content); err != nil {
		return materials.Detail{}, problem("invalid_material", err.Error(), 400)
	}
	scope, release, err := s.materialScope(ctx, id, true)
	if err != nil {
		return materials.Detail{}, err
	}
	defer release()
	if input.ScopeKey == "" || input.ScopeKey != scope.Key {
		return materials.Detail{}, problem("material_conflict", "The requirement branch changed. Refresh before saving.", 409)
	}
	if fileID != "" {
		catalog, err := s.scanMaterials(ctx, scope)
		if err != nil {
			return materials.Detail{}, err
		}
		file, err := materials.Find(catalog, fileID)
		if err != nil {
			return materials.Detail{}, problem("material_not_found", "Requirement file not found in the current branch.", 404)
		}
		if file.Kind != input.Kind || filepath.Base(file.Path) != input.Name || input.ExpectedHash == "" {
			return materials.Detail{}, problem("invalid_material", "Preserve the filename and provide the current content hash.", 400)
		}
	} else if input.ExpectedHash != "" {
		return materials.Detail{}, problem("invalid_material", "New files must not supply a previous hash.", 400)
	}
	newID, err := materials.Write(scope, input.Kind, input.Name, input.Content, input.ExpectedHash)
	if err != nil {
		if errors.Is(err, materials.ErrConflict) {
			return materials.Detail{}, problem("material_conflict", err.Error(), 409)
		}
		return materials.Detail{}, problem("materials_unavailable", materials.ErrorMessage(err), 409)
	}
	catalog, err := s.scanMaterials(ctx, scope)
	if err != nil {
		return materials.Detail{}, err
	}
	file, err := materials.Find(catalog, newID)
	if err != nil {
		return materials.Detail{}, problem("materials_index", "The file was saved. Refresh its index before continuing.", 500)
	}
	s.mu.Lock()
	err = s.commitLocked("materials.updated", id)
	s.mu.Unlock()
	if err != nil {
		return materials.Detail{}, err
	}
	return materials.Read(scope, file)
}

// Preview uses the exact saved scope and content version selected by the browser.
func (s *Service) PreviewRequirementMaterial(ctx context.Context, id, fileID, scopeKey, hash string) (materials.Detail, error) {
	detail, err := s.RequirementMaterial(ctx, id, fileID)
	if err != nil {
		return materials.Detail{}, err
	}
	if detail.Kind != "prototypes" {
		return materials.Detail{}, problem("invalid_material", "Only HTML prototypes support interactive preview.", 400)
	}
	if scopeKey == "" || scopeKey != detail.Scope.Key || hash == "" || hash != detail.Hash {
		return materials.Detail{}, problem("material_conflict", "The prototype changed. Refresh before previewing.", 409)
	}
	if detail.Truncated || !detail.Editable {
		return materials.Detail{}, problem("invalid_material", "Interactive prototypes must contain UTF-8 HTML of at most 1 MiB.", 400)
	}
	return detail, nil
}
func (s *Service) LinkRequirementMaterials(ctx context.Context, id, fileID string, input MaterialLinks) error {
	if len(input.RelatedIDs) > 128 {
		return problem("invalid_material", "Choose at most 128 related files.", 400)
	}
	scope, release, err := s.materialScope(ctx, id, false)
	if err != nil {
		return err
	}
	defer release()
	if input.ScopeKey != scope.Key {
		return problem("material_conflict", "The requirement branch changed. Refresh before saving.", 409)
	}
	catalog, err := s.scanMaterials(ctx, scope)
	if err != nil {
		return err
	}
	if _, err = materials.Find(catalog, fileID); err != nil {
		return problem("material_not_found", "Requirement file not found in the current branch.", 404)
	}
	seen := make(map[string]bool)
	for _, target := range input.RelatedIDs {
		if target == fileID || seen[target] {
			return problem("invalid_material", "Related files must be unique and distinct from the source.", 400)
		}
		if _, err := materials.Find(catalog, target); err != nil {
			return problem("material_not_found", "Related file not found in the current branch.", 404)
		}
		seen[target] = true
	}
	if err := s.store.SetMaterialLinks(scope, fileID, input.RelatedIDs); err != nil {
		return problem("materials_index", "Could not save material relationships.", 500)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.commitLocked("materials.updated", id)
}
func (s *Service) OpenRequirementMaterial(ctx context.Context, id, fileID string) (*os.File, string, error) {
	scope, release, err := s.materialScope(ctx, id, false)
	if err != nil {
		return nil, "", err
	}
	defer release()
	catalog, err := s.scanMaterials(ctx, scope)
	if err != nil {
		return nil, "", err
	}
	file, err := materials.Find(catalog, fileID)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, "", problem("material_not_found", "Requirement file not found in the current branch.", 404)
	}
	if err != nil {
		return nil, "", err
	}
	stream, err := materials.Open(scope, file)
	if err != nil {
		return nil, "", problem("materials_unavailable", materials.ErrorMessage(err), 409)
	}
	return stream, filepath.Base(file.Path), nil
}

func materialContext(catalog materials.Catalog) map[string]any {
	return map[string]any{"directory": catalog.Scope.Directory, "root": catalog.Scope.Root, "branch": catalog.Scope.Branch, "commit": catalog.Scope.Commit, "files": catalog.Files, "folders": materials.Folders, "limited": catalog.Limited}
}
