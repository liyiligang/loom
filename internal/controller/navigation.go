package controller

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/liyiligang/loom/internal/model"
	"github.com/liyiligang/loom/internal/workspace"
)

type ProjectInput struct {
	Name       string `json:"name"`
	Cwd        string `json:"cwd"`
	EndpointID string `json:"endpointId"`
}

type RequirementInput struct {
	ProjectID string  `json:"projectId"`
	Name      string  `json:"name"`
	Branch    *string `json:"branch"`
	BaseRef   string  `json:"baseRef"`
}

func validName(name string) bool {
	return name != "" && utf8.RuneCountInString(name) <= 120 && !strings.ContainsRune(name, 0)
}

func (s *Service) project(id string) *model.Project {
	for _, value := range s.data.Projects {
		if value.ID == id {
			return value
		}
	}
	return nil
}

func (s *Service) requirement(id string) *model.Requirement {
	for _, value := range s.data.Requirements {
		if value.ID == id {
			return value
		}
	}
	return nil
}

// Only local paths can be resolved against this computer's filesystem. Missing
// legacy directories keep their grouping and history without blocking startup.
func projectDirectory(cwd string, local bool) string {
	if !local {
		return cwd
	}
	if resolved, err := filepath.EvalSymlinks(cwd); err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(cwd)
}

func projectName(cwd string) string {
	parts := strings.Split(strings.TrimRight(strings.ReplaceAll(cwd, "\\", "/"), "/"), "/")
	name := parts[len(parts)-1]
	if name == "" {
		return cwd
	}
	return string([]rune(name)[:min(120, utf8.RuneCountInString(name))])
}

func (s *Service) ensureProjectLocked(endpoint *model.Endpoint, cwd string) (*model.Project, []any) {
	cwd = projectDirectory(cwd, endpoint.Local)
	endpointID := endpoint.ID
	if endpoint.Local {
		endpointID = ""
	}
	for _, project := range s.data.Projects {
		if project.Local == endpoint.Local && project.EndpointID == endpointID && project.Cwd == cwd {
			return project, nil
		}
	}
	project := &model.Project{ID: model.ID(), Name: projectName(cwd), Cwd: cwd, Local: endpoint.Local, EndpointID: endpointID}
	s.data.Projects = append(s.data.Projects, project)
	return project, []any{project}
}

func (s *Service) defaultRequirementLocked(project *model.Project) (*model.Requirement, []any) {
	for _, requirement := range s.data.Requirements {
		if requirement.ProjectID == project.ID && requirement.DefaultName {
			return requirement, nil
		}
	}
	requirement := &model.Requirement{ID: model.ID(), ProjectID: project.ID, Name: "General", DefaultName: true}
	s.data.Requirements = append(s.data.Requirements, requirement)
	return requirement, []any{requirement}
}

func (s *Service) backfillNavigation() ([]any, error) {
	var values []any
	for _, session := range s.data.Sessions {
		if session.RequirementID != "" {
			continue
		}
		endpoint := s.endpoint(session.EndpointID)
		if endpoint == nil {
			return nil, problem("not_found", "Service not found.", 404)
		}
		project, created := s.ensureProjectLocked(endpoint, session.Cwd)
		values = append(values, created...)
		requirement, created := s.defaultRequirementLocked(project)
		values = append(values, created...)
		session.RequirementID = requirement.ID
		values = append(values, session)
	}
	return values, nil
}

