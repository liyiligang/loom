package controller

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"github.com/liyiligang/loom/internal/skills"
	"github.com/liyiligang/loom/internal/workspace"
)

type ProjectSkillCatalog struct {
	skills.Catalog
	ProjectID string `json:"projectId"`
	Directory string `json:"directory"`
	ScopeKey  string `json:"scopeKey"`
}

func (s *Service) skillDirectory(id, sessionID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	project := s.project(id)
	if project == nil {
		return "", problem("project_not_found", "Project not found.", 404)
	}
	if !project.Local {
		return "", problem("remote_skills", "Skill browsing is available for local projects.", 409)
	}
	if !s.data.LocalSetup.Confirmed {
		return "", problem("local_unconfirmed", "Confirm local agents before browsing project directories.", 409)
	}
	if sessionID != "" {
		session := s.session(sessionID)
		if session == nil {
			return "", problem("not_found", "Session not found.", 404)
		}
		requirement := s.requirement(session.RequirementID)
		endpoint := s.endpoint(session.EndpointID)
		if requirement == nil || requirement.ProjectID != id || endpoint == nil || !endpoint.Local {
			return "", problem("skills_scope", "This session does not belong to the selected local project.", 400)
		}
		return session.Cwd, nil
	}
	return project.Cwd, nil
}

func (s *Service) ProjectSkills(ctx context.Context, id, sessionID string) (ProjectSkillCatalog, error) {
	directory, err := s.skillDirectory(id, sessionID)
	if err != nil {
		return ProjectSkillCatalog{}, err
	}
	catalog, err := skills.List(ctx, directory)
	if err != nil {
		return ProjectSkillCatalog{}, problem("local_directory", "The project directory is unavailable.", 400)
	}
	key, err := skillScopeKey(ctx, directory)
	if err != nil {
		return ProjectSkillCatalog{}, err
	}
	return ProjectSkillCatalog{Catalog: catalog, ProjectID: id, Directory: directory, ScopeKey: key}, nil
}

func (s *Service) ProjectSkill(ctx context.Context, id, skillID, sessionID string) (skills.Detail, error) {
	directory, err := s.skillDirectory(id, sessionID)
	if err != nil {
		return skills.Detail{}, err
	}
	detail, err := skills.Read(ctx, directory, skillID)
	switch {
	case err == nil:
		detail.ScopeKey, err = skillScopeKey(ctx, directory)
		if err != nil {
			return skills.Detail{}, err
		}
		return detail, nil
	case errors.Is(err, skills.ErrNotFound):
		return skills.Detail{}, problem("skill_not_found", err.Error(), 404)
	case errors.Is(err, skills.ErrTooLarge), errors.Is(err, skills.ErrInvalidText), errors.Is(err, skills.ErrUnavailable):
		return skills.Detail{}, problem("skill_unavailable", err.Error(), 400)
	default:
		return skills.Detail{}, problem("local_directory", "The project directory is unavailable.", 400)
	}
}

type SkillInput struct {
	Name         string `json:"name"`
	Content      string `json:"content"`
	ExpectedHash string `json:"expectedHash"`
	ScopeKey     string `json:"scopeKey"`
}

type SkillDeleteInput struct {
	ExpectedRevision string `json:"expectedRevision"`
	ScopeKey         string `json:"scopeKey"`
	Confirmed        bool   `json:"confirmed"`
}

func skillScopeKey(ctx context.Context, directory string) (string, error) {
	branch, commit, err := workspace.Version(ctx, directory)
	if err != nil {
		return "", problem("skills_unavailable", "Could not identify the project branch. Refresh before modifying skills.", 409)
	}
	if branch != "" {
		commit = ""
	}
	sum := sha256.Sum256([]byte(projectDirectory(directory, true) + "\x00" + branch + "\x00" + commit))
	return hex.EncodeToString(sum[:]), nil
}

