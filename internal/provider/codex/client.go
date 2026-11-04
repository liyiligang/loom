package codex

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type Packet struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *RPCError       `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string { return e.Message }

type Client struct {
	conn    *websocket.Conn
	writes  sync.Mutex
	mu      sync.Mutex
	pending map[string]chan Packet
	seq     atomic.Uint64
	Events  chan Packet
	Done    chan struct{}
	once    sync.Once
	err     error
	Version string
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
	c := &Client{conn: conn, pending: make(map[string]chan Packet), Events: make(chan Packet, 1024), Done: make(chan struct{})}
	conn.SetReadLimit(16 << 20)
	_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(90 * time.Second)) })
	go c.read()
	go c.heartbeat()
	var result struct {
		UserAgent string `json:"userAgent"`
	}
	if err := c.Call(ctx, "initialize", map[string]any{
		"clientInfo":   map[string]string{"name": "loom", "title": "Loom", "version": "0.1.0"},
		"capabilities": map[string]bool{"experimentalApi": true},
	}, &result); err != nil {
		c.Close()
		return nil, fmt.Errorf("Codex handshake failed: %w", err)
	}
	if result.UserAgent == "" {
		c.Close()
		return nil, errors.New("the endpoint did not identify a compatible Codex app-server")
	}
	c.Version = result.UserAgent
	if err := c.write(map[string]any{"method": "initialized"}); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

func (c *Client) Call(ctx context.Context, method string, params, result any) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	id := fmt.Sprintf("loom-%d", c.seq.Add(1))
	keyBytes, _ := json.Marshal(id)
	key := string(keyBytes)
	ch := make(chan Packet, 1)
	c.mu.Lock()
	c.pending[key] = ch
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.pending, key); c.mu.Unlock() }()
	if err := c.write(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return err
	}
	select {
	case packet := <-ch:
		if packet.Error != nil {
			return packet.Error
		}
		if result != nil {
			return json.Unmarshal(packet.Result, result)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%s acknowledgement unavailable: %w", method, ctx.Err())
	case <-c.Done:
		return errors.New("agent connection lost before acknowledgement")
	}
}

func (c *Client) Reply(id json.RawMessage, result any) error {
	return c.write(map[string]any{"id": id, "result": result})
}

func (c *Client) Reject(id json.RawMessage, message string) error {
	return c.write(map[string]any{"id": id, "error": RPCError{Code: -32601, Message: message}})
}

func (c *Client) write(value any) error {
	c.writes.Lock()
	defer c.writes.Unlock()
	select {
	case <-c.Done:
		return errors.New("agent connection is closed")
	default:
	}
	_ = c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	if err := c.conn.WriteJSON(value); err != nil {
		c.stop(err)
		return errors.New("agent write failed; execution status may be unknown")
	}
	return nil
}

func (c *Client) read() {
	defer close(c.Events)
	for {
		var packet Packet
		if err := c.conn.ReadJSON(&packet); err != nil {
			c.stop(err)
			return
		}
		if packet.Method == "" && len(packet.ID) > 0 {
			c.mu.Lock()
			ch := c.pending[string(packet.ID)]
			c.mu.Unlock()
			if ch != nil {
				select {
				case ch <- packet:
				default:
				}
			}
			continue
		}
		select {
		case c.Events <- packet:
		case <-c.Done:
			return
		default:
			c.stop(errors.New("agent event buffer exceeded; restore the session to reconcile history"))
			return
		}
	}
}

func (c *Client) heartbeat() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.Done:
			return
		case <-ticker.C:
			if err := c.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
				c.stop(err)
				return
			}
		}
	}
}

func (c *Client) stop(err error) {
	c.once.Do(func() { c.mu.Lock(); c.err = err; c.mu.Unlock(); close(c.Done); c.conn.Close() })
}

func (c *Client) Close() { c.stop(errors.New("connection closed")) }

type Thread struct {
	ID    string       `json:"id"`
	Turns []NativeTurn `json:"turns"`
}

type NativeTurn struct {
	ID     string            `json:"id"`
	Status string            `json:"status"`
	Items  []json.RawMessage `json:"items"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type ThreadResult struct {
	Thread Thread `json:"thread"`
	Model  string `json:"model"`
}
