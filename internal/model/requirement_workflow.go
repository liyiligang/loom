package model

import "github.com/liyiligang/loom/internal/requirementflow"

// RequirementWorkflow is the durable preparation ledger. Document bodies and
// accepted plans stay in the requirement's filesystem, outside this projection.
type RequirementWorkflow struct {
	ID            string                     `json:"id"`
	RequirementID string                     `json:"requirementId"`
	Revision      int                        `json:"revision"`
	State         string                     `json:"state"`
	PausedState   string                     `json:"pausedState,omitempty"`
	Stage         string                     `json:"stage"`
	Attempt       int                        `json:"attempt"`
	Brief         string                     `json:"brief"`
	EndpointID    string                     `json:"endpointId"`
	SessionID     string                     `json:"sessionId,omitempty"`
	TurnID        string                     `json:"turnId,omitempty"`
	RequestID     string                     `json:"requestId"`
	LastActionID  string                     `json:"lastActionId"`
	ScopeKey      string                     `json:"scopeKey"`
	Commit        string                     `json:"commit"`
	Criteria      []string                   `json:"criteria"`
	Questions     []requirementflow.Question `json:"questions"`
	Answers       map[string]string          `json:"answers"`
	Hashes        map[string]string          `json:"hashes"`
	ReviewHash    string                     `json:"reviewHash,omitempty"`
	Error         string                     `json:"error,omitempty"`
	UpdatedAt     string                     `json:"updatedAt"`
}
