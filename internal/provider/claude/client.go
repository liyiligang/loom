package claude

import (
	"context"
	"encoding/json"
	bridge "github.com/liyiligang/loom/bridge/claude"
	"github.com/liyiligang/loom/internal/provider"
	"github.com/liyiligang/loom/internal/provider/jsonrpc"
	"github.com/liyiligang/loom/internal/provider/stdio"
	"os"
	"path/filepath"
	"time"
)

type Client struct {
	*jsonrpc.Client
	version string
}

func Start(ctx context.Context, node, executable, directory string) (*Client, error) {
	runtimeDirectory := filepath.Join(directory, "runtime")
	if err := os.MkdirAll(runtimeDirectory, 0700); err != nil {
		return nil, err
	}
	script := filepath.Join(runtimeDirectory, "claude-bridge.mjs")
	if err := os.WriteFile(script, bridge.Source, 0600); err != nil {
		return nil, err
	}
	process, err := stdio.Start(ctx, node, []string{script, executable, filepath.Join(directory, "claude-sessions")}, "")
	if err != nil {
		return nil, err
	}
	c := &Client{}
	c.Client = jsonrpc.New(process, func(packet jsonrpc.Packet) (provider.Event, bool) {
		var event provider.Event
		if packet.Method != "event" {
			return event, false
		}
		err := json.Unmarshal(packet.Params, &event)
		return event, err == nil
	})
	handshake, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var result struct {
		Version string `json:"version"`
	}
	if err := c.Call(handshake, "initialize", map[string]any{}, &result); err != nil {
		c.Close()
		return nil, err
	}
	c.version = result.Version
	return c, nil
}
func (c *Client) Version() string { return c.version }
func (c *Client) CreateSession(ctx context.Context, options provider.SessionOptions) (provider.SessionSnapshot, error) {
	var result provider.SessionSnapshot
	err := c.Call(ctx, "session/create", map[string]any{"cwd": options.Cwd, "model": options.Model}, &result)
	return result, err
}
func (c *Client) SynchronizeSession(ctx context.Context, ref provider.SessionReference) (provider.SessionSnapshot, error) {
	var result provider.SessionSnapshot
	err := c.Call(ctx, "session/synchronize", map[string]any{"id": ref.ID, "cwd": ref.Cwd, "subscribed": ref.Subscribed, "empty": ref.Empty}, &result)
	if err != nil && err.Error() == provider.ErrEmptySession.Error() {
		return result, provider.ErrEmptySession
	}
	return result, err
}
func (c *Client) StartTurn(ctx context.Context, id string, options provider.TurnOptions) (provider.TurnSnapshot, error) {
	var result provider.TurnSnapshot
	err := c.Call(ctx, "turn/start", map[string]any{"id": id, "text": options.Text, "credentialTools": options.Tools, "accessMode": options.AccessMode.Effective(), "model": options.Model}, &result)
	return result, err
}
func (c *Client) Interrupt(ctx context.Context, id, turnID string) error {
	return c.Call(ctx, "turn/interrupt", map[string]any{"id": id, "turnId": turnID}, nil)
}
