package controller

import (
	"context"
	"strings"

	"github.com/liyiligang/loom/internal/model"
	"github.com/liyiligang/loom/internal/workspace"
)

func (s *Service) ProjectGit(ctx context.Context, id string) (workspace.Repository, error) {
	s.mu.Lock()
	project := s.project(id)
	if project == nil {
		s.mu.Unlock()
		return workspace.Repository{}, problem("project_not_found", "Project not found.", 404)
	}
	if !project.Local {
		s.mu.Unlock()
		return workspace.Repository{}, problem("workspace_remote", "Automatic worktrees are available for local projects.", 409)
	}
	if !s.data.LocalSetup.Confirmed {
		s.mu.Unlock()
		return workspace.Repository{}, problem("local_unconfirmed", "Confirm local agents before browsing project directories.", 409)
	}
	directory := project.Cwd
	s.mu.Unlock()
	repository, err := workspace.Inspect(ctx, directory)
	if err != nil {
		return workspace.Repository{}, problem("workspace_repository", err.Error(), 400)
	}
	return repository, nil
}

func (s *Service) reserveWorkspaceLocked(requirementID string, binding *workspace.Binding) error {
	for _, requirement := range s.data.Requirements {
		if requirement.ID == requirementID {
			continue
		}
		if group := requirement.BranchGroup; group != nil && group.CommonDir == binding.CommonDir && strings.HasPrefix(binding.Branch, group.Prefix+"/") {
			return problem("branch_group_owned", "This branch group prefix overlaps another requirement's group.", 409)
		}
		for _, other := range requirement.WorkspaceBindings() {
			if other.CommonDir == binding.CommonDir && other.Branch == binding.Branch {
				return problem("workspace_branch_owned", "This branch is already bound to another requirement.", 409)
			}
		}
	}
	if s.creatingSessions[requirementID] > 0 || s.creatingSessions[""] > 0 {
		return problem("workspace_busy", "Wait for session creation before binding a worktree.", 409)
	}
	return nil
}

func (s *Service) prepareWorkspaceLocked(id string) {
	requirement := s.requirement(id)
	if requirement == nil {
		return
	}
	requirement.SyncBranchGroup()
	if requirement.BranchGroup != nil {
		for _, member := range requirement.BranchGroup.Members {
			s.prepareBranchMemberLocked(id, member.ID)
		}
	}
}

type RequirementUpdate struct {
	Name    *string `json:"name"`
	Branch  *string `json:"branch"`
	BaseRef string  `json:"baseRef"`
}