func (s *Service) CreateProject(input ProjectInput) (string, error) {
	input.Name, input.Cwd = strings.TrimSpace(input.Name), strings.TrimSpace(input.Cwd)
	if input.Name != "" && !validName(input.Name) {
		return "", problem("invalid_name", "Enter a name of at most 120 characters.", 400)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	endpoint := s.endpoint(input.EndpointID)
	if endpoint == nil {
		return "", problem("not_found", "Service not found.", 404)
	}
	if endpoint.Local && !s.data.LocalSetup.Confirmed {
		return "", problem("local_unconfirmed", "Confirm local agents before browsing project directories.", 409)
	}
	if endpoint.Local && input.Cwd == "" {
		input.Cwd = s.defaultDirectory
	}
	if input.Cwd == "" || len(input.Cwd) > 4096 || strings.ContainsRune(input.Cwd, 0) {
		return "", problem("invalid_directory", "Enter a working directory on the target machine.", 400)
	}
	if endpoint.Local {
		info, err := os.Stat(input.Cwd)
		if err != nil || !filepath.IsAbs(input.Cwd) || !info.IsDir() {
			return "", problem("local_directory", "The project directory is unavailable.", 400)
		}
	}
	project, values := s.ensureProjectLocked(endpoint, input.Cwd)
	if len(values) == 0 {
		return "", problem("duplicate_project", "This project directory is already added.", 409)
	}
	if input.Name != "" {
		project.Name = input.Name
	}
	_, created := s.defaultRequirementLocked(project)
	values = append(values, created...)
	if err := s.commitLocked("navigation.updated", project.ID, values...); err != nil {
		return "", err
	}
	return project.ID, nil
}

func (s *Service) RenameProject(id, name string) error {
	return s.UpdateProject(id, ProjectUpdate{Name: &name})
}

type ProjectUpdate struct {
	Name      *string   `json:"name"`
	SecretIDs *[]string `json:"secretIds"`
}

func (s *Service) UpdateProject(id string, input ProjectUpdate) error {
	if input.Name != nil {
		value := strings.TrimSpace(*input.Name)
		input.Name = &value
	}
	if input.Name != nil && !validName(*input.Name) {
		return problem("invalid_name", "Enter a name of at most 120 characters.", 400)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	project := s.project(id)
	if project == nil {
		return problem("project_not_found", "Project not found.", 404)
	}
	if input.Name == nil && input.SecretIDs == nil {
		return problem("invalid_project_update", "Choose a project name or credentials to update.", 400)
	}
	updated := *project
	if input.Name != nil {
		updated.Name = *input.Name
	}
	if input.SecretIDs != nil {
		if len(*input.SecretIDs) > 100 {
			return problem("invalid_project_credentials", "Choose at most 100 credentials for a project.", 400)
		}
		if !project.Local && len(*input.SecretIDs) != 0 {
			return problem("credentials_remote", "Project credentials are currently available for local agents.", 409)
		}
		updated.SecretIDs = []string{}
		for _, secretID := range *input.SecretIDs {
			if s.secret(secretID) == nil {
				return problem("secret_not_found", "Credential not found.", 404)
			}
			if slices.Contains(updated.SecretIDs, secretID) {
				return problem("invalid_project_credentials", "Choose each credential only once.", 400)
			}
			updated.SecretIDs = append(updated.SecretIDs, secretID)
		}
	}
	if err := s.commitLocked("navigation.updated", id, &updated); err != nil {
		return err
	}
	for _, secretID := range project.SecretIDs {
		if !slices.Contains(updated.SecretIDs, secretID) {
			s.cancelCredentialUsesLocked(id, secretID, "")
		}
	}
	*project = updated
	return nil
}

func (s *Service) CreateRequirement(ctx context.Context, input RequirementInput) (string, error) {
	input.Name = strings.TrimSpace(input.Name)
	if !validName(input.Name) {
		return "", problem("invalid_name", "Enter a name of at most 120 characters.", 400)
	}
	s.mu.Lock()
	project := s.project(input.ProjectID)
	if project == nil {
		s.mu.Unlock()
		return "", problem("project_not_found", "Project not found.", 404)
	}
	requirement := &model.Requirement{ID: model.ID(), ProjectID: input.ProjectID, Name: input.Name}
	if input.Branch != nil {
		if !project.Local {
			s.mu.Unlock()
			return "", problem("workspace_remote", "Automatic worktrees are available for local projects.", 409)
		}
		if !s.data.LocalSetup.Confirmed {
			s.mu.Unlock()
			return "", problem("local_unconfirmed", "Confirm local agents before browsing project directories.", 409)
		}
		directory := project.Cwd
		s.mu.Unlock()
		binding, err := workspace.Plan(ctx, directory, *input.Branch, input.BaseRef, requirement.ID)
		if err != nil {
			return "", problem("workspace_plan", err.Error(), 400)
		}
		s.mu.Lock()
		if err := s.reserveWorkspaceLocked(requirement.ID, &binding); err != nil {
			s.mu.Unlock()
			return "", err
		}
		requirement.Workspace = &binding
	}
	defer s.mu.Unlock()
	s.data.Requirements = append(s.data.Requirements, requirement)
	if err := s.commitLocked("navigation.updated", requirement.ID, requirement); err != nil {
		return "", err
	}
	if requirement.Workspace != nil {
		s.prepareWorkspaceLocked(requirement.ID)
	}
	return requirement.ID, nil
}

func (s *Service) requirementForEndpointLocked(id string, endpoint *model.Endpoint) (*model.Requirement, error) {
	requirement := s.requirement(id)
	if requirement == nil {
		return nil, problem("requirement_not_found", "Requirement not found.", 404)
	}
	project := s.project(requirement.ProjectID)
	if project == nil || endpoint == nil || project.Local != endpoint.Local || (!project.Local && project.EndpointID != endpoint.ID) {
		return nil, problem("project_scope", "Choose a requirement on the same machine as this session.", 400)
	}
	return requirement, nil
}
