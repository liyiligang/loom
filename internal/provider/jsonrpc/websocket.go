package jsonrpc

import (
	"github.com/gorilla/websocket"
	"sync"
	"time"
)

type WebSocket struct {
	Conn *websocket.Conn
	done chan struct{}
	once sync.Once
}

func WrapWebSocket(conn *websocket.Conn) *WebSocket {
	wire := &WebSocket{Conn: conn, done: make(chan struct{})}
	conn.SetReadLimit(16 << 20)
	_ = conn.SetReadDeadline(time.Now().Add(90 * time.Second))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(90 * time.Second)) })
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-wire.done:
				return
			case <-ticker.C:
				if conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)) != nil {
					_ = wire.Close()
					return
				}
			}
		}
	}()
	return wire
}
func (w *WebSocket) ReadJSON(value any) error { return w.Conn.ReadJSON(value) }
func (w *WebSocket) WriteJSON(value any) error {
	_ = w.Conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	return w.Conn.WriteJSON(value)
}
func (w *WebSocket) Close() error {
	var err error
	w.once.Do(func() { close(w.done); err = w.Conn.Close() })
	return err
}
