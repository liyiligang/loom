package model

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"
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

type Endpoint struct {
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

type Session struct {
	ID         string `json:"id"`
	EndpointID string `json:"endpointId"`
	ProviderID string `json:"providerId"`
	Title      string `json:"title"`
	Cwd        string `json:"cwd"`
	Model      string `json:"model,omitempty"`
	State      string `json:"state"`
	ActiveTurn string `json:"activeTurn,omitempty"`
	Error      string `json:"error,omitempty"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

type Turn struct {
	ID              string `json:"id"`
	SessionID       string `json:"sessionId"`
	ProviderID      string `json:"providerId,omitempty"`
	ClientRequestID string `json:"clientRequestId"`
	PriorTurnID     string `json:"priorTurnId,omitempty"`
	BaselineKnown   bool   `json:"baselineKnown"`
	Input           string `json:"input"`
	State           string `json:"state"`
	Error           string `json:"error,omitempty"`
	CreatedAt       string `json:"createdAt"`
}

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
	Endpoints    []*Endpoint
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
