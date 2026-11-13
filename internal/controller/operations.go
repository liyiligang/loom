package controller

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/liyiligang/loom/internal/model"
	"github.com/liyiligang/loom/internal/provider"
	"github.com/liyiligang/loom/internal/workspace"
)

type SessionInput struct {
	AccessMode    provider.AccessMode `json:"accessMode"`
	BranchID      string              `json:"branchId"`
	RequirementID string              `json:"requirementId"`
	EndpointID    string              `json:"endpointId"`
	Title         string              `json:"title"`
	Cwd           string              `json:"cwd"`
	Model         string              `json:"model"`
}

func (s *Service) readyLocked(endpointID string) (*connection, error) {
	endpoint := s.endpoint(endpointID)
	if endpoint == nil {
		return nil, problem("not_found", "Service not found.", 404)
	}
	current := s.clients[endpointID]
	if current == nil || endpoint.State != "ready" {
		return nil, problem("agent_offline", "Connect the agent service before performing this action.", 409)
	}
	return current, nil
}

func (s *Service) CreateSession(ctx context.Context, input SessionInput) (string, error) {
	return s.createSession(ctx, input, "")
}

func (s *Service) createSession(ctx context.Context, input SessionInput, purpose string) (string, error) {
	if input.AccessMode != "" && !input.AccessMode.Valid() {
		return "", invalidAccessMode()
	}
	input.AccessMode = input.AccessMode.Effective()
	input.Cwd, input.Title, input.Model = strings.TrimSpace(input.Cwd), strings.TrimSpace(input.Title), strings.TrimSpace(input.Model)
	s.mu.Lock()
	s.creatingSessions[input.RequirementID]++
	defer func() { s.mu.Lock(); s.creatingSessions[input.RequirementID]--; s.mu.Unlock() }()
	endpoint := s.endpoint(input.EndpointID)
	local := endpoint != nil && endpoint.Local
	var binding *workspace.Binding
	var modelSelection *string
	if input.RequirementID != "" {
		requirement, err := s.requirementForEndpointLocked(input.RequirementID, endpoint)
		if err != nil {
			s.mu.Unlock()
			return "", err
		}
		modelSelection = s.scopeModelLocked(requirement, endpoint.Provider)
		if modelSelection != nil {
			input.Model = *modelSelection
		}
		if purpose != "requirement" {
			binding, input.BranchID, err = s.executionWorkspaceLocked(requirement, input.BranchID, input.Cwd)
			if err != nil {
				s.mu.Unlock()
				return "", err
			}
		} else {
			input.AccessMode = requirement.ConversationAccessMode.Effective()
		}
		if binding != nil {
			input.Cwd = binding.Directory
		} else if input.Cwd == "" {
			input.Cwd = s.project(requirement.ProjectID).Cwd
		}
	}
	if local && input.Cwd == "" {
		input.Cwd = s.defaultDirectory
	}
	if input.RequirementID == "" && endpoint != nil {
		for _, project := range s.data.Projects {
			if project.Local == local && (local || project.EndpointID == endpoint.ID) && project.Cwd == projectDirectory(input.Cwd, local) {
				if value, ok := project.Models[endpoint.Provider]; ok {
					modelSelection = &value
					input.Model = value
				}
				break
			}
		}
	}
	s.mu.Unlock()
	if binding != nil {
		if err := workspace.Verify(ctx, *binding); err != nil {
			return "", problem("workspace_unavailable", err.Error(), 409)
		}
	}
	if input.Cwd == "" || len(input.Cwd) > 4096 || strings.ContainsRune(input.Cwd, 0) || (input.Title != "" && !validName(input.Title)) || len(input.Model) > 120 {
		return "", problem("invalid_session", "Enter a working directory on the target machine and a title of at most 120 characters.", 400)
	}
	if local {
		info, err := os.Stat(input.Cwd)
		if err != nil || !filepath.IsAbs(input.Cwd) || !info.IsDir() {
			return "", problem("local_directory", "The project directory is unavailable.", 400)
		}
	}
	defaultTitle := input.Title == ""
	if defaultTitle {
		input.Title = "New session"
	}
	sessionID := model.ID()
	created := false
	defer func() {
		if !created {
			s.credentialBroker.RevokeSession(sessionID)
		}
	}()
	var tools *provider.ToolServer
	if local {
		server := s.credentialBroker.Session(sessionID)
		tools = &server
	}
	s.mu.Lock()
	current, err := s.readyLocked(input.EndpointID)
	s.mu.Unlock()
	if err != nil {
		return "", err
	}
	result, err := current.client.CreateSession(ctx, provider.SessionOptions{Cwd: input.Cwd, Model: input.Model, Tools: tools, AccessMode: input.AccessMode})
	if err != nil {
		return "", err
	}
	if result.ID == "" {
		return "", errors.New("The agent returned no session identifier")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if purpose == "requirement" {
		// A settings update may have arrived while the native session was created.
		input.AccessMode = s.requirement(input.RequirementID).ConversationAccessMode.Effective()
	}
	session := &model.Session{ID: sessionID, BranchID: input.BranchID, Purpose: purpose, EndpointID: input.EndpointID, ProviderID: result.ID,
		Title: input.Title, DefaultTitle: &defaultTitle, Cwd: input.Cwd, Workspace: binding, Model: result.Model, ModelManaged: modelSelection != nil, AccessMode: input.AccessMode, State: "idle", CreatedAt: model.Now(), UpdatedAt: model.Now()}
	var values []any
	if input.RequirementID == "" {
		project, created := s.ensureProjectLocked(endpoint, input.Cwd)
		values = append(values, created...)
		requirement, created := s.defaultRequirementLocked(project)
		values = append(values, created...)
		session.RequirementID = requirement.ID
	} else {
		session.RequirementID = input.RequirementID
	}
	if s.clients[input.EndpointID] != current {
		session.State = "syncing"
	}
	s.data.Sessions = append(s.data.Sessions, session)
	if s.clients[input.EndpointID] == current {
		s.subscribed[session.ID] = current.generation
	}
	if err := s.commitLocked("navigation.updated", session.ID, append(values, session)...); err != nil {
		return "", err
	}
	created = true
	return session.ID, nil
}

func (s *Service) Rename(id, title string) error {
	return s.UpdateSession(id, SessionUpdate{Title: &title})
}

type SessionUpdate struct {
	AccessMode    *provider.AccessMode `json:"accessMode"`
	Title         *string              `json:"title"`
	RequirementID *string              `json:"requirementId"`
}

func (s *Service) UpdateSession(id string, input SessionUpdate) error {
	if input.AccessMode != nil && !input.AccessMode.Valid() {
		return invalidAccessMode()
	}
	if input.Title != nil {
		title := strings.TrimSpace(*input.Title)
		if !validName(title) {
			return problem("invalid_title", "Enter a title of at most 120 characters.", 400)
		}
		input.Title = &title
	}
	if input.Title == nil && input.RequirementID == nil && input.AccessMode == nil {
		return problem("invalid_session_update", "Choose a session title, requirement or access mode to update.", 400)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.session(id)
	if session == nil {
		return problem("not_found", "Session not found.", 404)
	}
	if input.RequirementID != nil {
		if session.Purpose == "requirement" && *input.RequirementID != session.RequirementID {
			return problem("requirement_chat_scope", "A requirement conversation must remain in its owning requirement.", 400)
		}
		if *input.RequirementID != session.RequirementID {
			s.cancelCredentialUsesLocked("", "", session.ID)
		}
		requirement, err := s.requirementForEndpointLocked(*input.RequirementID, s.endpoint(session.EndpointID))
		if err != nil {
			return err
		}
		if requirement.ID != session.RequirementID {
			if session.Workspace != nil {
				return problem("workspace_scope", "Worktree sessions must remain in the requirement that owns their branch.", 400)
			}
			binding, err := s.requirementWorkspaceLocked(requirement, session.Cwd)
			if err != nil {
				return err
			}
			session.Workspace = binding
		}
		session.RequirementID = *input.RequirementID
	}
	if input.Title != nil {
		session.Title = *input.Title
		defaultTitle := false
		session.DefaultTitle = &defaultTitle
	}
	session.UpdatedAt = model.Now()
	kind := "session.updated"
	values := []any{session}
	if input.AccessMode != nil {
		session.AccessMode = *input.AccessMode
		if session.Purpose == "requirement" {
			requirement := s.requirement(session.RequirementID)
			requirement.ConversationAccessMode = *input.AccessMode
			values = append(values, requirement)
			kind = "navigation.updated"
		}
	}
	if input.RequirementID != nil {
		kind = "navigation.updated"
	}
	return s.commitLocked(kind, id, values...)
}

type TurnInput struct {
	Context         string `json:"-"`
	ClientRequestID string `json:"clientRequestId"`
	Text            string `json:"text"`
}

func (s *Service) submittedTurnLocked(id string, input TurnInput) (*model.Turn, error) {
	for _, turn := range s.data.Turns {
		if turn.SessionID == id && turn.ClientRequestID == input.ClientRequestID {
			if turn.Input != input.Text {
				return nil, problem("request_conflict", "This request ID already belongs to a different task.", 409)
			}
			return turn, nil
		}
	}
	return nil, nil
}

func (s *Service) Submit(id string, input TurnInput) (string, error) {
	if strings.TrimSpace(input.Text) == "" || len(input.Text) > 65536 || input.ClientRequestID == "" || len(input.ClientRequestID) > 128 {
		return "", problem("invalid_task", "A task of at most 64 KiB and a clientRequestId are required.", 400)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.session(id)
	if session == nil {
		return "", problem("not_found", "Session not found.", 404)
	}
	if turn, err := s.submittedTurnLocked(id, input); err != nil {
		return "", err
	} else if turn != nil {
		return turn.ID, nil
	}
	if session.Purpose == "requirement" {
		if w := s.workflow(session.RequirementID); w != nil && (w.State == "pending" || w.State == "running") && input.ClientRequestID != w.RequestID {
			return "", problem("workflow_busy", "The preparation workflow owns this conversation. Pause it before sending a separate message.", 409)
		}
	}
	current, err := s.readyLocked(session.EndpointID)
	if err != nil {
		return "", err
	}
	if session.ActiveTurn != "" || session.State != "idle" {
		return "", problem("session_busy", "Wait for the active task or synchronize the session before sending another task.", 409)
	}
	binding, err := s.sessionWorkspaceLocked(session)
	if err != nil {
		return "", err
	}
	if binding != nil {
		requirementID := session.RequirementID
		s.mu.Unlock()
		verification := workspace.Verify(s.ctx, *binding)
		s.mu.Lock()
		if turn, err := s.submittedTurnLocked(id, input); err != nil {
			return "", err
		} else if turn != nil {
			return turn.ID, nil
		}
		if verification != nil {
			return "", problem("workspace_unavailable", verification.Error(), 409)
		}
		if session.RequirementID != requirementID {
			return "", problem("workspace_scope", "Sessions must use their requirement's worktree directory.", 409)
		}
		current, err = s.readyLocked(session.EndpointID)
		if err != nil {
			return "", err
		}
		if session.ActiveTurn != "" || session.State != "idle" {
			return "", problem("session_busy", "Wait for the active task or synchronize the session before sending another task.", 409)
		}
	}
	if err := s.writerAvailableLocked(session); err != nil {
		return "", err
	}
	selection := s.scopeModelLocked(s.requirement(session.RequirementID), s.endpoint(session.EndpointID).Provider)
	if selection == nil && session.ModelManaged {
		value := ""
		selection = &value
	}
	if selection != nil {
		session.Model, session.ModelManaged = *selection, true
	}
	turn := &model.Turn{Model: selection, ID: model.ID(), SessionID: id, ClientRequestID: input.ClientRequestID, Context: input.Context, Input: input.Text, AccessMode: session.AccessMode.Effective(), State: "pending", CreatedAt: model.Now()}
	turn.BaselineKnown = true
	for _, previous := range s.data.Turns {
		if previous.SessionID == id && previous.ProviderID != "" {
			turn.PriorTurnID = previous.ProviderID
		}
	}
	entry := &model.Entry{ID: model.ID(), SessionID: id, TurnID: turn.ID, Kind: "message", Role: "user", Text: input.Text, Status: "completed", CreatedAt: turn.CreatedAt}
	session.ActiveTurn, session.State, session.Error, session.UpdatedAt = turn.ID, "running", "", model.Now()
	s.data.Turns = append(s.data.Turns, turn)
	s.data.Entries = append(s.data.Entries, entry)
	if err := s.commitLocked("session.updated", id, session, turn, entry); err != nil {
		return "", err
	}
	threadID, turnID, prompt := session.ProviderID, turn.ID, turn.Prompt()
	tools := s.credentialToolsLocked(session)
	accessMode, selectedModel, cwd := turn.AccessMode, turn.Model, session.Cwd
	s.workers.Add(1)
	go func() {
		defer s.workers.Done()
		result, err := current.client.StartTurn(s.ctx, threadID, provider.TurnOptions{Text: prompt, Tools: tools, AccessMode: accessMode, Model: selectedModel, Cwd: cwd})
		s.mu.Lock()
		defer s.mu.Unlock()
		turn := s.turn(turnID)
		session := s.session(id)
		if terminal(turn.State) {
			return
		}
		if err != nil || result.ID == "" {
			turn.State, session.State = "unknown", "syncing"
			turn.Error = "Task acknowledgement was lost. Synchronize the session; this task will not be resubmitted automatically."
			var rpcError *provider.Rejection
			if errors.As(err, &rpcError) {
				turn.State, session.State, session.ActiveTurn, turn.Error = "failed", "idle", "", rpcError.Message
			}
			session.Error = turn.Error
		} else if s.clients[session.EndpointID] == current {
			if turn.ProviderID != "" && turn.ProviderID != result.ID {
				turn.State, session.State, turn.Error = "unknown", "syncing", "The agent returned a conflicting task identifier."
			} else {
				turn.ProviderID = result.ID
				if turn.State == "pending" {
					turn.State = "running"
				}
			}
		}
		s.commitLocked("session.updated", id, session, turn)
	}()
	return turn.ID, nil
}

func terminal(state string) bool {
	return state == "completed" || state == "interrupted" || state == "failed"
}

func (s *Service) Interrupt(ctx context.Context, id string) error {
	s.mu.Lock()
	turn := s.turn(id)
	if turn == nil {
		s.mu.Unlock()
		return problem("not_found", "Task not found.", 404)
	}
	session := s.session(turn.SessionID)
	current, err := s.readyLocked(session.EndpointID)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	if terminal(turn.State) {
		s.mu.Unlock()
		return nil
	}
	if turn.ProviderID == "" {
		s.mu.Unlock()
		return problem("sync_required", "Synchronize the session to obtain its active task identifier before interrupting.", 409)
	}
	threadID, providerID := session.ProviderID, turn.ProviderID
	s.mu.Unlock()
	return current.client.Interrupt(ctx, threadID, providerID)
}

func (s *Service) Restore(ctx context.Context, id string) error {
	s.mu.Lock()
	session := s.session(id)
	if session == nil {
		s.mu.Unlock()
		return problem("not_found", "Session not found.", 404)
	}
	current, err := s.readyLocked(session.EndpointID)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	if s.restoring[id] {
		s.mu.Unlock()
		return problem("sync_in_progress", "Session synchronization is already in progress.", 409)
	}
	s.restoring[id] = true
	defer func() { s.mu.Lock(); delete(s.restoring, id); s.mu.Unlock() }()
	threadID, cwd, purpose, sessionModel := session.ProviderID, session.Cwd, session.Purpose, session.Model
	accessMode := session.AccessMode.Effective()
	var activeModel *string
	if active := s.turn(session.ActiveTurn); active != nil {
		accessMode, activeModel = active.AccessMode.Effective(), active.Model
	}
	tools := s.credentialToolsLocked(session)
	subscribed := s.subscribed[id] == current.generation
	empty := true
	for _, turn := range s.data.Turns {
		if turn.SessionID == id {
			empty = false
			break
		}
	}
	session.State, session.Error = "syncing", ""
	if err := s.commitLocked("session.updated", id, session); err != nil {
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	result, err := current.client.SynchronizeSession(ctx, provider.SessionReference{ID: threadID, Cwd: cwd, Subscribed: subscribed, Empty: empty, Tools: tools, AccessMode: accessMode, Model: activeModel})
	if empty && purpose == "requirement" && errors.Is(err, provider.ErrEmptySession) {
		result, err = current.client.CreateSession(ctx, provider.SessionOptions{Cwd: cwd, Model: sessionModel, Tools: tools, AccessMode: accessMode})
		if err == nil && result.ID != "" {
			threadID = result.ID
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.clients[session.EndpointID] != current {
		return problem("agent_offline", "The connection changed while synchronizing this session.", 409)
	}
	if err != nil || result.ID != threadID {
		session.State, session.Error = "syncing", "Session synchronization failed. Check the service and try again."
		if errors.Is(err, provider.ErrEmptySession) {
			session.Error = err.Error()
		}
		s.commitLocked("session.updated", id, session)
		if err != nil {
			return err
		}
		return errors.New("The agent returned a mismatched session identifier")
	}
	s.subscribed[id] = current.generation
	session.ProviderID = threadID
	values := []any{session}
	if intent := s.reconcileIntentLocked(session, result.Turns); intent != nil {
		values = append(values, intent)
	}
	for _, nativeTurn := range result.Turns {
		turn := s.nativeTurnLocked(session, nativeTurn.ID, false)
		if !terminal(turn.State) {
			s.applyTurnLocked(session, turn, nativeTurn)
		}
		values = append(values, turn)
		for _, raw := range nativeTurn.Items {
			if entry := s.itemLocked(session, turn, raw, terminal(nativeTurn.State)); entry != nil {
				values = append(values, entry)
			}
		}
		if terminal(turn.State) {
			values = append(values, s.closeInteractionsLocked(turn.ID)...)
		}
	}
	if session.ActiveTurn == "" {
		session.State, session.Error = "idle", ""
	} else {
		active := s.turn(session.ActiveTurn)
		if active.State == "unknown" {
			session.State, session.Error = "syncing", "The agent has not confirmed this task's outcome. Further submission is blocked to avoid duplicate execution."
		} else {
			session.State, session.Error = "running", ""
		}
	}
	s.refreshWaitingLocked(session)
	if active := s.turn(session.ActiveTurn); active != nil {
		values = append(values, active)
	}
	return s.commitLocked("session.updated", id, values...)
}

type ReplyInput struct {
	Decision string `json:"decision"`
	Answers  map[string]struct {
		Answers []string `json:"answers"`
	} `json:"answers"`
}

func (s *Service) Respond(id string, input ReplyInput) error {
	s.mu.Lock()
	var interaction *model.Interaction
	for _, v := range s.data.Interactions {
		if v.ID == id {
			interaction = v
			break
		}
	}
	if interaction == nil {
		s.mu.Unlock()
		return problem("not_found", "Interaction not found.", 404)
	}
	if interaction.State != "pending" {
		s.mu.Unlock()
		return problem("interaction_closed", "This interaction has already been answered or its connection has expired.", 409)
	}
	session := s.session(interaction.SessionID)
	current, err := s.readyLocked(session.EndpointID)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	if current.generation != interaction.Generation {
		s.mu.Unlock()
		return problem("interaction_expired", "This interaction belongs to an earlier connection.", 409)
	}
	var answer any
	if interaction.Kind == "question" {
		var payload struct {
			Questions []struct {
				ID string `json:"id"`
			} `json:"questions"`
		}
		if json.Unmarshal(interaction.Payload, &payload) != nil || len(payload.Questions) == 0 {
			s.mu.Unlock()
			return problem("invalid_question", "The agent supplied an invalid question.", 400)
		}
		if len(input.Answers) != len(payload.Questions) {
			s.mu.Unlock()
			return problem("invalid_answer", "Answer every question before continuing.", 400)
		}
		for _, question := range payload.Questions {
			response, exists := input.Answers[question.ID]
			if !exists || len(response.Answers) == 0 {
				s.mu.Unlock()
				return problem("invalid_answer", "Answer every question before continuing.", 400)
			}
			for _, text := range response.Answers {
				if strings.TrimSpace(text) == "" || len(text) > 8192 {
					s.mu.Unlock()
					return problem("invalid_answer", "Answers must contain text and fit within 8 KiB.", 400)
				}
			}
		}
		answer = map[string]any{"answers": input.Answers}
	} else {
		if input.Decision != "accept" && input.Decision != "decline" {
			s.mu.Unlock()
			return problem("invalid_decision", "Choose accept or decline for this approval.", 400)
		}
		answer = map[string]any{"decision": input.Decision}
	}
	interaction.State = "replying"
	interaction.Answer, _ = json.Marshal(answer)
	if err := s.commitLocked("session.updated", session.ID, interaction); err != nil {
		s.mu.Unlock()
		return err
	}
	requestID := interaction.RequestID
	s.mu.Unlock()
	err = current.client.Reply(requestID, answer)
	s.mu.Lock()
	defer s.mu.Unlock()
	if interaction.State == "replying" && (err != nil || s.clients[session.EndpointID] != current) {
		interaction.State = "unknown"
	}
	s.refreshWaitingLocked(session)
	values := []any{session, interaction}
	if turn := s.turn(session.ActiveTurn); turn != nil {
		values = append(values, turn)
	}
	if persistErr := s.commitLocked("session.updated", session.ID, values...); persistErr != nil {
		return persistErr
	}
	return err
}

func (s *Service) refreshWaitingLocked(session *model.Session) {
	turn := s.turn(session.ActiveTurn)
	if turn == nil || terminal(turn.State) || turn.State == "unknown" {
		return
	}
	session.State, turn.State = "running", "running"
	for _, interaction := range s.data.Interactions {
		if interaction.TurnID == turn.ID && (interaction.State == "pending" || interaction.State == "replying") {
			if interaction.Kind == "question" {
				session.State, turn.State = "waiting_input", "waiting_input"
			} else {
				session.State, turn.State = "waiting_approval", "waiting_approval"
			}
		}
	}
}
