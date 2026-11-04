package controller

import (
	"encoding/json"
	"strings"

	"github.com/liyiligang/loom/internal/model"
	"github.com/liyiligang/loom/internal/provider/codex"
)

type eventParams struct {
	ThreadID string           `json:"threadId"`
	TurnID   string           `json:"turnId"`
	ItemID   string           `json:"itemId"`
	Delta    string           `json:"delta"`
	Message  string           `json:"message"`
	Turn     codex.NativeTurn `json:"turn"`
	Item     json.RawMessage  `json:"item"`
	Error    *struct {
		Message string `json:"message"`
	} `json:"error"`
	WillRetry bool `json:"willRetry"`
}

func (s *Service) handle(endpointID string, current *connection, packet codex.Packet) {
	var params eventParams
	if json.Unmarshal(packet.Params, &params) != nil {
		if len(packet.ID) > 0 {
			current.client.Reject(packet.ID, "Invalid interaction payload")
		}
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.clients[endpointID] != current {
		return
	}
	session := s.sessionForThread(endpointID, params.ThreadID)
	if session == nil {
		if len(packet.ID) > 0 {
			current.client.Reject(packet.ID, "This session is not controlled by Loom")
		}
		return
	}
	if packet.Method == "serverRequest/resolved" {
		var resolved struct {
			RequestID json.RawMessage `json:"requestId"`
		}
		if json.Unmarshal(packet.Params, &resolved) != nil {
			return
		}
		for _, interaction := range s.data.Interactions {
			if interaction.SessionID == session.ID && interaction.Generation == current.generation && string(interaction.RequestID) == string(resolved.RequestID) {
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
	providerTurn := params.TurnID
	if providerTurn == "" {
		providerTurn = params.Turn.ID
	}
	if providerTurn == "" {
		return
	}
	turn := s.nativeTurnLocked(session, providerTurn, true)
	values := []any{session, turn}
	if len(packet.ID) > 0 {
		kind := ""
		switch packet.Method {
		case "item/commandExecution/requestApproval":
			kind = "command"
		case "item/fileChange/requestApproval":
			kind = "file"
		case "item/tool/requestUserInput":
			kind = "question"
		default:
			current.client.Reject(packet.ID, "This interaction is not supported by Loom")
			return
		}
		if terminal(turn.State) {
			current.client.Reject(packet.ID, "The task has already finished")
			return
		}
		for _, interaction := range s.data.Interactions {
			if interaction.Generation == current.generation && string(interaction.RequestID) == string(packet.ID) {
				return
			}
		}
		interaction := &model.Interaction{ID: model.ID(), SessionID: session.ID, TurnID: turn.ID,
			RequestID: packet.ID, Generation: current.generation, Kind: kind, Payload: packet.Params, State: "pending"}
		s.data.Interactions = append(s.data.Interactions, interaction)
		values = append(values, interaction)
		s.refreshWaitingLocked(session)
	} else {
		switch packet.Method {
		case "turn/started":
			if !terminal(turn.State) {
				turn.State, turn.Error, session.State, session.Error, session.ActiveTurn = "running", "", "running", "", turn.ID
			}
		case "turn/completed":
			s.applyTurnLocked(session, turn, params.Turn)
			for _, raw := range params.Turn.Items {
				if entry := s.itemLocked(session, turn, raw, true); entry != nil {
					values = append(values, entry)
				}
			}
			if terminal(turn.State) {
				values = append(values, s.closeInteractionsLocked(turn.ID)...)
			}
		case "item/started", "item/completed":
			if entry := s.itemLocked(session, turn, params.Item, packet.Method == "item/completed"); entry != nil {
				values = append(values, entry)
			}
		case "item/agentMessage/delta", "item/commandExecution/outputDelta", "item/fileChange/outputDelta", "item/reasoning/summaryTextDelta", "item/reasoning/textDelta", "item/plan/delta", "item/mcpToolCall/progress":
			kind, role := "activity", ""
			if packet.Method == "item/agentMessage/delta" {
				kind, role = "message", "assistant"
			}
			entry := s.entryLocked(session, turn, params.ItemID, kind, role)
			if entry.Status != "completed" {
				delta := params.Delta
				if packet.Method == "item/mcpToolCall/progress" {
					delta = params.Message + "\n"
				}
				entry.Text, entry.Truncated = bounded(entry.Text + delta)
			}
			values = append(values, entry)
		case "error":
			if params.Error != nil {
				turn.Error, session.Error = params.Error.Message, params.Error.Message
			}
			if !params.WillRetry {
				s.applyTurnLocked(session, turn, codex.NativeTurn{ID: providerTurn, Status: "failed"})
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

func (s *Service) applyTurnLocked(session *model.Session, turn *model.Turn, native codex.NativeTurn) {
	if terminal(turn.State) && native.Status == "inProgress" {
		return
	}
	switch native.Status {
	case "completed":
		turn.State = "completed"
	case "interrupted":
		turn.State = "interrupted"
	case "failed":
		turn.State = "failed"
	case "inProgress":
		turn.State = "running"
	default:
		return
	}
	if native.Error != nil {
		turn.Error = native.Error.Message
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
func (s *Service) reconcileIntentLocked(session *model.Session, native []codex.NativeTurn) *model.Turn {
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
	for _, raw := range candidate.Items {
		var item struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		}
		if json.Unmarshal(raw, &item) != nil || item.Type != "userMessage" {
			continue
		}
		for _, content := range item.Content {
			if content.Type == "text" {
				text.WriteString(content.Text)
			}
		}
	}
	if candidate.ID == "" || text.String() != intent.Input {
		return nil
	}
	intent.ProviderID, intent.Error = candidate.ID, ""
	return intent
}

func (s *Service) itemLocked(session *model.Session, turn *model.Turn, raw json.RawMessage, completed bool) *model.Entry {
	var item struct {
		ID               string  `json:"id"`
		Type             string  `json:"type"`
		Text             string  `json:"text"`
		Command          string  `json:"command"`
		AggregatedOutput *string `json:"aggregatedOutput"`
		Content          []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(raw, &item) != nil || item.ID == "" {
		return nil
	}
	kind, role := "activity", ""
	switch item.Type {
	case "agentMessage":
		kind, role = "message", "assistant"
	case "userMessage":
		kind, role = "message", "user"
	}
	entry := s.entryLocked(session, turn, item.ID, kind, role)
	if !completed && entry.Status == "completed" {
		return entry
	}
	entry.Kind, entry.Role = kind, role
	if kind == "message" {
		text := item.Text
		if role == "user" {
			for _, content := range item.Content {
				if content.Type == "text" {
					text += content.Text
				}
			}
			turn.Input = text
		}
		newerPrefix := !completed && len(entry.Text) > len(text) && strings.HasPrefix(entry.Text, text)
		if !newerPrefix && (completed || text != "") {
			entry.Text, entry.Truncated = bounded(text)
		}
	} else {
		newerPrefix := item.AggregatedOutput != nil && !completed && len(entry.Text) > len(*item.AggregatedOutput) && strings.HasPrefix(entry.Text, *item.AggregatedOutput)
		if item.AggregatedOutput != nil && *item.AggregatedOutput != "" && !newerPrefix {
			entry.Text, entry.Truncated = bounded(*item.AggregatedOutput)
		}
		if len(raw) <= 1<<20 {
			entry.Detail = raw
		}
	}
	if completed {
		entry.Status = "completed"
	}
	return entry
}
