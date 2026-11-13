package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/liyiligang/loom/internal/provider"
	"github.com/liyiligang/loom/internal/provider/jsonrpc"
	"github.com/liyiligang/loom/internal/provider/stdio"
	"net/http"
	"sync"
	"time"
)

type packet = jsonrpc.Packet

type Client struct {
	*jsonrpc.Client
	version string
	toolsMu sync.Mutex
	tools   map[string]provider.ToolServer
}

func Dial(ctx context.Context, address, token string) (*Client, error) {
	header := http.Header{}
	if token != "" {
		header.Set("Authorization", "Bearer "+token)
	}
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, response, err := dialer.DialContext(ctx, address, header)
	if err != nil {
		if response != nil && response.StatusCode == http.StatusUnauthorized {
			return nil, errors.New("authentication required: update the service token")
		}
		return nil, errors.New("cannot reach the Codex service; check its address and listener")
	}
	return initialize(ctx, jsonrpc.WrapWebSocket(conn))
}

func Start(ctx context.Context, executable string) (*Client, error) {
	process, err := stdio.Start(ctx, executable, []string{"app-server", "--listen", "stdio://"}, "")
	if err != nil {
		return nil, err
	}
	handshake, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return initialize(handshake, process)
}

func initialize(ctx context.Context, wire jsonrpc.Transport) (*Client, error) {
	c := &Client{tools: make(map[string]provider.ToolServer)}
	c.Client = jsonrpc.New(wire, c.event)
	var result struct {
		UserAgent string `json:"userAgent"`
	}
	err := c.Call(ctx, "initialize", map[string]any{"clientInfo": map[string]string{"name": "loom", "title": "Loom", "version": "0.1.0"}, "capabilities": map[string]bool{"experimentalApi": true}}, &result)
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("Codex handshake failed: %w", err)
	}
	if result.UserAgent == "" {
		c.Close()
		return nil, errors.New("the endpoint did not identify a compatible Codex app-server")
	}
	c.version = result.UserAgent
	if err := c.Write(map[string]any{"method": "initialized"}); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

type nativeThread struct {
	ID    string       `json:"id"`
	Turns []nativeTurn `json:"turns"`
}

type nativeTurn struct {
	ID     string            `json:"id"`
	Status string            `json:"status"`
	Items  []json.RawMessage `json:"items"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type nativeThreadResult struct {
	Thread nativeThread `json:"thread"`
	Model  string       `json:"model"`
}
