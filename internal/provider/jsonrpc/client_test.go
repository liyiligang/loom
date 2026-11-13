package jsonrpc

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"testing"

	"github.com/liyiligang/loom/internal/provider"
)

type responseWire struct {
	responses chan Packet
	done      chan struct{}
	once      sync.Once
	final     bool
}

func (w *responseWire) ReadJSON(value any) error {
	select {
	case packet := <-w.responses:
		*(value.(*Packet)) = packet
		if w.final {
			w.Close()
		}
		return nil
	case <-w.done:
		return io.EOF
	}
}
func (w *responseWire) WriteJSON(value any) error {
	request := value.(map[string]any)
	id, _ := json.Marshal(request["id"])
	result, _ := json.Marshal(request["params"])
	w.responses <- Packet{ID: id, Result: result}
	return nil
}
func (w *responseWire) Close() error { w.once.Do(func() { close(w.done) }); return nil }
func TestFinalAcknowledgementIsNotLostAtEOF(t *testing.T) {
	for range 30 {
		wire := &responseWire{responses: make(chan Packet, 1), done: make(chan struct{}), final: true}
		client := New(wire, func(Packet) (provider.Event, bool) { return provider.Event{}, false })
		var answer string
		if err := client.Call(context.Background(), "last", "acknowledged", &answer); err != nil || answer != "acknowledged" {
			t.Fatalf("final acknowledgement: %s %v", answer, err)
		}
		client.Close()
	}
}
func TestConcurrentCallsKeepTheirOwnResponses(t *testing.T) {
	wire := &responseWire{responses: make(chan Packet, 32), done: make(chan struct{})}
	client := New(wire, func(Packet) (provider.Event, bool) { return provider.Event{}, false })
	defer client.Close()
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var answer int
			if err := client.Call(context.Background(), "echo", i, &answer); err != nil || answer != i {
				t.Errorf("correlation: got %d want %d: %v", answer, i, err)
			}
		}()
	}
	wg.Wait()
}
