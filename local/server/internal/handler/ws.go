package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// wsMessage is the top-level JSON envelope sent over WebSocket.
type wsMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// jobStatusEvent is broadcast over WebSocket for any background job.
type jobStatusEvent struct {
	JobType   string `json:"jobType"`             // "import", "history_scan", "diff_recompute", "commit_ingest"
	State     string `json:"state"`               // "running", "complete", "error"
	Message   string `json:"message"`
	ProjectID string `json:"projectId,omitempty"`
	Branch    string `json:"branch,omitempty"`
}

// wsClient represents a single connected WebSocket client.
type wsClient struct {
	conn *websocket.Conn
	send chan []byte
}

// wsHub manages connected WebSocket clients and broadcasts messages.
type wsHub struct {
	mu      sync.RWMutex
	clients map[*wsClient]struct{}
}

func newWSHub() *wsHub {
	return &wsHub{clients: make(map[*wsClient]struct{})}
}

func (h *wsHub) register(c *wsClient) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
}

func (h *wsHub) unregister(c *wsClient) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

func (h *wsHub) broadcast(msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.clients {
		select {
		case c.send <- msg:
		default:
			// client too slow, drop message
		}
	}
}

func (h *wsHub) broadcastEvent(eventType string, data any) {
	raw, err := json.Marshal(data)
	if err != nil {
		return
	}
	msg, err := json.Marshal(wsMessage{Type: eventType, Data: raw})
	if err != nil {
		return
	}
	h.broadcast(msg)
}

func (h *wsHub) clientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

const (
	wsPingInterval = 30 * time.Second
	wsWriteWait    = 10 * time.Second
	wsPongWait     = 60 * time.Second
)

// handleWS upgrades the HTTP connection to WebSocket and manages the client lifecycle.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("websocket upgrade failed: %v", err)
		return
	}

	client := &wsClient{
		conn: conn,
		send: make(chan []byte, 64),
	}
	s.ws.register(client)

	// Writer goroutine: sends queued messages and pings to the client.
	go func() {
		ticker := time.NewTicker(wsPingInterval)
		defer ticker.Stop()
		defer conn.Close()

		for {
			select {
			case msg, ok := <-client.send:
				if !ok {
					conn.WriteMessage(websocket.CloseMessage, nil)
					return
				}
				conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
				if err := conn.WriteMessage(websocket.TextMessage, msg); err != nil {
					return
				}
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}()

	// Reader loop: reads and discards messages, keeps connection alive via pong handler.
	conn.SetReadDeadline(time.Now().Add(wsPongWait))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(wsPongWait))
		return nil
	})
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}

	s.ws.unregister(client)
	close(client.send)
}
