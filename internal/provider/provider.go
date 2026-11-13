package provider

import (
	"context"
	"encoding/json"
	"errors"
)

var ErrEmptySession = errors.New("The agent cannot restore an empty session after its connection closes. Create a new session.")

// Connection owns an agent transport. Session IDs and item IDs belong to that agent.
// A successful response acknowledges an operation; completion arrives through Events.
type Connection interface {
	CreateSession(context.Context, SessionOptions) (SessionSnapshot, error)
	SynchronizeSession(context.Context, SessionReference) (SessionSnapshot, error)
	StartTurn(context.Context, string, TurnOptions) (TurnSnapshot, error)
	Interrupt(context.Context, string, string) error
	Reply(json.RawMessage, any) error
	Reject(json.RawMessage, string) error
	Events() <-chan Event
	Done() <-chan struct{}
	Version() string
	Close()
}

type SessionOptions struct {
	AccessMode AccessMode
	Cwd        string
	Model      string
	Tools      *ToolServer
}

type ToolServer struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

type Model struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"isDefault,omitempty"`
}

type TurnOptions struct {
	Cwd        string
	Model      *string
	AccessMode AccessMode
	Text       string
	Tools      *ToolServer
}

type SessionReference struct {
	Model      *string
	AccessMode AccessMode
	ID         string
	Cwd        string
	Subscribed bool
	Empty      bool
	Tools      *ToolServer
}

// AccessMode is selected by the user and captured when a turn is admitted.
type AccessMode string

const (
	AccessWorkspace AccessMode = "workspace"
	AccessFull      AccessMode = "full"
)

func (mode AccessMode) Valid() bool { return mode == AccessWorkspace || mode == AccessFull }

// Legacy sessions without a saved choice retain restricted access.
func (mode AccessMode) Effective() AccessMode {
	if mode == AccessFull {
		return AccessFull
	}
	return AccessWorkspace
}

type SessionSnapshot struct {
	ID    string
	Model string
	Turns []TurnSnapshot
}

type TurnSnapshot struct {
	ID    string
	State string
	Items []Item
	Error string
}

type Item struct {
	ID     string
	Kind   string
	Role   string
	Text   string
	Output *string
	Detail json.RawMessage
}

// Rejection is an explicit agent rejection, distinct from a lost acknowledgement.
type Rejection struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *Rejection) Error() string { return e.Message }

type EventKind string

const (
	TurnStarted         EventKind = "turn.started"
	TurnCompleted       EventKind = "turn.completed"
	ItemStarted         EventKind = "item.started"
	ItemCompleted       EventKind = "item.completed"
	TextDelta           EventKind = "message.delta"
	OutputDelta         EventKind = "output.delta"
	ApprovalRequested   EventKind = "approval.requested"
	QuestionRequested   EventKind = "question.requested"
	InteractionResolved EventKind = "interaction.resolved"
	ExecutionError      EventKind = "execution.error"
)

type Event struct {
	Kind            EventKind
	SessionID       string
	TurnID          string
	ItemID          string
	Delta           string
	Turn            TurnSnapshot
	Item            Item
	RequestID       json.RawMessage
	ResolvedID      json.RawMessage
	InteractionKind string
	Payload         json.RawMessage
	Error           string
	WillRetry       bool
}

func (e Event) IsDelta() bool {
	return len(e.RequestID) == 0 && (e.Kind == TextDelta || e.Kind == OutputDelta)
}
