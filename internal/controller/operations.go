package controller

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/liyiligang/loom/internal/model"
	"github.com/liyiligang/loom/internal/provider/codex"
)

type SessionInput struct {
	EndpointID string `json:"endpointId"`
	Title      string `json:"title"`
	Cwd        string `json:"cwd"`
	Model      string `json:"model"`
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
	input.Cwd, input.Title, input.Model = strings.TrimSpace(input.Cwd), strings.TrimSpace(input.Title), strings.TrimSpace(input.Model)
	if input.Cwd == "" || len(input.Cwd) > 4096 || strings.ContainsRune(input.Cwd, 0) || len(input.Title) > 120 || len(input.Model) > 120 {
		return "", problem("invalid_session", "Enter a working directory on the target machine and a title of at most 120 characters.", 400)
	}
	if input.Title == "" {
		input.Title = "New session"
	}
	s.mu.Lock()
	current, err := s.readyLocked(input.EndpointID)
	s.mu.Unlock()
	if err != nil {
		return "", err
	}
	params := map[string]any{"cwd": input.Cwd, "approvalPolicy": "on-request", "sandbox": "workspace-write", "ephemeral": false}
	if input.Model != "" {
		params["model"] = input.Model
	}
	var result codex.ThreadResult
	if err := current.client.Call(ctx, "thread/start", params, &result); err != nil {
		return "", err
	}
	if result.Thread.ID == "" {
		return "", errors.New("Codex returned no session identifier")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session := &model.Session{ID: model.ID(), EndpointID: input.EndpointID, ProviderID: result.Thread.ID,
		Title: input.Title, Cwd: input.Cwd, Model: result.Model, State: "idle", CreatedAt: model.Now(), UpdatedAt: model.Now()}
	if s.clients[input.EndpointID] != current {
		session.State = "syncing"
	}
	s.data.Sessions = append(s.data.Sessions, session)
	if s.clients[input.EndpointID] == current {
		s.subscribed[session.ID] = current.generation
	}
	if err := s.commitLocked("session.updated", session.ID, session); err != nil {
		return "", err
	}
	return session.ID, nil
}

func (s *Service) Rename(id, title string) error {
	title = strings.TrimSpace(title)
	if title == "" || len(title) > 120 {
		return problem("invalid_title", "Enter a title of at most 120 characters.", 400)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session := s.session(id)
	if session == nil {
		return problem("not_found", "Session not found.", 404)
	}
	session.Title, session.UpdatedAt = title, model.Now()
	return s.commitLocked("session.updated", id, session)
}

type TurnInput struct {
	ClientRequestID string `json:"clientRequestId"`
	Text            string `json:"text"`
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
	for _, turn := range s.data.Turns {
		if turn.SessionID == id && turn.ClientRequestID == input.ClientRequestID {
			if turn.Input != input.Text {
				return "", problem("request_conflict", "This request ID already belongs to a different task.", 409)
			}
			return turn.ID, nil
		}
	}
	current, err := s.readyLocked(session.EndpointID)
	if err != nil {
		return "", err
	}
	if session.ActiveTurn != "" || session.State != "idle" {
		return "", problem("session_busy", "Wait for the active task or synchronize the session before sending another task.", 409)
	}
	turn := &model.Turn{ID: model.ID(), SessionID: id, ClientRequestID: input.ClientRequestID, Input: input.Text, State: "pending", CreatedAt: model.Now()}
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
	threadID, turnID := session.ProviderID, turn.ID
	s.workers.Add(1)
	go func() {
		defer s.workers.Done()
		var result struct {
			Turn codex.NativeTurn `json:"turn"`
		}
		err := current.client.Call(s.ctx, "turn/start", map[string]any{"threadId": threadID,
			"input": []map[string]any{{"type": "text", "text": input.Text, "text_elements": []any{}}}}, &result)
		s.mu.Lock()
		defer s.mu.Unlock()
		turn := s.turn(turnID)
		session := s.session(id)
		if terminal(turn.State) {
			return
		}
		if err != nil || result.Turn.ID == "" {
			turn.State, session.State = "unknown", "syncing"
			turn.Error = "Task acknowledgement was lost. Synchronize the session; this task will not be resubmitted automatically."
			var rpcError *codex.RPCError
			if errors.As(err, &rpcError) {
				turn.State, session.State, session.ActiveTurn, turn.Error = "failed", "idle", "", rpcError.Message
			}
			session.Error = turn.Error
		} else if s.clients[session.EndpointID] == current {
			if turn.ProviderID != "" && turn.ProviderID != result.Turn.ID {
				turn.State, session.State, turn.Error = "unknown", "syncing", "The agent returned a conflicting task identifier."
			} else {
				turn.ProviderID = result.Turn.ID
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
	return current.client.Call(ctx, "turn/interrupt", map[string]any{"threadId": threadID, "turnId": providerID}, nil)
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
	threadID, cwd := session.ProviderID, session.Cwd
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
	var result codex.ThreadResult
	if subscribed {
		err = current.client.Call(ctx, "thread/read", map[string]any{"threadId": threadID, "includeTurns": !empty}, &result)
	} else {
		err = current.client.Call(ctx, "thread/resume", map[string]any{"threadId": threadID, "cwd": cwd,
			"approvalPolicy": "on-request", "sandbox": "workspace-write"}, &result)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.clients[session.EndpointID] != current {
		return problem("agent_offline", "The connection changed while synchronizing this session.", 409)
	}
	if err != nil || result.Thread.ID != threadID {
		session.State, session.Error = "syncing", "Session synchronization failed. Check the service and try again."
		if err != nil && strings.Contains(err.Error(), "no rollout found") && empty {
			session.Error = "Codex cannot restore an empty session after its connection closes. Create a new session."
		}
		s.commitLocked("session.updated", id, session)
		if err != nil {
			return err
		}
		return errors.New("Codex returned a mismatched session identifier")
	}
	s.subscribed[id] = current.generation
	values := []any{session}
	if intent := s.reconcileIntentLocked(session, result.Thread.Turns); intent != nil {
		values = append(values, intent)
	}
	for _, nativeTurn := range result.Thread.Turns {
		turn := s.nativeTurnLocked(session, nativeTurn.ID, false)
		if !terminal(turn.State) {
			s.applyTurnLocked(session, turn, nativeTurn)
		}
		values = append(values, turn)
		for _, raw := range nativeTurn.Items {
			if entry := s.itemLocked(session, turn, raw, terminal(nativeTurn.Status)); entry != nil {
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
