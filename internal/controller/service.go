package controller

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/liyiligang/loom/internal/credentials"
	"github.com/liyiligang/loom/internal/credentialtool"
	"github.com/liyiligang/loom/internal/local"
	"github.com/liyiligang/loom/internal/model"
	"github.com/liyiligang/loom/internal/provider"
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
	client     provider.Connection
	generation string
}

type Service struct {
	localManager         local.Manager
	localAgents          []local.Agent
	localState           string
	defaultDirectory     string
	mu                   sync.Mutex
	ctx                  context.Context
	cancel               context.CancelFunc
	workers              sync.WaitGroup
	store                *storage.Store
	secrets              *credentials.Store
	vault                *credentials.Vault
	credentialBroker     *credentialtool.Broker
	credentialUses       map[string]credentialUse
	data                 model.Data
	clients              map[string]*connection
	attempts             map[string]bool
	localRetry           map[string]bool
	subscribed           map[string]string
	restoring            map[string]bool
	changed              chan struct{}
	cursor               int64
	fault                error
	Fatal                chan error
	revisions            map[string]int64
	preparingWorkspaces  map[string]bool
	creatingSessions     map[string]int
	requirementChatLocks sync.Map
	workflowLocks        sync.Map
	materialLocks        sync.Map
	skillLocks           sync.Map
	materialWrites       map[string]int
}

func New(ctx context.Context, store *storage.Store, secrets *credentials.Store, options ...Option) (*Service, error) {
	data, err := store.Load()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	s := &Service{ctx: ctx, cancel: cancel, store: store, secrets: secrets, data: data,
		clients: make(map[string]*connection), attempts: make(map[string]bool), localRetry: make(map[string]bool), subscribed: make(map[string]string), restoring: make(map[string]bool), changed: make(chan struct{}), Fatal: make(chan error, 1)}
	s.localManager = local.New(store.Directory())
	vaultDirectory := filepath.Join(store.Directory(), "credentials")
	if len(data.Secrets) != 0 {
		if _, err := os.Lstat(filepath.Join(vaultDirectory, "vault.key")); err != nil {
			cancel()
			return nil, errors.New("Credential storage key is unavailable. Restore it with its matching database backup.")
		}
	}
	s.vault, err = credentials.NewVault(vaultDirectory)
	if err != nil {
		cancel()
		return nil, err
	}
	s.preparingWorkspaces = make(map[string]bool)
	s.creatingSessions = make(map[string]int)
	s.materialWrites = make(map[string]int)
	s.localState = "pending"
	s.localAgents = []local.Agent{}
	s.defaultDirectory, _ = os.Getwd()
	for _, option := range options {
		option(s)
	}
	s.cursor, err = store.Cursor()
	if err != nil {
		cancel()
		return nil, err
	}
	recovered, err := s.backfillNavigation()
	if err != nil {
		cancel()
		return nil, err
	}
	s.backfillSessionWorkspaces()
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
	s.revisions = make(map[string]int64)
	for _, turn := range data.Turns {
		s.revisions[turn.ID] = s.cursor
	}
	for _, entry := range data.Entries {
		s.revisions[entry.ID] = s.cursor
	}
	for _, interaction := range data.Interactions {
		s.revisions[interaction.ID] = s.cursor
	}
	s.credentialUses = make(map[string]credentialUse)
	s.credentialBroker, err = credentialtool.New(s, nil)
	if err != nil {
		cancel()
		return nil, err
	}
	return s, nil
}

func (s *Service) Start() {
	s.ScanLocal()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.workers.Add(1)
	go s.runRequirementWorkflows()
	for _, requirement := range s.data.Requirements {
		s.prepareWorkspaceLocked(requirement.ID)
	}
	for _, endpoint := range s.data.Endpoints {
		if endpoint.AutoConnect && !endpoint.Local {
			s.connectLocked(endpoint)
		}
	}
}

func (s *Service) Close() {
	s.cancel()
	s.credentialBroker.Close()
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
	for _, value := range values {
		switch value := value.(type) {
		case *model.Turn:
			s.revisions[value.ID] = cursor
		case *model.Entry:
			s.revisions[value.ID] = cursor
		case *model.Interaction:
			s.revisions[value.ID] = cursor
		}
	}
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
		"modelCatalog": s.data.ModelCatalog,
		"endpoints":    append([]*model.Endpoint{}, s.data.Endpoints...), "sessions": sessions,
		"projects": append([]*model.Project{}, s.data.Projects...), "requirements": append([]*model.Requirement{}, s.data.Requirements...),
		"workflows":    append([]*model.RequirementWorkflow{}, s.data.Workflows...),
		"writers":      s.writersLocked(),
		"local":        LocalSnapshot{State: s.localState, Confirmed: s.data.LocalSetup.Confirmed, Agents: s.localAgents, DefaultDirectory: s.defaultDirectory},
		"capabilities": map[string]bool{"codex": true, "claude": true, "mdns": false}})
}

