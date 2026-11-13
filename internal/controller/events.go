package controller

import (
	"strings"

	"github.com/liyiligang/loom/internal/model"
	"github.com/liyiligang/loom/internal/provider"
)

func (s *Service) handle(endpointID string, current *connection, event provider.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.clients[endpointID] != current {
		return
	}
	session := s.sessionForThread(endpointID, event.SessionID)
	if session == nil {
		if len(event.RequestID) > 0 {
			_ = current.client.Reject(event.RequestID, "This session is not controlled by Loom")
		}
		return
	}
	if event.Kind == provider.InteractionResolved {
		for _, interaction := range s.data.Interactions {
			if interaction.SessionID == session.ID && interaction.Generation == current.generation && string(interaction.RequestID) == string(event.ResolvedID) {
				if interaction.State == "replying" {
					interaction.State = "resolved"
				} else if interaction.State == "pending" {
					interaction.State = "cancelled"
				} else {
					return
				}
				s.refreshWaitingLocked(session)
				values := []any{session, interaction}
				if active := s.turn(session.ActiveTurn); active != nil {
					values = append(values, active)
				}
				s.commitLocked("session.updated", session.ID, values...)
				return
			}
		}
		return
	}
	if event.TurnID == "" {
		return
	}
	turn := s.nativeTurnLocked(session, event.TurnID, true)
	values := []any{session, turn}
	if len(event.RequestID) > 0 {
		if terminal(turn.State) {
			_ = current.client.Reject(event.RequestID, "The task has already finished")
			return
		}
		for _, interaction := range s.data.Interactions {
			if interaction.Generation == current.generation && string(interaction.RequestID) == string(event.RequestID) {
				return
			}
		}
		interaction := &model.Interaction{ID: model.ID(), SessionID: session.ID, TurnID: turn.ID,
			RequestID: event.RequestID, Generation: current.generation, Kind: event.InteractionKind, Payload: event.Payload, State: "pending"}
		s.data.Interactions = append(s.data.Interactions, interaction)
		values = append(values, interaction)
		s.refreshWaitingLocked(session)
	} else {
		switch event.Kind {
		case provider.TurnStarted:
			if !terminal(turn.State) {
				turn.State, turn.Error, session.State, session.Error, session.ActiveTurn = "running", "", "running", "", turn.ID
			}
		case provider.TurnCompleted:
			s.applyTurnLocked(session, turn, event.Turn)
			for _, item := range event.Turn.Items {
				if entry := s.itemLocked(session, turn, item, true); entry != nil {
					values = append(values, entry)
				}
			}
			if terminal(turn.State) {
				values = append(values, s.closeInteractionsLocked(turn.ID)...)
			}
		case provider.ItemStarted, provider.ItemCompleted:
			if entry := s.itemLocked(session, turn, event.Item, event.Kind == provider.ItemCompleted); entry != nil {
				values = append(values, entry)
			}
		case provider.TextDelta, provider.OutputDelta:
			kind, role := "activity", ""
			if event.Kind == provider.TextDelta {
				kind, role = "message", "assistant"
			}
			entry := s.entryLocked(session, turn, event.ItemID, kind, role)
			if entry.Status != "completed" {
				entry.Text, entry.Truncated = bounded(entry.Text + event.Delta)
			}
			values = append(values, entry)
		case provider.ExecutionError:
			turn.Error, session.Error = event.Error, event.Error
			if !event.WillRetry {
				s.applyTurnLocked(session, turn, provider.TurnSnapshot{ID: event.TurnID, State: "failed"})
				values = append(values, s.closeInteractionsLocked(turn.ID)...)
			}
		default:
			return
		}
	}
	session.UpdatedAt = model.Now()
	s.commitLocked("session.updated", session.ID, values...)
}

func (s *Service) nativeTurnLocked(session *model.Session, providerID string, bindPending bool) *model.Turn {
	for _, turn := range s.data.Turns {
		if turn.SessionID == session.ID && turn.ProviderID == providerID {
			return turn
		}
	}
	if active := s.turn(session.ActiveTurn); bindPending && active != nil && active.ProviderID == "" && (active.State == "pending" || active.State == "unknown") {
		active.ProviderID = providerID
		return active
	}
	turn := &model.Turn{ID: model.ID(), SessionID: session.ID, ProviderID: providerID, State: "running", CreatedAt: model.Now()}
	s.data.Turns = append(s.data.Turns, turn)
	return turn
}

