package controller

import (
	"context"
	"slices"
	"sort"

	"github.com/liyiligang/loom/internal/credentialtool"
	"github.com/liyiligang/loom/internal/model"
	"github.com/liyiligang/loom/internal/provider"
)

type credentialUse struct {
	projectID, secretID, sessionID string
	cancel                         context.CancelFunc
}

func (s *Service) credentialToolsLocked(session *model.Session) *provider.ToolServer {
	endpoint := s.endpoint(session.EndpointID)
	if endpoint == nil || !endpoint.Local {
		return nil
	}
	tools := s.credentialBroker.Session(session.ID)
	return &tools
}

func (s *Service) credentialProjectLocked(sessionID string) (*model.Project, error) {
	session := s.session(sessionID)
	if session == nil {
		return nil, problem("not_found", "Session not found.", 404)
	}
	endpoint := s.endpoint(session.EndpointID)
	if endpoint == nil || !endpoint.Local {
		return nil, problem("credentials_remote", "Project credentials are currently available for local agents.", 409)
	}
	requirement := s.requirement(session.RequirementID)
	if requirement == nil {
		return nil, problem("project_not_found", "Project not found.", 404)
	}
	project := s.project(requirement.ProjectID)
	if project == nil || !project.Local {
		return nil, problem("project_not_found", "Project not found.", 404)
	}
	turn := s.turn(session.ActiveTurn)
	if turn == nil || (turn.State != "pending" && turn.State != "running") {
		return nil, problem("credential_task_required", "Credential tools require an active task in this session.", 409)
	}
	return project, nil
}

func (s *Service) ListSessionCredentials(sessionID string) ([]model.Secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	project, err := s.credentialProjectLocked(sessionID)
	if err != nil {
		return nil, err
	}
	result := []model.Secret{}
	for _, id := range project.SecretIDs {
		if secret := s.secret(id); secret != nil {
			result = append(result, *secret)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (s *Service) LeaseSessionCredential(ctx context.Context, sessionID, name string) (credentialtool.Lease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	project, err := s.credentialProjectLocked(sessionID)
	if err != nil {
		return credentialtool.Lease{}, err
	}
	id := ""
	for _, secretID := range project.SecretIDs {
		if secret := s.secret(secretID); secret != nil && (secret.ID == name || secret.Name == name) {
			id = secret.ID
			break
		}
	}
	if id == "" {
		return credentialtool.Lease{}, problem("credential_not_active", "This credential is not activated for the session's project.", 403)
	}
	values, err := s.secretValuesLocked(id)
	if err != nil {
		return credentialtool.Lease{}, err
	}
	requestCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.ctx, cancel)
	useID := model.ID()
	s.credentialUses[useID] = credentialUse{projectID: project.ID, secretID: id, sessionID: sessionID, cancel: cancel}
	return credentialtool.Lease{Context: requestCtx, Values: values, Release: func() { cancel(); stop(); s.mu.Lock(); delete(s.credentialUses, useID); s.mu.Unlock() }}, nil
}

func (s *Service) cancelCredentialUsesLocked(projectID, secretID, sessionID string) {
	for id, use := range s.credentialUses {
		if (projectID == "" || use.projectID == projectID) && (secretID == "" || use.secretID == secretID) && (sessionID == "" || use.sessionID == sessionID) {
			use.cancel()
			delete(s.credentialUses, id)
		}
	}
}

func (s *Service) projectsWithoutSecretLocked(id string) []*model.Project {
	result := []*model.Project{}
	for _, project := range s.data.Projects {
		if !slices.Contains(project.SecretIDs, id) {
			continue
		}
		copy := *project
		copy.SecretIDs = slices.DeleteFunc(slices.Clone(project.SecretIDs), func(value string) bool { return value == id })
		result = append(result, &copy)
	}
	return result
}
