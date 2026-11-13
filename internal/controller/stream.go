package controller

import (
	"time"

	"github.com/liyiligang/loom/internal/provider"
)

// Only output deltas wait for batching. Lifecycle and interaction events flush
// preceding output first, preserving finality and the durable snapshot cursor.
func (s *Service) consume(endpointID string, current *connection) {
	const byteLimit = 4 << 10
	type key struct {
		session string
		turn    string
		item    string
		kind    provider.EventKind
	}
	var pending []provider.Event
	positions := make(map[key]int)
	bytes := 0
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()
	var tick <-chan time.Time
	flush := func() {
		for _, event := range pending {
			s.handle(endpointID, current, event)
		}
		pending = nil
		clear(positions)
		bytes = 0
		timer.Stop()
		tick = nil
	}
	for {
		select {
		case event, open := <-current.client.Events():
			if !open {
				flush()
				return
			}
			if !event.IsDelta() {
				flush()
				s.handle(endpointID, current, event)
				continue
			}
			if len(pending) == 0 {
				timer.Reset(50 * time.Millisecond)
				tick = timer.C
			}
			identity := key{event.SessionID, event.TurnID, event.ItemID, event.Kind}
			if index, exists := positions[identity]; exists {
				pending[index].Delta += event.Delta
			} else {
				positions[identity] = len(pending)
				pending = append(pending, event)
			}
			bytes += len(event.Delta)
			if bytes >= byteLimit || len(pending) >= 128 {
				flush()
			}
		case <-tick:
			flush()
		case <-s.ctx.Done():
			flush()
			return
		}
	}
}
