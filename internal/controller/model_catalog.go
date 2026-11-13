package controller

import (
	"slices"
	"strings"
	"unicode"

	"github.com/liyiligang/loom/internal/model"
)

type ModelCatalogInput struct {
	Revision int64                    `json:"revision"`
	Models   *[]model.ConfiguredModel `json:"models"`
}

func (s *Service) ModelCatalog() model.ModelCatalog {
	s.mu.Lock()
	defer s.mu.Unlock()
	value := *s.data.ModelCatalog
	value.Models = slices.Clone(value.Models)
	return value
}

func (s *Service) UpdateModelCatalog(input ModelCatalogInput) (model.ModelCatalog, error) {
	invalid := func() (model.ModelCatalog, error) {
		return model.ModelCatalog{}, problem("invalid_model_catalog", "Enter at most 200 models with a valid agent, display name and model identifier. Use each identifier once per agent.", 400)
	}
	if input.Models == nil || len(*input.Models) > 200 {
		return invalid()
	}
	values := make([]model.ConfiguredModel, 0, len(*input.Models))
	seen := make(map[string]bool)
	for _, value := range *input.Models {
		value.Name, value.ID = strings.TrimSpace(value.Name), strings.TrimSpace(value.ID)
		if !validName(value.Name) || (value.Provider != "codex" && value.Provider != "claude") || value.ID == "" || len(value.ID) > 120 || strings.ContainsFunc(value.ID, unicode.IsSpace) || strings.ContainsFunc(value.ID, unicode.IsControl) {
			return invalid()
		}
		key := value.Provider + "\x00" + value.ID
		if seen[key] {
			return invalid()
		}
		seen[key] = true
		values = append(values, value)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if input.Revision != s.data.ModelCatalog.Revision {
		return model.ModelCatalog{}, problem("model_catalog_conflict", "Model settings changed elsewhere. Reload the saved list before saving.", 409)
	}
	updated := model.ModelCatalog{Models: values, Revision: input.Revision + 1}
	if err := s.commitLocked("settings.updated", "ai_models", &updated); err != nil {
		return model.ModelCatalog{}, err
	}
	s.data.ModelCatalog = &updated
	return model.ModelCatalog{Revision: updated.Revision, Models: slices.Clone(updated.Models)}, nil
}
