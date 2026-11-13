package model

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/liyiligang/loom/internal/provider"
	"github.com/liyiligang/loom/internal/workspace"
)

const Version = "0.1.0"

func ID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}

func Now() string { return time.Now().UTC().Format(time.RFC3339Nano) }

type LocalSetup struct {
	Confirmed bool `json:"confirmed"`
}

type ConfiguredModel struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
	Name     string `json:"name"`
}

type ModelCatalog struct {
	Revision int64             `json:"revision"`
	Models   []ConfiguredModel `json:"models"`
}

type Endpoint struct {
	Local       bool   `json:"local,omitempty"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	Provider    string `json:"provider"`
	Address     string `json:"address"`
	HasToken    bool   `json:"hasToken"`
	AutoConnect bool   `json:"autoConnect"`
	State       string `json:"state"`
	Error       string `json:"error,omitempty"`
	Version     string `json:"version,omitempty"`
}

type Secret struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

// SecretRecord keeps ciphertext outside the public metadata projection.
type SecretRecord struct {
	Secret
	Ciphertext []byte `json:"-"`
}

type SecretRemoval struct{ ID string }

type Project struct {
	Models     map[string]string `json:"models,omitempty"`
	SecretIDs  []string          `json:"secretIds,omitempty"`
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Cwd        string            `json:"cwd"`
	Local      bool              `json:"local"`
	EndpointID string            `json:"endpointId,omitempty"`
}

type Requirement struct {
	Models                 map[string]string   `json:"models,omitempty"`
	ConversationAccessMode provider.AccessMode `json:"conversationAccessMode,omitempty"`
	BranchGroup            *BranchGroup        `json:"branchGroup,omitempty"`
	ID                     string              `json:"id"`
	ProjectID              string              `json:"projectId"`
	Name                   string              `json:"name"`
	DefaultName            bool                `json:"defaultName,omitempty"`
	Workspace              *workspace.Binding  `json:"workspace,omitempty"`
	PreviousWorkspaces     []workspace.Binding `json:"previousWorkspaces,omitempty"`
}

// WorkspaceBindings includes retained branches as well as the default for new sessions.
func (r *Requirement) WorkspaceBindings() []workspace.Binding {
	bindings := []workspace.Binding{}
	if r.BranchGroup != nil {
		for _, member := range r.BranchGroup.Members {
			bindings = append(bindings, member.Workspace)
		}
	}
	bindings = append(bindings, r.PreviousWorkspaces...)
	if r.Workspace != nil {
		bindings = append(bindings, *r.Workspace)
	}
	unique := bindings[:0]
	seen := make(map[string]bool)
	for _, binding := range bindings {
		key := binding.CommonDir + "\x00" + binding.Branch
		if !seen[key] {
			seen[key] = true
			unique = append(unique, binding)
		}
	}
	return unique
}

type Session struct {
	ModelManaged      bool                `json:"modelManaged,omitempty"`
	AccessMode        provider.AccessMode `json:"accessMode,omitempty"`
	BranchID          string              `json:"branchId,omitempty"`
	MaterialDirectory string              `json:"materialDirectory,omitempty"`
	Purpose           string              `json:"purpose,omitempty"`
	Workspace         *workspace.Binding  `json:"workspace,omitempty"`
	RequirementID     string              `json:"requirementId"`
	ID                string              `json:"id"`
	EndpointID        string              `json:"endpointId"`
	ProviderID        string              `json:"providerId"`
	Title             string              `json:"title"`
	DefaultTitle      *bool               `json:"defaultTitle,omitempty"`
	Cwd               string              `json:"cwd"`
	Model             string              `json:"model,omitempty"`
	State             string              `json:"state"`
	ActiveTurn        string              `json:"activeTurn,omitempty"`
	Error             string              `json:"error,omitempty"`
	CreatedAt         string              `json:"createdAt"`
	UpdatedAt         string              `json:"updatedAt"`
}

type Turn struct {
	Model           *string             `json:"model,omitempty"`
	AccessMode      provider.AccessMode `json:"accessMode,omitempty"`
	Context         string              `json:"context,omitempty"`
	ID              string              `json:"id"`
	SessionID       string              `json:"sessionId"`
	ProviderID      string              `json:"providerId,omitempty"`
	ClientRequestID string              `json:"clientRequestId"`
	PriorTurnID     string              `json:"priorTurnId,omitempty"`
	BaselineKnown   bool                `json:"baselineKnown"`
	Input           string              `json:"input"`
	State           string              `json:"state"`
	Error           string              `json:"error,omitempty"`
	CreatedAt       string              `json:"createdAt"`
}

func (t *Turn) Prompt() string { return t.Context + t.Input }

type Entry struct {
	ID         string          `json:"id"`
	SessionID  string          `json:"sessionId"`
	TurnID     string          `json:"turnId"`
	ProviderID string          `json:"providerId,omitempty"`
	Kind       string          `json:"kind"`
	Role       string          `json:"role,omitempty"`
	Text       string          `json:"text"`
	Status     string          `json:"status"`
	Detail     json.RawMessage `json:"detail,omitempty"`
	Truncated  bool            `json:"truncated,omitempty"`
	CreatedAt  string          `json:"createdAt"`
}

type Interaction struct {
	ID         string          `json:"id"`
	SessionID  string          `json:"sessionId"`
	TurnID     string          `json:"turnId"`
	RequestID  json.RawMessage `json:"-"`
	Generation string          `json:"-"`
	Kind       string          `json:"kind"`
	Payload    json.RawMessage `json:"payload"`
	State      string          `json:"state"`
	Answer     json.RawMessage `json:"answer,omitempty"`
}

// InteractionRecord retains routing metadata without exposing it to browsers.
type InteractionRecord struct {
	Interaction
	NativeID   json.RawMessage `json:"nativeId"`
	Connection string          `json:"connection"`
}

type Event struct {
	ID        int64  `json:"id"`
	Type      string `json:"type"`
	Resource  string `json:"resource"`
	CreatedAt string `json:"createdAt"`
}

type Data struct {
	ModelCatalog *ModelCatalog
	Workflows    []*RequirementWorkflow
	Secrets      []*Secret
	LocalSetup   *LocalSetup
	Endpoints    []*Endpoint
	Projects     []*Project
	Requirements []*Requirement
	Sessions     []*Session
	Turns        []*Turn
	Entries      []*Entry
	Interactions []*Interaction
}

type Detail struct {
	Session      *Session       `json:"session"`
	Turns        []*Turn        `json:"turns"`
	Entries      []*Entry       `json:"entries"`
	Interactions []*Interaction `json:"interactions"`
}