func (s *Service) UpdateRequirement(ctx context.Context, id string, input RequirementUpdate) error {
	if input.Name == nil && input.Branch == nil {
		return problem("invalid_requirement", "Provide a requirement name or branch.", 400)
	}
	if input.Name != nil {
		*input.Name = strings.TrimSpace(*input.Name)
		if !validName(*input.Name) {
			return problem("invalid_name", "Enter a name of at most 120 characters.", 400)
		}
	}
	s.mu.Lock()
	requirement := s.requirement(id)
	if requirement == nil {
		s.mu.Unlock()
		return problem("requirement_not_found", "Requirement not found.", 404)
	}
	project := s.project(requirement.ProjectID)
	if input.Branch == nil {
		defer s.mu.Unlock()
		requirement.Name, requirement.DefaultName = *input.Name, false
		return s.commitLocked("navigation.updated", id, requirement)
	}
	if !project.Local {
		s.mu.Unlock()
		return problem("workspace_remote", "Automatic worktrees are available for local projects.", 409)
	}
	if !s.data.LocalSetup.Confirmed {
		s.mu.Unlock()
		return problem("local_unconfirmed", "Confirm local agents before browsing project directories.", 409)
	}
	branch := strings.TrimSpace(*input.Branch)
	previous := requirement.Workspace
	if previous != nil && branch == previous.Branch {
		defer s.mu.Unlock()
		if input.Name != nil {
			requirement.Name, requirement.DefaultName = *input.Name, false
		}
		if requirement.Workspace.State == "error" {
			requirement.Workspace.State, requirement.Workspace.Error = "preparing", ""
		}
		if err := s.commitLocked("navigation.updated", id, requirement); err != nil {
			return err
		}
		if requirement.Workspace.State == "preparing" {
			s.prepareWorkspaceLocked(id)
		}
		return nil
	}
	if previous != nil && previous.State == "preparing" {
		s.mu.Unlock()
		return problem("workspace_busy", "Wait for worktree preparation before changing the branch.", 409)
	}
	var binding workspace.Binding
	for _, retained := range requirement.PreviousWorkspaces {
		if branch == retained.Branch {
			binding = retained
			binding.State, binding.Error = "preparing", ""
			break
		}
	}
	directory := project.Cwd
	s.mu.Unlock()
	if binding.Branch == "" {
		identity := id
		if previous != nil {
			identity = model.ID()
		}
		var err error
		binding, err = workspace.Plan(ctx, directory, branch, input.BaseRef, identity)
		if err != nil {
			return problem("workspace_plan", err.Error(), 400)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	requirement = s.requirement(id)
	if requirement == nil || requirement.Workspace != previous {
		return problem("workspace_busy", "The requirement branch changed. Reload it before saving.", 409)
	}
	if previous != nil && previous.State == "preparing" {
		return problem("workspace_busy", "Wait for worktree preparation before changing the branch.", 409)
	}
	if err := s.reserveWorkspaceLocked(id, &binding); err != nil {
		return err
	}
	if input.Name != nil {
		requirement.Name, requirement.DefaultName = *input.Name, false
	}
	retained := make([]workspace.Binding, 0, len(requirement.PreviousWorkspaces)+1)
	for _, other := range requirement.PreviousWorkspaces {
		if other.CommonDir != binding.CommonDir || other.Branch != binding.Branch {
			retained = append(retained, other)
		}
	}
	if previous != nil {
		retained = append(retained, *previous)
	}
	requirement.PreviousWorkspaces = retained
	requirement.Workspace = &binding
	if err := s.commitLocked("navigation.updated", id, requirement); err != nil {
		return err
	}
	s.prepareWorkspaceLocked(id)
	return nil
}

func (s *Service) requirementWorkspaceLocked(requirement *model.Requirement, directory string) (*workspace.Binding, error) {
	if requirement == nil || requirement.Workspace == nil {
		return nil, nil
	}
	binding := requirement.Workspace
	if binding.State != "ready" {
		return nil, problem("workspace_not_ready", "The requirement worktree is not ready. Retry preparation before creating or running a session.", 409)
	}
	if directory != "" && projectDirectory(directory, true) != projectDirectory(binding.Directory, true) {
		return nil, problem("workspace_scope", "Sessions must use their requirement's worktree directory.", 400)
	}
	copy := *binding
	return &copy, nil
}

func (s *Service) sessionWorkspaceLocked(session *model.Session) (*workspace.Binding, error) {
	if session.Workspace == nil {
		return nil, nil
	}
	if projectDirectory(session.Cwd, true) != projectDirectory(session.Workspace.Directory, true) {
		return nil, problem("workspace_scope", "The session directory does not match its saved worktree.", 409)
	}
	copy := *session.Workspace
	return &copy, nil
}

func (s *Service) backfillSessionWorkspaces() {
	for _, session := range s.data.Sessions {
		if session.Workspace != nil {
			if requirement := s.requirement(session.RequirementID); requirement != nil {
				if member := requirement.BranchGroup.MemberFor(*session.Workspace); member != nil {
					session.BranchID = member.ID
				}
			}
		}
		if session.Workspace != nil || session.Purpose == "requirement" {
			continue
		}
		requirement := s.requirement(session.RequirementID)
		if requirement == nil {
			continue
		}
		for _, binding := range requirement.WorkspaceBindings() {
			if projectDirectory(session.Cwd, true) == projectDirectory(binding.Directory, true) {
				binding.State, binding.Error = "ready", ""
				session.Workspace = &binding
				if member := requirement.BranchGroup.MemberFor(binding); member != nil {
					session.BranchID = member.ID
				}
				break
			}
		}
	}
}

func (s *Service) writerAvailableLocked(session *model.Session) error {
	endpoint := s.endpoint(session.EndpointID)
	if endpoint == nil || !endpoint.Local {
		return nil
	}
	for _, directory := range sessionDirectories(session) {
		if err := s.directoryWriterAvailableLocked(directory, session.ID); err != nil {
			return err
		}
	}
	return nil
}

func sessionDirectories(session *model.Session) []string {
	values := []string{projectDirectory(session.Cwd, true)}
	if session.MaterialDirectory != "" && projectDirectory(session.MaterialDirectory, true) != values[0] {
		values = append(values, projectDirectory(session.MaterialDirectory, true))
	}
	return values
}

func (s *Service) directoryWriterAvailableLocked(directory, except string) error {
	directory = projectDirectory(directory, true)
	if s.materialWrites[directory] > 0 {
		return problem("workspace_writer_busy", "Requirement files are being saved. Wait before starting another task.", 409)
	}
	for _, other := range s.data.Sessions {
		if other.ID == except {
			continue
		}
		agent := s.endpoint(other.EndpointID)
		if agent != nil && agent.Local && (other.ActiveTurn != "" || (other.State != "idle" && s.hasTurnsLocked(other.ID))) {
			for _, value := range sessionDirectories(other) {
				if value == directory {
					return problem("workspace_writer_busy", "Another session is using this worktree. Wait for it to finish or synchronize it.", 409)
				}
			}
		}
	}
	return nil
}

func (s *Service) hasTurnsLocked(id string) bool {
	for _, turn := range s.data.Turns {
		if turn.SessionID == id {
			return true
		}
	}
	return false
}

type workspaceWriter struct {
	SessionID string `json:"sessionId"`
	Directory string `json:"directory"`
}

func (s *Service) writersLocked() []workspaceWriter {
	history := make(map[string]bool)
	for _, turn := range s.data.Turns {
		history[turn.SessionID] = true
	}
	result := []workspaceWriter{}
	for _, session := range s.data.Sessions {
		endpoint := s.endpoint(session.EndpointID)
		if endpoint != nil && endpoint.Local && (session.ActiveTurn != "" || (session.State != "idle" && history[session.ID])) {
			for _, directory := range sessionDirectories(session) {
				result = append(result, workspaceWriter{SessionID: session.ID, Directory: directory})
			}
		}
	}
	return result
}
