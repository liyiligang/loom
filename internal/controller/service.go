package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/liyiligang/loom/internal/credentials"
	"github.com/liyiligang/loom/internal/model"
	"github.com/liyiligang/loom/internal/provider/codex"
	"github.com/liyiligang/loom/internal/storage"
)

type Error struct {
	Code, Message string
	Status        int
}

func (e *Error) Error() string { return e.Message }
func problem(code, message string, status int) error {
	return &Error{Code: code, Message: message, Status: status}
}

type connection struct {
	client     *codex.Client
	generation string
}

type Service struct {
	mu         sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	workers    sync.WaitGroup
	store      *storage.Store
	secrets    *credentials.Store
	data       model.Data
	clients    map[string]*connection
	attempts   map[string]bool
	subscribed map[string]string
	restoring  map[string]bool
	changed    chan struct{}
	cursor     int64
	fault      error
	Fatal      chan error
}

func New(ctx context.Context, store *storage.Store, secrets *credentials.Store) (*Service, error) {
	data, err := store.Load()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	s := &Service{ctx: ctx, cancel: cancel, store: store, secrets: secrets, data: data,
		clients: make(map[string]*connection), attempts: make(map[string]bool), subscribed: make(map[string]string), restoring: make(map[string]bool), changed: make(chan struct{}), Fatal: make(chan error, 1)}
	s.cursor, err = store.Cursor()
	if err != nil {
		cancel()
		return nil, err
	}
	var recovered []any
	for _, endpoint := range data.Endpoints {
		endpoint.State = "offline"
		recovered = append(recovered, endpoint)
	}
	for _, session := range data.Sessions {
		if session.ActiveTurn != "" {
			session.State, session.Error = "syncing", "Execution status requires synchronization with the agent."
			if turn := s.turn(session.ActiveTurn); turn != nil {
				turn.State = "unknown"
				recovered = append(recovered, turn)
			}
		}
		recovered = append(recovered, session)
	}
	for _, interaction := range data.Interactions {
		if interaction.State == "pending" || interaction.State == "replying" {
			interaction.State = "unknown"
			recovered = append(recovered, interaction)
		}
	}
	if len(recovered) > 0 {
		if _, err := store.Commit("state.updated", "", recovered...); err != nil {
			cancel()
			return nil, err
		}
		s.cursor, _ = store.Cursor()
	}
	return s, nil
}

func (s *Service) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, endpoint := range s.data.Endpoints {
		if endpoint.AutoConnect {
			s.connectLocked(endpoint)
		}
	}
}

func (s *Service) Close() {
	s.cancel()
	s.mu.Lock()
	for _, connection := range s.clients {
		connection.client.Close()
	}
	s.mu.Unlock()
	s.workers.Wait()
}

func (s *Service) commitLocked(kind, id string, values ...any) error {
	if s.fault != nil {
		return s.fault
	}
	cursor, err := s.store.Commit(kind, id, values...)
	if err != nil {
		s.fault = fmt.Errorf("persist controller state: %w", err)
		select {
		case s.Fatal <- s.fault:
		default:
		}
		s.cancel()
		return s.fault
	}
	s.cursor = cursor
	close(s.changed)
	s.changed = make(chan struct{})
	return nil
}

func (s *Service) Watch() <-chan struct{}                    { s.mu.Lock(); defer s.mu.Unlock(); return s.changed }
func (s *Service) Events(after int64) ([]model.Event, error) { return s.store.Events(after) }

func (s *Service) Bootstrap() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sessions := append([]*model.Session{}, s.data.Sessions...)
	sort.SliceStable(sessions, func(i, j int) bool { return sessions[i].UpdatedAt > sessions[j].UpdatedAt })
	return json.Marshal(map[string]any{"version": model.Version, "cursor": s.cursor,
		"endpoints": append([]*model.Endpoint{}, s.data.Endpoints...), "sessions": sessions,
		"capabilities": map[string]bool{"codex": true, "claude": false, "mdns": false}})
}

