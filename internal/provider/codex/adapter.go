package codex

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/liyiligang/loom/internal/provider"
)

var _ provider.Connection = (*Client)(nil)

func (c *Client) Version() string { return c.version }

func (c *Client) CreateSession(ctx context.Context, options provider.SessionOptions) (provider.SessionSnapshot, error) {
	approval, sandbox := accessSettings(options.AccessMode)
	params := map[string]any{"cwd": options.Cwd, "approvalPolicy": approval, "sandbox": sandbox, "ephemeral": false}
	if options.Model != "" {
		params["model"] = options.Model
	}
	if options.Tools != nil {
		params["config"] = credentialConfig(*options.Tools)
	}
	var result nativeThreadResult
	err := c.Call(ctx, "thread/start", params, &result)
	if err == nil && options.Tools != nil {
		c.rememberTools(result.Thread.ID, *options.Tools)
	}
	return sessionSnapshot(result), err
}

func (c *Client) SynchronizeSession(ctx context.Context, ref provider.SessionReference) (provider.SessionSnapshot, error) {
	method := "thread/resume"
	approval, sandbox := accessSettings(ref.AccessMode)
	params := map[string]any{"threadId": ref.ID, "cwd": ref.Cwd, "approvalPolicy": approval, "sandbox": sandbox}
	if ref.Subscribed {
		method = "thread/read"
		params = map[string]any{"threadId": ref.ID, "includeTurns": !ref.Empty}
	}
	var result nativeThreadResult
	if method == "thread/resume" && ref.Model != nil {
		selected, err := c.selectedModel(ctx, ref.Cwd, *ref.Model)
		if err != nil {
			return provider.SessionSnapshot{}, err
		}
		params["model"] = selected
	}
	if ref.Tools != nil && method == "thread/resume" {
		params["config"] = credentialConfig(*ref.Tools)
	}
	err := c.Call(ctx, method, params, &result)
	if err != nil && ref.Empty && strings.Contains(err.Error(), "no rollout found") {
		return provider.SessionSnapshot{}, provider.ErrEmptySession
	}
	if err == nil && ref.Tools != nil && method == "thread/resume" {
		c.rememberTools(ref.ID, *ref.Tools)
	}
	return sessionSnapshot(result), err
}

func (c *Client) StartTurn(ctx context.Context, sessionID string, options provider.TurnOptions) (provider.TurnSnapshot, error) {
	if options.Tools != nil && !c.hasTools(sessionID, *options.Tools) {
		if err := c.Call(ctx, "thread/resume", map[string]any{"threadId": sessionID, "config": credentialConfig(*options.Tools), "excludeTurns": true}, nil); err != nil {
			return provider.TurnSnapshot{}, &provider.Rejection{Code: -1, Message: "Could not configure project credential tools. Synchronize the session and retry."}
		}
		c.rememberTools(sessionID, *options.Tools)
	}
	var result struct {
		Turn nativeTurn `json:"turn"`
	}
	approval, sandbox := accessSettings(options.AccessMode)
	policy := map[string]any{"type": "workspaceWrite"}
	if sandbox == "danger-full-access" {
		policy["type"] = "dangerFullAccess"
	}
	params := map[string]any{"threadId": sessionID,
		"approvalPolicy": approval, "sandboxPolicy": policy,
		"input": []map[string]any{{"type": "text", "text": options.Text, "text_elements": []any{}}}}
	if options.Model != nil {
		selected, err := c.selectedModel(ctx, options.Cwd, *options.Model)
		if err != nil {
			return provider.TurnSnapshot{}, &provider.Rejection{Code: -1, Message: "Could not resolve the default model. Choose an explicit model and retry."}
		}
		params["model"] = selected
	}
	err := c.Call(ctx, "turn/start", params, &result)
	return turnSnapshot(result.Turn), err
}

func accessSettings(mode provider.AccessMode) (approval, sandbox string) {
	if mode.Effective() == provider.AccessFull {
		return "never", "danger-full-access"
	}
	return "on-request", "workspace-write"
}

func credentialConfig(server provider.ToolServer) map[string]any {
	return map[string]any{"mcp_servers.loom_credentials": map[string]any{"url": server.URL, "http_headers": map[string]string{"Authorization": "Bearer " + server.Token}, "startup_timeout_sec": 10, "tool_timeout_sec": 60, "enabled": true}}
}
func (c *Client) rememberTools(id string, tools provider.ToolServer) {
	c.toolsMu.Lock()
	defer c.toolsMu.Unlock()
	c.tools[id] = tools
}
func (c *Client) hasTools(id string, tools provider.ToolServer) bool {
	c.toolsMu.Lock()
	defer c.toolsMu.Unlock()
	return c.tools[id] == tools
}