func (s *Service) closeInteractionsLocked(turnID string) []any {
	var values []any
	for _, interaction := range s.data.Interactions {
		if interaction.TurnID == turnID && (interaction.State == "pending" || interaction.State == "replying" || interaction.State == "unknown") {
			interaction.State = "cancelled"
			values = append(values, interaction)
		}
	}
	return values
}

func (s *Service) applyTurnLocked(session *model.Session, turn *model.Turn, native provider.TurnSnapshot) {
	if terminal(native.State) {
		s.cancelCredentialUsesLocked("", "", session.ID)
	}
	if terminal(turn.State) && native.State == "running" {
		return
	}
	switch native.State {
	case "completed":
		turn.State = "completed"
	case "interrupted":
		turn.State = "interrupted"
	case "failed":
		turn.State = "failed"
	case "running":
		turn.State = "running"
	case "unknown":
		turn.State, turn.Error = "unknown", native.Error
		session.ActiveTurn, session.State, session.Error = turn.ID, "syncing", "The agent has not confirmed this task's outcome. Further submission is blocked to avoid duplicate execution."
		return
	default:
		return
	}
	if native.Error != "" {
		turn.Error = native.Error
	}
	if terminal(turn.State) {
		if session.ActiveTurn == turn.ID {
			session.ActiveTurn, session.State, session.Error = "", "idle", turn.Error
		}
	} else {
		session.ActiveTurn, session.State = turn.ID, "running"
	}
}

func (s *Service) entryLocked(session *model.Session, turn *model.Turn, providerID, kind, role string) *model.Entry {
	for _, entry := range s.data.Entries {
		if entry.TurnID == turn.ID && entry.ProviderID == providerID && providerID != "" {
			return entry
		}
		if role == "user" && entry.TurnID == turn.ID && entry.Role == "user" && entry.ProviderID == "" {
			entry.ProviderID = providerID
			return entry
		}
	}
	entry := &model.Entry{ID: model.ID(), SessionID: session.ID, TurnID: turn.ID, ProviderID: providerID,
		Kind: kind, Role: role, Status: "running", CreatedAt: model.Now()}
	s.data.Entries = append(s.data.Entries, entry)
	return entry
}

func bounded(text string) (string, bool) {
	const limit = 1 << 20
	if len(text) > limit {
		return strings.ToValidUTF8(text[:limit], ""), true
	}
	return text, false
}

// A Loom-owned session has one writer. Reconcile only one new native turn whose
// user input matches the persisted intent after the last observed native turn.
func (s *Service) reconcileIntentLocked(session *model.Session, native []provider.TurnSnapshot) *model.Turn {
	intent := s.turn(session.ActiveTurn)
	if intent == nil || intent.ProviderID != "" || !intent.BaselineKnown {
		return nil
	}
	start := 0
	if intent.PriorTurnID != "" {
		start = -1
		for index, turn := range native {
			if turn.ID == intent.PriorTurnID {
				start = index + 1
				break
			}
		}
		if start < 0 {
			return nil
		}
	}
	if len(native)-start != 1 {
		return nil
	}
	candidate := native[start]
	var text strings.Builder
	for _, item := range candidate.Items {
		if item.Role == "user" {
			text.WriteString(item.Text)
		}
	}
	if candidate.ID == "" || text.String() != intent.Prompt() {
		return nil
	}
	intent.ProviderID, intent.Error = candidate.ID, ""
	return intent
}

func (s *Service) itemLocked(session *model.Session, turn *model.Turn, item provider.Item, completed bool) *model.Entry {
	if item.ID == "" {
		return nil
	}
	if item.Role == "user" && turn.Context != "" && item.Text == turn.Prompt() {
		item.Text = turn.Input
	}
	entry := s.entryLocked(session, turn, item.ID, item.Kind, item.Role)
	if !completed && entry.Status == "completed" {
		return entry
	}
	entry.Kind, entry.Role = item.Kind, item.Role
	if item.Kind == "message" {
		if item.Role == "user" {
			turn.Input = item.Text
		}
		newerPrefix := !completed && len(entry.Text) > len(item.Text) && strings.HasPrefix(entry.Text, item.Text)
		if !newerPrefix && (completed || item.Text != "") {
			entry.Text, entry.Truncated = bounded(item.Text)
		}
	} else {
		newerPrefix := item.Output != nil && !completed && len(entry.Text) > len(*item.Output) && strings.HasPrefix(entry.Text, *item.Output)
		if item.Output != nil && *item.Output != "" && !newerPrefix {
			entry.Text, entry.Truncated = bounded(*item.Output)
		}
		if len(item.Detail) <= 1<<20 {
			entry.Detail = item.Detail
		}
	}
	if completed {
		entry.Status = "completed"
	}
	return entry
}
