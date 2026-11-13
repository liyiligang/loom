package jsonrpc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/liyiligang/loom/internal/provider"
)

type Packet struct {
	ID     json.RawMessage     `json:"id,omitempty"`
	Method string              `json:"method,omitempty"`
	Params json.RawMessage     `json:"params,omitempty"`
	Result json.RawMessage     `json:"result,omitempty"`
	Error  *provider.Rejection `json:"error,omitempty"`
}

type Transport interface {
	ReadJSON(any) error
	WriteJSON(any) error
	Close() error
}

type Client struct {
	wire    Transport
	convert func(Packet) (provider.Event, bool)
	writes  sync.Mutex
	mu      sync.Mutex
	pending map[string]chan Packet
	seq     atomic.Uint64
	events  chan provider.Event
	done    chan struct{}
	once    sync.Once
}

func New(wire Transport, convert func(Packet) (provider.Event, bool)) *Client {
	c := &Client{wire: wire, convert: convert, pending: make(map[string]chan Packet), events: make(chan provider.Event, 1024), done: make(chan struct{})}
	go c.read()
	return c
}
func (c *Client) Events() <-chan provider.Event { return c.events }
func (c *Client) Done() <-chan struct{}         { return c.done }
func (c *Client) Call(ctx context.Context, method string, params, result any) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	id := fmt.Sprintf("loom-%d", c.seq.Add(1))
	raw, _ := json.Marshal(id)
	key := string(raw)
	ch := make(chan Packet, 1)
	c.mu.Lock()
	c.pending[key] = ch
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(c.pending, key); c.mu.Unlock() }()
	if err := c.Write(map[string]any{"id": id, "method": method, "params": params}); err != nil {
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
	case <-c.done:
		// A final response can arrive immediately before the transport reaches EOF.
		select {
		case packet := <-ch:
			if packet.Error != nil {
				return packet.Error
			}
			if result != nil {
				return json.Unmarshal(packet.Result, result)
			}
			return nil
		default:
		}
		return errors.New("agent connection lost before acknowledgement")
	}
}
func (c *Client) Reply(id json.RawMessage, result any) error {
	return c.Write(map[string]any{"id": id, "result": result})
}
func (c *Client) Reject(id json.RawMessage, message string) error {
	return c.Write(map[string]any{"id": id, "error": provider.Rejection{Code: -32601, Message: message}})
}
func (c *Client) Write(value any) error {
	c.writes.Lock()
	defer c.writes.Unlock()
	select {
	case <-c.done:
		return errors.New("agent connection is closed")
	default:
	}
	if err := c.wire.WriteJSON(value); err != nil {
		c.Close()
		return errors.New("agent write failed; execution status may be unknown")
	}
	return nil
}
func (c *Client) read() {
	defer close(c.events)
	for {
		var packet Packet
		if err := c.wire.ReadJSON(&packet); err != nil {
			c.Close()
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
		event, ok := c.convert(packet)
		if !ok {
			continue
		}
		select {
		case c.events <- event:
		case <-c.done:
			return
		default:
			c.Close()
			return
		}
	}
}
func (c *Client) Close() { c.once.Do(func() { close(c.done); _ = c.wire.Close() }) }