func (s *Service) Detail(id string) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.session(id)
	if session == nil {
		return nil, problem("not_found", "Session not found.", 404)
	}
	detail := model.Detail{Session: session, Turns: []*model.Turn{}, Entries: []*model.Entry{}, Interactions: []*model.Interaction{}}
	for _, turn := range s.data.Turns {
		if turn.SessionID == id {
			detail.Turns = append(detail.Turns, turn)
		}
	}
	for _, entry := range s.data.Entries {
		if entry.SessionID == id {
			detail.Entries = append(detail.Entries, entry)
		}
	}
	for _, interaction := range s.data.Interactions {
		if interaction.SessionID == id {
			detail.Interactions = append(detail.Interactions, interaction)
		}
	}
	return json.Marshal(struct {
		model.Detail
		Cursor int64 `json:"cursor"`
	}{detail, s.cursor})
}

func (s *Service) endpoint(id string) *model.Endpoint {
	for _, v := range s.data.Endpoints {
		if v.ID == id {
			return v
		}
	}
	return nil
}
func (s *Service) session(id string) *model.Session {
	for _, v := range s.data.Sessions {
		if v.ID == id {
			return v
		}
	}
	return nil
}
func (s *Service) turn(id string) *model.Turn {
	for _, v := range s.data.Turns {
		if v.ID == id {
			return v
		}
	}
	return nil
}
func (s *Service) sessionForThread(endpointID, threadID string) *model.Session {
	for _, v := range s.data.Sessions {
		if v.EndpointID == endpointID && v.ProviderID == threadID {
			return v
		}
	}
	return nil
}

func NormalizeAddress(address string) (string, error) {
	target, err := url.Parse(strings.TrimSpace(address))
	if err != nil || (target.Scheme != "ws" && target.Scheme != "wss") || target.Hostname() == "" || target.User != nil || target.RawQuery != "" || target.Fragment != "" {
		return "", problem("invalid_address", "Enter a ws:// or wss:// service address without credentials or a query string.", 400)
	}
	target.Host = strings.ToLower(target.Host)
	if target.Path == "" {
		target.Path = "/"
	}
	return target.String(), nil
}

type EndpointInput struct {
	Name    string  `json:"name"`
	Address string  `json:"address"`
	Token   *string `json:"token"`
}

