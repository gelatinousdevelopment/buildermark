package handler

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
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
	JobType   string `json:"jobType"` // "import", "history_scan", "diff_recompute", "commit_ingest"
	State     string `json:"state"`   // "running", "complete", "error"
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
	mu               sync.RWMutex
	clients          map[*wsClient]struct{}
	runningJobStatus map[string][]byte
	onClientChange   func() // called after register/unregister (with lock released)
}

func newWSHub() *wsHub {
	return &wsHub{
		clients:          make(map[*wsClient]struct{}),
		runningJobStatus: make(map[string][]byte),
	}
}

func (h *wsHub) register(c *wsClient) {
	h.mu.Lock()
	h.clients[c] = struct{}{}
	running := make([][]byte, 0, len(h.runningJobStatus))
	for _, msg := range h.runningJobStatus {
		running = append(running, msg)
	}
	h.mu.Unlock()

	for _, msg := range running {
		select {
		case c.send <- msg:
		default:
			return
		}
	}
	if h.onClientChange != nil {
		h.onClientChange()
	}
}

func (h *wsHub) unregister(c *wsClient) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
	if h.onClientChange != nil {
		h.onClientChange()
	}
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

	h.trackRunningJobStatus(eventType, data, msg)
	h.broadcast(msg)
}

func (h *wsHub) trackRunningJobStatus(eventType string, data any, msg []byte) {
	if eventType != "job_status" {
		return
	}
	status, ok := data.(jobStatusEvent)
	if !ok || status.JobType == "" {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	if status.State == "running" {
		h.runningJobStatus[status.JobType] = append([]byte(nil), msg...)
		return
	}
	delete(h.runningJobStatus, status.JobType)
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

// wsClientsEvent is broadcast over the main WebSocket when client counts change.
type wsClientsEvent struct {
	Frontend     int `json:"frontend"`
	Notification int `json:"notification"`
}

// broadcastWSClients sends updated client counts over the main WebSocket.
func (s *Server) broadcastWSClients() {
	if s.ws == nil {
		return
	}
	s.ws.broadcastEvent("ws_clients", wsClientsEvent{
		Frontend:     s.ws.clientCount(),
		Notification: s.notifyWS.clientCount(),
	})
}

// handleDebugSendNotification sends a test notification over the notifications WebSocket.
func (s *Server) handleDebugSendNotification(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	var body struct {
		Kind  string `json:"kind"`
		Title string `json:"title"`
		Body  string `json:"body"`
		URL   string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if body.Title == "" {
		body.Title = "Test notification"
	}
	if body.Body == "" {
		body.Body = "This is a test notification from the debug page"
	}
	if body.Kind == "" {
		body.Kind = "debug_test"
	}
	s.sendNotification(body.Kind, body.Title, body.Body, body.URL)
	writeSuccess(w, http.StatusOK, map[string]any{"sent": true})
}

// handleDebugWSClients returns current WebSocket client counts.
func (s *Server) handleDebugWSClients(w http.ResponseWriter, r *http.Request) {
	writeSuccess(w, http.StatusOK, wsClientsEvent{
		Frontend:     s.ws.clientCount(),
		Notification: s.notifyWS.clientCount(),
	})
}

// notificationEvent is the payload sent over the notifications WebSocket.
type notificationEvent struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	URL       string `json:"url,omitempty"`
	Timestamp string `json:"timestamp"`
}

// sendNotification broadcasts a notification to all connected notification WebSocket clients.
func (s *Server) sendNotification(kind, title, body, url string) {
	if s.notifyWS == nil {
		return
	}
	var id [8]byte
	if _, err := rand.Read(id[:]); err != nil {
		return
	}
	event := notificationEvent{
		ID:        hex.EncodeToString(id[:]),
		Kind:      kind,
		Title:     title,
		Body:      body,
		URL:       url,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return
	}
	msg, err := json.Marshal(wsMessage{Type: "notification", Data: raw})
	if err != nil {
		return
	}
	s.notifyWS.broadcast(msg)
}

// notifyIngestedCommits sends a native notification summarizing ingested commits.
func (s *Server) notifyIngestedCommits(commits []db.Commit, projectLabel string) {
	if len(commits) == 0 {
		return
	}

	// Compute weighted average percentage.
	var totalLines, agentLines int
	for _, c := range commits {
		totalLines += c.LinesTotal
		agentLines += c.LinesFromAgent
	}
	pct := 0
	if totalLines > 0 {
		pct = agentLines * 100 / totalLines
	}

	if len(commits) == 1 {
		c := commits[0]
		subject := c.Subject
		if len(subject) > 256 {
			subject = subject[:256]
		}
		url := fmt.Sprintf("/projects/%s/commits/%s/%s", c.ProjectID, c.BranchName, c.CommitHash)
		s.sendNotification("commit_ingested", fmt.Sprintf("New commit %d%% by agents", pct), subject, url)
	} else {
		url := ""
		// If all commits are from the same project, link to that project.
		sameProject := true
		for _, c := range commits[1:] {
			if c.ProjectID != commits[0].ProjectID {
				sameProject = false
				break
			}
		}
		if sameProject {
			url = fmt.Sprintf("/projects/%s", commits[0].ProjectID)
		}
		s.sendNotification("commit_ingested", fmt.Sprintf("%d commits %d%% by agents", len(commits), pct), projectLabel, url)
	}
}

// handleNotificationsWS upgrades the HTTP connection to a notifications-only WebSocket.
func (s *Server) handleNotificationsWS(w http.ResponseWriter, r *http.Request) {
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("notifications websocket upgrade failed: %v", err)
		return
	}

	client := &wsClient{
		conn: conn,
		send: make(chan []byte, 64),
	}
	s.notifyWS.register(client)

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

	s.notifyWS.unregister(client)
	close(client.send)
}
