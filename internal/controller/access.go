package controller

import (
	"github.com/liyiligang/loom/internal/model"
	"github.com/liyiligang/loom/internal/provider"
)

type AccessUpdate struct {
	AccessMode provider.AccessMode `json:"accessMode"`
}

func invalidAccessMode() error {
	return problem("invalid_access_mode", "Choose workspace access or full access.", 400)
}

// Updating the requirement preference never creates a native conversation or
// changes an admitted turn. Its existing conversation and future turns share it.
func (s *Service) UpdateRequirementAccess(id string, input AccessUpdate) error {
	if !input.AccessMode.Valid() {
		return invalidAccessMode()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	requirement := s.requirement(id)
	if requirement == nil {
		return problem("requirement_not_found", "Requirement not found.", 404)
	}
	requirement.ConversationAccessMode = input.AccessMode
	values := []any{requirement}
	for _, session := range s.data.Sessions {
		if session.RequirementID == id && session.Purpose == "requirement" {
			session.AccessMode, session.UpdatedAt = input.AccessMode, model.Now()
			values = append(values, session)
		}
	}
	return s.commitLocked("navigation.updated", id, values...)
}