func (s *Service) SaveEndpoint(id string, input EndpointInput) (string, error) {
	address, err := NormalizeAddress(input.Address)
	if err != nil {
		return "", err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" || len(name) > 120 || len(address) > 2048 {
		return "", problem("invalid_endpoint", "A service name of at most 120 characters is required.", 400)
	}
	if input.Token != nil && (len(*input.Token) > 8192 || strings.ContainsAny(*input.Token, "\r\n")) {
		return "", problem("invalid_token", "The service token is invalid.", 400)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, endpoint := range s.data.Endpoints {
		if endpoint.Address == address && endpoint.ID != id {
			return "", problem("duplicate_endpoint", "This service address is already saved.", 409)
		}
	}
	endpoint := s.endpoint(id)
	if id != "" && endpoint == nil {
		return "", problem("not_found", "Service not found.", 404)
	}
	if endpoint != nil {
		for _, session := range s.data.Sessions {
			if session.EndpointID == id && session.ActiveTurn != "" {
				return "", problem("session_busy", "Interrupt active tasks before changing this connection.", 409)
			}
		}
	} else {
		endpoint = &model.Endpoint{ID: model.ID(), Provider: "codex"}
	}
	if input.Token != nil {
		if err := s.secrets.Save(endpoint.ID, *input.Token); err != nil {
			return "", errors.New("could not save the service credential")
		}
		endpoint.HasToken = *input.Token != ""
	}
	endpoint.Name, endpoint.Address, endpoint.AutoConnect, endpoint.State, endpoint.Error = name, address, true, "offline", ""
	if id == "" {
		s.data.Endpoints = append(s.data.Endpoints, endpoint)
	}
	if err := s.commitLocked("endpoint.updated", endpoint.ID, endpoint); err != nil {
		return "", err
	}
	if existing := s.clients[endpoint.ID]; existing != nil {
		existing.client.Close()
	}
	s.connectLocked(endpoint)
	return endpoint.ID, nil
}

func (s *Service) Connect(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	endpoint := s.endpoint(id)
	if endpoint == nil {
		return problem("not_found", "Service not found.", 404)
	}
	endpoint.AutoConnect = true
	if err := s.commitLocked("endpoint.updated", id, endpoint); err != nil {
		return err
	}
	s.connectLocked(endpoint)
	return nil
}

func (s *Service) Disconnect(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	endpoint := s.endpoint(id)
	if endpoint == nil {
		return problem("not_found", "Service not found.", 404)
	}
	for _, session := range s.data.Sessions {
		if session.EndpointID == id && session.ActiveTurn != "" {
			return problem("session_busy", "Interrupt and confirm active tasks before disconnecting.", 409)
		}
	}
	endpoint.AutoConnect, endpoint.State = false, "disconnected"
	if existing := s.clients[id]; existing != nil {
		existing.client.Close()
	}
	return s.commitLocked("endpoint.updated", id, endpoint)
}

func (s *Service) connectLocked(endpoint *model.Endpoint) {
	if s.attempts[endpoint.ID] || s.ctx.Err() != nil {
		return
	}
	s.attempts[endpoint.ID] = true
	s.workers.Add(1)
	go s.connectionLoop(endpoint.ID)
}

func (s *Service) connectionLoop(id string) {
	defer s.workers.Done()
	defer func() { s.mu.Lock(); delete(s.attempts, id); s.mu.Unlock() }()
	backoff := time.Second
	for s.ctx.Err() == nil {
		s.mu.Lock()
		endpoint := s.endpoint(id)
		if !endpoint.AutoConnect {
			s.mu.Unlock()
			return
		}
		address := endpoint.Address
		endpoint.State, endpoint.Error = "connecting", ""
		if s.commitLocked("endpoint.updated", id, endpoint) != nil {
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
		token, err := s.secrets.Read(id)
		var client *codex.Client
		if err == nil {
			ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
			client, err = codex.Dial(ctx, address, token)
			cancel()
		}
		s.mu.Lock()
		endpoint = s.endpoint(id)
		if !endpoint.AutoConnect || s.ctx.Err() != nil || endpoint.Address != address {
			s.mu.Unlock()
			if client != nil {
				client.Close()
			}
			if s.ctx.Err() != nil {
				return
			}
			continue
		}
		if err == nil {
			current := &connection{client: client, generation: model.ID()}
			s.clients[id] = current
			endpoint.State, endpoint.Version, endpoint.Error = "ready", client.Version, ""
			s.commitLocked("endpoint.updated", id, endpoint)
			var sessions []string
			for _, session := range s.data.Sessions {
				if session.EndpointID == id {
					sessions = append(sessions, session.ID)
				}
			}
			s.mu.Unlock()
			s.workers.Add(1)
			go func() {
				defer s.workers.Done()
				for packet := range client.Events {
					s.handle(id, current, packet)
				}
			}()
			for _, sessionID := range sessions {
				if s.ctx.Err() == nil {
					_ = s.Restore(s.ctx, sessionID)
				}
			}
			select {
			case <-client.Done:
			case <-s.ctx.Done():
				client.Close()
			}
			s.mu.Lock()
			if s.clients[id] == current {
				delete(s.clients, id)
				s.connectionLostLocked(id, current)
			}
			s.mu.Unlock()
			backoff = time.Second
		} else {
			endpoint.State, endpoint.Error = "reconnecting", err.Error()
			stop := strings.Contains(err.Error(), "authentication") || strings.Contains(err.Error(), "compatible") || strings.Contains(err.Error(), "credential")
			if stop {
				endpoint.State = "auth_required"
				if strings.Contains(err.Error(), "compatible") {
					endpoint.State = "incompatible"
				}
			}
			s.commitLocked("endpoint.updated", id, endpoint)
			s.mu.Unlock()
			if stop {
				return
			}
		}
		delay := time.Duration(float64(backoff) * (0.8 + rand.Float64()*0.4))
		timer := time.NewTimer(delay)
		select {
		case <-s.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		backoff = min(backoff*2, 30*time.Second)
	}
}

func (s *Service) connectionLostLocked(id string, current *connection) {
	endpoint := s.endpoint(id)
	if endpoint.AutoConnect {
		endpoint.State, endpoint.Error = "reconnecting", "Connection lost. Session state will be synchronized before further tasks."
	}
	values := []any{endpoint}
	for _, session := range s.data.Sessions {
		if session.EndpointID != id {
			continue
		}
		session.State = "syncing"
		if session.ActiveTurn != "" {
			if turn := s.turn(session.ActiveTurn); turn != nil {
				turn.State = "unknown"
				values = append(values, turn)
			}
		}
		values = append(values, session)
	}
	for _, interaction := range s.data.Interactions {
		if interaction.Generation == current.generation && (interaction.State == "pending" || interaction.State == "replying") {
			interaction.State = "unknown"
			values = append(values, interaction)
		}
	}
	s.commitLocked("state.updated", id, values...)
}
