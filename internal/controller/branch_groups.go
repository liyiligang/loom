package controller

import (
	"context"
	"strings"

	"github.com/liyiligang/loom/internal/model"
	"github.com/liyiligang/loom/internal/workspace"
)

type BranchGroupInput struct {
	Prefix  string `json:"prefix"`
	BaseRef string `json:"baseRef"`
}

type BranchMemberInput struct {
	Name           string `json:"name"`
	BaseRef        string `json:"baseRef"`
	SourceBranchID string `json:"sourceBranchId"`
}

func (s *Service) localRequirementLocked(id string) (*model.Requirement, *model.Project, error) {
	requirement := s.requirement(id)
	if requirement == nil {
		return nil, nil, problem("requirement_not_found", "Requirement not found.", 404)
	}
	project := s.project(requirement.ProjectID)
	if !project.Local {
		return nil, nil, problem("workspace_remote", "Automatic worktrees are available for local projects.", 409)
	}
	if !s.data.LocalSetup.Confirmed {
		return nil, nil, problem("local_unconfirmed", "Confirm local agents before browsing project directories.", 409)
	}
	return requirement, project, nil
}

func (s *Service) BindBranchGroup(ctx context.Context, id string, input BranchGroupInput) error {
	s.mu.Lock()
	requirement, project, err := s.localRequirementLocked(id)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	previous, directory := requirement.BranchGroup, project.Cwd
	base := input.BaseRef
	if previous != nil && base == "" {
		base = previous.BaseCommit
	}
	s.mu.Unlock()
	plan, err := workspace.PlanGroup(ctx, directory, input.Prefix, base)
	if err != nil {
		return problem("branch_group_invalid", "Enter a valid, available branch group prefix and base reference.", 400)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	requirement = s.requirement(id)
	if requirement == nil || requirement.BranchGroup != previous || s.creatingSessions[id] > 0 {
		return problem("workspace_busy", "Wait for session creation before binding a worktree.", 409)
	}
	if previous != nil && previous.CommonDir != plan.CommonDir {
		return problem("workspace_plan", "The branch or repository changed during preparation. Refresh and retry.", 409)
	}
	for _, other := range s.data.Requirements {
		if other.ID != id {
			for _, binding := range other.WorkspaceBindings() {
				if binding.CommonDir == plan.CommonDir && strings.HasPrefix(binding.Branch, plan.Prefix+"/") {
					return problem("branch_group_owned", "This branch group prefix overlaps another requirement's group.", 409)
				}
			}
		}
		group := other.BranchGroup
		if other.ID != id && group != nil && group.CommonDir == plan.CommonDir &&
			(group.Prefix == plan.Prefix || strings.HasPrefix(group.Prefix, plan.Prefix+"/") || strings.HasPrefix(plan.Prefix, group.Prefix+"/")) {
			return problem("branch_group_owned", "This branch group prefix overlaps another requirement's group.", 409)
		}
	}
	group := &model.BranchGroup{ID: model.ID(), Members: []model.BranchMember{}}
	if previous != nil {
		*group = *previous
		group.Members = append([]model.BranchMember{}, previous.Members...)
	}
	group.Prefix, group.RepositoryRoot, group.CommonDir = plan.Prefix, plan.RepositoryRoot, plan.CommonDir
	group.BaseRef, group.BaseCommit, group.ExplicitSelection = plan.BaseRef, plan.BaseCommit, true
	if previous != nil && input.BaseRef == "" {
		group.BaseRef = previous.BaseRef
	}
	requirement.BranchGroup = group
	return s.commitLocked("navigation.updated", id, requirement)
}

func (s *Service) CreateBranchMember(ctx context.Context, id string, input BranchMemberInput) (model.BranchMember, error) {
	s.mu.Lock()
	requirement, project, err := s.localRequirementLocked(id)
	if err != nil {
		s.mu.Unlock()
		return model.BranchMember{}, err
	}
	group := requirement.BranchGroup
	if group == nil {
		s.mu.Unlock()
		return model.BranchMember{}, problem("branch_group_required", "Bind a branch group before adding branches.", 409)
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		s.mu.Unlock()
		return model.BranchMember{}, problem("branch_member_invalid", "Enter a branch name within this group.", 400)
	}
	branch := name
	if !strings.HasPrefix(branch, group.Prefix+"/") {
		branch = group.Prefix + "/" + name
	}
	for _, member := range group.Members {
		if member.Workspace.Branch == branch {
			s.mu.Unlock()
			return member, nil
		}
	}
	base, directory := input.BaseRef, project.Cwd
	if input.SourceBranchID != "" {
		source := group.Member(input.SourceBranchID)
		if source == nil || source.Workspace.State != "ready" || input.BaseRef != "" {
			s.mu.Unlock()
			return model.BranchMember{}, problem("branch_member_invalid", "Choose a ready source branch or a base reference.", 400)
		}
		base = "refs/heads/" + source.Workspace.Branch
	}
	if base == "" {
		base = group.BaseCommit
	}
	identity := model.ID()
	s.mu.Unlock()
	binding, err := workspace.Plan(ctx, directory, branch, base, identity)
	if err != nil {
		return model.BranchMember{}, problem("workspace_plan", err.Error(), 400)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	requirement = s.requirement(id)
	if requirement == nil || requirement.BranchGroup != group || group.CommonDir != binding.CommonDir {
		return model.BranchMember{}, problem("workspace_busy", "The requirement branch changed. Reload it before saving.", 409)
	}
	for _, member := range group.Members {
		if member.Workspace.Branch == branch {
			return member, nil
		}
	}
	if err := s.reserveWorkspaceLocked(id, &binding); err != nil {
		return model.BranchMember{}, err
	}
	member := model.BranchMember{ID: model.BranchMemberID(id, binding), Workspace: binding}
	group.ExplicitSelection = true
	group.Members = append(group.Members, member)
	if err := s.commitLocked("navigation.updated", id, requirement); err != nil {
		return model.BranchMember{}, err
	}
	s.prepareBranchMemberLocked(id, member.ID)
	return member, nil
}

func (s *Service) RetryBranchMember(id, memberID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	requirement, _, err := s.localRequirementLocked(id)
	if err != nil {
		return err
	}
	member := requirement.BranchGroup.Member(memberID)
	if member == nil {
		return problem("branch_member_not_found", "Branch not found in this requirement's group.", 404)
	}
	if s.creatingSessions[id] > 0 {
		return problem("workspace_busy", "Wait for session creation before binding a worktree.", 409)
	}
	if err := s.directoryWriterAvailableLocked(member.Workspace.Directory, ""); err != nil {
		return err
	}
	if member.Workspace.State == "preparing" {
		s.prepareBranchMemberLocked(id, memberID)
		return nil
	}
	requirement.SetMemberState(memberID, "preparing", "")
	if err := s.commitLocked("navigation.updated", id, requirement); err != nil {
		return err
	}
	s.prepareBranchMemberLocked(id, memberID)
	return nil
}

func (s *Service) prepareBranchMemberLocked(id, memberID string) {
	requirement := s.requirement(id)
	if requirement == nil || !s.data.LocalSetup.Confirmed || s.preparingWorkspaces[memberID] {
		return
	}
	member := requirement.BranchGroup.Member(memberID)
	if member == nil || member.Workspace.State != "preparing" {
		return
	}
	s.preparingWorkspaces[memberID] = true
	binding, groupID := member.Workspace, requirement.BranchGroup.ID
	s.workers.Add(1)
	go func() {
		defer s.workers.Done()
		err := workspace.Prepare(s.ctx, binding)
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.preparingWorkspaces, memberID)
		if s.ctx.Err() != nil {
			return
		}
		requirement := s.requirement(id)
		if requirement == nil || requirement.BranchGroup == nil || requirement.BranchGroup.ID != groupID {
			return
		}
		current := requirement.BranchGroup.Member(memberID)
		if current == nil || current.Workspace.Path != binding.Path || current.Workspace.Branch != binding.Branch {
			return
		}
		state, message := "ready", ""
		if err != nil {
			state, message = "error", err.Error()
		}
		requirement.SetMemberState(memberID, state, message)
		s.commitLocked("navigation.updated", id, requirement)
	}()
}

func (s *Service) SetMaterialBranch(ctx context.Context, id, memberID string) error {
	s.mu.Lock()
	requirement, _, err := s.localRequirementLocked(id)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	group := requirement.BranchGroup
	if group == nil {
		s.mu.Unlock()
		return problem("branch_group_required", "Bind a branch group before adding branches.", 409)
	}
	var selected *workspace.Binding
	if memberID != "" {
		member := group.Member(memberID)
		if member == nil || member.Workspace.State != "ready" {
			s.mu.Unlock()
			return problem("branch_member_invalid", "Choose a ready branch from this requirement's group.", 400)
		}
		copy := member.Workspace
		selected = &copy
	}
	s.mu.Unlock()
	if selected != nil {
		if err := workspace.Verify(ctx, *selected); err != nil {
			return problem("workspace_unavailable", err.Error(), 409)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	requirement = s.requirement(id)
	if requirement == nil || requirement.BranchGroup != group || s.creatingSessions[id] > 0 {
		return problem("workspace_busy", "Wait for session creation before binding a worktree.", 409)
	}
	// Preserve the legacy projection and all prior workspaces without moving files.
	retained := []workspace.Binding{}
	for _, binding := range requirement.WorkspaceBindings() {
		if selected == nil || binding.CommonDir != selected.CommonDir || binding.Branch != selected.Branch {
			retained = append(retained, binding)
		}
	}
	requirement.Workspace, requirement.PreviousWorkspaces = selected, retained
	group.MaterialBranchID = memberID
	group.ExplicitSelection = true
	return s.commitLocked("navigation.updated", id, requirement)
}

func (s *Service) executionWorkspaceLocked(requirement *model.Requirement, memberID, directory string) (*workspace.Binding, string, error) {
	group := requirement.BranchGroup
	if memberID == "" {
		if group != nil && group.ExplicitSelection {
			return nil, "", problem("branch_member_required", "Choose a branch from this requirement's group before creating a session.", 400)
		}
		binding, err := s.requirementWorkspaceLocked(requirement, directory)
		if binding != nil && group != nil {
			if member := group.MemberFor(*binding); member != nil {
				memberID = member.ID
			}
		}
		return binding, memberID, err
	}
	member := group.Member(memberID)
	if member == nil {
		return nil, "", problem("branch_member_not_found", "Branch not found in this requirement's group.", 400)
	}
	binding := member.Workspace
	if binding.State != "ready" {
		return nil, "", problem("workspace_not_ready", "The requirement worktree is not ready. Retry preparation before creating or running a session.", 409)
	}
	if directory != "" && projectDirectory(directory, true) != projectDirectory(binding.Directory, true) {
		return nil, "", problem("workspace_scope", "Sessions must use their requirement's worktree directory.", 400)
	}
	return &binding, memberID, nil
}