func (c *Client) Interrupt(ctx context.Context, sessionID, turnID string) error {
	return c.Call(ctx, "turn/interrupt", map[string]any{"threadId": sessionID, "turnId": turnID}, nil)
}

func sessionSnapshot(result nativeThreadResult) provider.SessionSnapshot {
	snapshot := provider.SessionSnapshot{ID: result.Thread.ID, Model: result.Model}
	for _, turn := range result.Thread.Turns {
		snapshot.Turns = append(snapshot.Turns, turnSnapshot(turn))
	}
	return snapshot
}

func turnSnapshot(turn nativeTurn) provider.TurnSnapshot {
	snapshot := provider.TurnSnapshot{ID: turn.ID, State: turn.Status}
	if snapshot.State == "inProgress" {
		snapshot.State = "running"
	}
	if turn.Error != nil {
		snapshot.Error = turn.Error.Message
	}
	for _, raw := range turn.Items {
		if item := itemSnapshot(raw); item.ID != "" {
			snapshot.Items = append(snapshot.Items, item)
		}
	}
	return snapshot
}

func itemSnapshot(raw json.RawMessage) provider.Item {
	var native struct {
		ID               string  `json:"id"`
		Type             string  `json:"type"`
		Text             string  `json:"text"`
		AggregatedOutput *string `json:"aggregatedOutput"`
		Content          []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(raw, &native) != nil {
		return provider.Item{}
	}
	item := provider.Item{ID: native.ID, Kind: "activity", Output: native.AggregatedOutput, Detail: raw}
	switch native.Type {
	case "agentMessage":
		item.Kind, item.Role, item.Text = "message", "assistant", native.Text
	case "userMessage":
		item.Kind, item.Role = "message", "user"
		var text strings.Builder
		text.WriteString(native.Text)
		for _, content := range native.Content {
			if content.Type == "text" {
				text.WriteString(content.Text)
			}
		}
		item.Text = text.String()
	}
	return item
}

func (c *Client) event(packet packet) (provider.Event, bool) {
	var params struct {
		ThreadID  string          `json:"threadId"`
		TurnID    string          `json:"turnId"`
		ItemID    string          `json:"itemId"`
		Delta     string          `json:"delta"`
		Message   string          `json:"message"`
		Turn      nativeTurn      `json:"turn"`
		Item      json.RawMessage `json:"item"`
		RequestID json.RawMessage `json:"requestId"`
		Error     *struct {
			Message string `json:"message"`
		} `json:"error"`
		WillRetry bool `json:"willRetry"`
	}
	if json.Unmarshal(packet.Params, &params) != nil {
		if len(packet.ID) > 0 {
			_ = c.Reject(packet.ID, "Invalid interaction payload")
		}
		return provider.Event{}, false
	}
	event := provider.Event{SessionID: params.ThreadID, TurnID: params.TurnID, ItemID: params.ItemID,
		Delta: params.Delta, RequestID: packet.ID, Payload: packet.Params, WillRetry: params.WillRetry}
	if event.TurnID == "" {
		event.TurnID = params.Turn.ID
	}
	switch packet.Method {
	case "turn/started":
		event.Kind = provider.TurnStarted
	case "turn/completed":
		event.Kind, event.Turn = provider.TurnCompleted, turnSnapshot(params.Turn)
	case "item/started":
		event.Kind, event.Item = provider.ItemStarted, itemSnapshot(params.Item)
	case "item/completed":
		event.Kind, event.Item = provider.ItemCompleted, itemSnapshot(params.Item)
	case "item/agentMessage/delta":
		event.Kind = provider.TextDelta
	case "item/commandExecution/outputDelta", "item/fileChange/outputDelta", "item/reasoning/summaryTextDelta", "item/reasoning/textDelta", "item/plan/delta":
		event.Kind = provider.OutputDelta
	case "item/mcpToolCall/progress":
		event.Kind, event.Delta = provider.OutputDelta, params.Message+"\n"
	case "item/commandExecution/requestApproval":
		event.Kind, event.InteractionKind = provider.ApprovalRequested, "command"
	case "item/fileChange/requestApproval":
		event.Kind, event.InteractionKind = provider.ApprovalRequested, "file"
	case "item/tool/requestUserInput":
		event.Kind, event.InteractionKind = provider.QuestionRequested, "question"
	case "serverRequest/resolved":
		event.Kind, event.ResolvedID = provider.InteractionResolved, params.RequestID
	case "error":
		event.Kind = provider.ExecutionError
		if params.Error != nil {
			event.Error = params.Error.Message
		}
	default:
		if len(packet.ID) > 0 {
			_ = c.Reject(packet.ID, "This interaction is not supported by Loom")
		}
		return provider.Event{}, false
	}
	return event, true
}