func (s *Service) skillMutationScope(ctx context.Context, id, sessionID, expectedScope string) (string, func(), error) {
	directory, err := s.skillDirectory(id, sessionID)
	if err != nil {
		return "", nil, err
	}
	directory = projectDirectory(directory, true)
	value, _ := s.skillLocks.LoadOrStore(directory, make(chan struct{}, 1))
	gate := value.(chan struct{})
	select {
	case gate <- struct{}{}:
	case <-ctx.Done():
		return "", nil, ctx.Err()
	}
	releaseGate := func() { <-gate }
	current, err := s.skillDirectory(id, sessionID)
	if err != nil {
		releaseGate()
		return "", nil, err
	}
	if projectDirectory(current, true) != directory {
		releaseGate()
		return "", nil, problem("skills_scope", "The selected skill directory changed. Refresh before modifying skills.", 409)
	}
	s.mu.Lock()
	if err := s.directoryWriterAvailableLocked(directory, ""); err != nil {
		s.mu.Unlock()
		releaseGate()
		return "", nil, err
	}
	var binding *workspace.Binding
	if sessionID != "" {
		binding, err = s.sessionWorkspaceLocked(s.session(sessionID))
		if err != nil {
			s.mu.Unlock()
			releaseGate()
			return "", nil, err
		}
	}
	s.materialWrites[directory]++
	s.mu.Unlock()
	release := func() { s.mu.Lock(); s.materialWrites[directory]--; s.mu.Unlock(); releaseGate() }
	if binding != nil {
		if err := workspace.Verify(ctx, *binding); err != nil {
			release()
			return "", nil, problem("workspace_unavailable", err.Error(), 409)
		}
	}
	key, err := skillScopeKey(ctx, directory)
	if err != nil {
		release()
		return "", nil, err
	}
	if expectedScope == "" || key != expectedScope {
		release()
		return "", nil, problem("skill_conflict", "The project branch changed. Refresh before modifying skills.", 409)
	}
	return directory, release, nil
}

func skillMutationError(err error) error {
	switch {
	case errors.Is(err, skills.ErrConflict):
		return problem("skill_conflict", err.Error(), 409)
	case errors.Is(err, skills.ErrReadOnly):
		return problem("skill_read_only", err.Error(), 409)
	case errors.Is(err, skills.ErrInvalidDisplayName):
		return problem("invalid_skill_name", err.Error(), 400)
	case errors.Is(err, skills.ErrInvalidName), errors.Is(err, skills.ErrInvalidSource):
		return problem("invalid_skill", err.Error(), 400)
	case errors.Is(err, skills.ErrNotFound):
		return problem("skill_not_found", err.Error(), 404)
	default:
		return problem("skills_unavailable", "Could not modify the project skill. Refresh and retry.", 409)
	}
}

func (s *Service) SaveProjectSkill(ctx context.Context, id, skillID, sessionID string, input SkillInput) (skills.Detail, error) {
	if skillID == "" && input.ExpectedHash != "" {
		return skills.Detail{}, problem("invalid_skill", "Provide a source hash only when editing a skill.", 400)
	}
	directory, release, err := s.skillMutationScope(ctx, id, sessionID, input.ScopeKey)
	if err != nil {
		return skills.Detail{}, err
	}
	defer release()
	var result skills.Detail
	if skillID == "" {
		result, err = skills.CreateNamed(ctx, directory, input.Name, input.Content)
	} else if input.Name != "" {
		result, err = skills.UpdateNamed(ctx, directory, skillID, input.Name, input.Content, input.ExpectedHash)
	} else {
		result, err = skills.Update(ctx, directory, skillID, input.Content, input.ExpectedHash)
	}
	if err != nil {
		return skills.Detail{}, skillMutationError(err)
	}
	result.ScopeKey = input.ScopeKey
	s.mu.Lock()
	err = s.commitLocked("skills.updated", id)
	s.mu.Unlock()
	return result, err
}

func (s *Service) DeleteProjectSkill(ctx context.Context, id, skillID, sessionID string, input SkillDeleteInput) error {
	if !input.Confirmed {
		return problem("skill_confirmation", "Confirm the skill deletion before continuing.", 400)
	}
	directory, release, err := s.skillMutationScope(ctx, id, sessionID, input.ScopeKey)
	if err != nil {
		return err
	}
	defer release()
	if err := skills.Delete(ctx, directory, skillID, input.ExpectedRevision); err != nil {
		return skillMutationError(err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.commitLocked("skills.updated", id)
}