func (s *Service) Detail(id string) ([]byte, error) {
	return s.detail(id, -1)
}

// Changes returns current projections touched after a committed cursor. Revisions
// are rebuilt on restart, so pre-restart subscribers receive a complete refresh.
func (s *Service) Changes(id string, after int64) ([]byte, error) {
	return s.detail(id, after)
}

func (s *Service) detail(id string, after int64) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if after > s.cursor {
		return nil, problem("invalid_cursor", "The cursor is ahead of the controller. Reload the session.", 409)
	}
	session := s.session(id)
	if session == nil {
		return nil, problem("not_found", "Session not found.", 404)
	}
	detail := model.Detail{Session: session, Turns: []*model.Turn{}, Entries: []*model.Entry{}, Interactions: []*model.Interaction{}}
	for _, turn := range s.data.Turns {
		if turn.SessionID == id && (after < 0 || s.revisions[turn.ID] > after) {
			detail.Turns = append(detail.Turns, turn)
		}
	}
	for _, entry := range s.data.Entries {
		if entry.SessionID == id && (after < 0 || s.revisions[entry.ID] > after) {
			detail.Entries = append(detail.Entries, entry)
		}
	}
	for _, interaction := range s.data.Interactions {
		if interaction.SessionID == id && (after < 0 || s.revisions[interaction.ID] > after) {
			detail.Interactions = append(detail.Interactions, interaction)
		}
	}
	return json.Marshal(struct {
		model.Detail
		Cursor  int64 `json:"cursor"`
		Partial bool  `json:"partial"`
	}{detail, s.cursor, after >= 0})
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
	if endpoint != nil && endpoint.Local {
		return "", problem("local_managed", "Local agent connections are managed automatically. Detect installed agents again to update them.", 409)
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
		if endpoint.Local && s.clients[endpoint.ID] == nil && s.ctx.Err() == nil {
			s.localRetry[endpoint.ID] = true
		}
		return
	}
	s.attempts[endpoint.ID] = true
	s.workers.Add(1)
	go s.connectionLoop(endpoint.ID)
}

func (s *Service) connectionLoop(id string) {
	defer s.workers.Done()
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.attempts, id)
		if s.localRetry[id] {
			delete(s.localRetry, id)
			if endpoint := s.endpoint(id); endpoint != nil && endpoint.AutoConnect {
				s.connectLocked(endpoint)
			}
		}
	}()
	backoff := time.Second
	for s.ctx.Err() == nil {
		s.mu.Lock()
		endpoint := s.endpoint(id)
		if !endpoint.AutoConnect {
			s.mu.Unlock()
			return
		}
		address := endpoint.Address
		isLocal := endpoint.Local
		agent, available := s.localAgentLocked(endpoint.Provider)
		endpoint.State, endpoint.Error = "connecting", ""
		if s.commitLocked("endpoint.updated", id, endpoint) != nil {
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
		var client provider.Connection
		var err error
		if isLocal {
			if !available {
				err = errors.New("The local agent is unavailable. Detect installed agents again.")
			} else {
				client, err = s.localManager.Connect(s.ctx, agent)
			}
		} else {
			token, credentialError := s.secrets.Read(id)
			err = credentialError
			if err == nil {
				ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
				client, err = codex.Dial(ctx, address, token)
				cancel()
			}
		}
		if err != nil {
			// Concrete adapters return typed nil pointers on startup failure.
			client = nil
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
			endpoint.State, endpoint.Version, endpoint.Error = "ready", client.Version(), ""
			s.commitLocked("endpoint.updated", id, endpoint)
			var sessions []string
			for _, session := range s.data.Sessions {
				if session.EndpointID == id {
					sessions = append(sessions, session.ID)
				}
			}
			s.mu.Unlock()
			s.workers.Add(1)
			eventsDone := make(chan struct{})
			go func() {
				defer s.workers.Done()
				defer close(eventsDone)
				s.consume(id, current)
			}()
			for _, sessionID := range sessions {
				if s.ctx.Err() == nil {
					_ = s.Restore(s.ctx, sessionID)
				}
			}
			select {
			case <-client.Done():
			case <-s.ctx.Done():
				client.Close()
			}
			<-eventsDone
			s.mu.Lock()
			if s.clients[id] == current {
				delete(s.clients, id)
				s.connectionLostLocked(id, current)
			}
			s.mu.Unlock()
			backoff = time.Second
		} else {
			endpoint.State, endpoint.Error = "reconnecting", err.Error()
			stop := isLocal || strings.Contains(err.Error(), "authentication") || strings.Contains(err.Error(), "compatible") || strings.Contains(err.Error(), "credential")
			if stop {
				endpoint.State = "auth_required"
				if isLocal {
					endpoint.State = "unavailable"
				}
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
