package controller

import (
	"strings"
	"unicode"

	"github.com/liyiligang/loom/internal/model"
)

// Missing provider keys inherit; an empty value selects the agent's default.
type ModelsUpdate struct {
	Models *map[string]string `json:"models"`
}

func normalizeModels(input ModelsUpdate) (map[string]string, error) {
	if input.Models == nil {
		return nil, problem("invalid_models", "Provide model preferences for Codex or Claude.", 400)
	}
	values := make(map[string]string, len(*input.Models))
	for agent, value := range *input.Models {
		value = strings.TrimSpace(value)
		if (agent != "codex" && agent != "claude") || len(value) > 120 || strings.ContainsFunc(value, unicode.IsSpace) || strings.ContainsFunc(value, unicode.IsControl) {
			return nil, problem("invalid_models", "Choose Codex or Claude and a model identifier of at most 120 bytes without spaces or control characters.", 400)
		}
		values[agent] = value
	}
	return values, nil
}

func (s *Service) UpdateProjectModels(id string, input ModelsUpdate) error {
	values, err := normalizeModels(input)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	project := s.project(id)
	if project == nil {
		return problem("project_not_found", "Project not found.", 404)
	}
	updated := *project
	updated.Models = values
	if err := s.commitLocked("navigation.updated", id, &updated); err != nil {
		return err
	}
	*project = updated
	return nil
}

func (s *Service) UpdateRequirementModels(id string, input ModelsUpdate) error {
	values, err := normalizeModels(input)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	requirement := s.requirement(id)
	if requirement == nil {
		return problem("requirement_not_found", "Requirement not found.", 404)
	}
	updated := *requirement
	updated.Models = values
	if err := s.commitLocked("navigation.updated", id, &updated); err != nil {
		return err
	}
	*requirement = updated
	return nil
}

func (s *Service) scopeModelLocked(requirement *model.Requirement, agent string) *string {
	if requirement == nil {
		return nil
	}
	if value, ok := requirement.Models[agent]; ok {
		return &value
	}
	if project := s.project(requirement.ProjectID); project != nil {
		if value, ok := project.Models[agent]; ok {
			return &value
		}
	}
	return nil
}
