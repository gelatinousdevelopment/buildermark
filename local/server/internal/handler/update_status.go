package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/updater"
)

// UpdateStatusEvent describes the current update state broadcast to frontend clients.
type UpdateStatusEvent struct {
	State           string `json:"state"`                     // "available", "installed", "none"
	Version         string `json:"version,omitempty"`         // the new/installed version
	PreviousVersion string `json:"previousVersion,omitempty"` // for "installed" state
	Platform        string `json:"platform"`                  // "darwin", "windows", "linux"
	ReleaseNotesURL string `json:"releaseNotesUrl,omitempty"` // github release link
}

// updateState tracks the current update status on the Server.
type updateState struct {
	mu            sync.Mutex
	status        *UpdateStatusEvent // current state, nil = none
	installedTime time.Time          // when "installed" was set
	installedSent bool               // true after first frontend received it post-15s
}

// SetUpdateStatus stores the update state and broadcasts it to frontend WS clients.
func (s *Server) SetUpdateStatus(event UpdateStatusEvent) {
	s.updateState.mu.Lock()
	s.updateState.status = &event
	if event.State == "installed" {
		s.updateState.installedTime = time.Now()
		s.updateState.installedSent = false
	}
	s.updateState.mu.Unlock()

	if s.ws != nil {
		s.ws.broadcastEvent("update_status", event)
	}
}

// getUpdateStatusForNewClient returns the event to send to a newly connecting
// frontend client, or nil if nothing should be sent.
func (s *Server) getUpdateStatusForNewClient() *UpdateStatusEvent {
	s.updateState.mu.Lock()
	defer s.updateState.mu.Unlock()

	if s.updateState.status == nil || s.updateState.status.State == "none" {
		return nil
	}

	event := *s.updateState.status

	if event.State == "available" {
		return &event
	}

	// "installed" state: within 15s window, send to all clients
	if time.Since(s.updateState.installedTime) < 15*time.Second {
		return &event
	}

	// After 15s, send to exactly one more client
	if !s.updateState.installedSent {
		s.updateState.installedSent = true
		return &event
	}

	return nil
}

// ClearUpdateStatus resets the update state.
func (s *Server) ClearUpdateStatus() {
	s.updateState.mu.Lock()
	s.updateState.status = nil
	s.updateState.installedTime = time.Time{}
	s.updateState.installedSent = false
	s.updateState.mu.Unlock()

	if s.ws != nil {
		s.ws.broadcastEvent("update_status", UpdateStatusEvent{State: "none", Platform: runtime.GOOS})
	}
}

// SetVersion sets the server version string.
func (s *Server) SetVersion(v string) {
	s.version = v
}

// handleGetUpdateStatus returns the current update status.
func (s *Server) handleGetUpdateStatus(w http.ResponseWriter, r *http.Request) {
	s.updateState.mu.Lock()
	status := s.updateState.status
	s.updateState.mu.Unlock()

	if status == nil {
		writeSuccess(w, http.StatusOK, UpdateStatusEvent{State: "none", Platform: runtime.GOOS})
		return
	}
	writeSuccess(w, http.StatusOK, *status)
}

// handleUpdateApply applies an available update (Linux CLI only).
func (s *Server) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	if runtime.GOOS != "linux" {
		writeError(w, http.StatusBadRequest, "update-apply is only supported on Linux CLI")
		return
	}

	u := updater.GetUpdater(s.version)
	result, err := u.Check()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check for updates: "+err.Error())
		return
	}
	if !result.HasUpdate {
		writeError(w, http.StatusConflict, "no update available")
		return
	}

	if err := u.Apply(result); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to apply update: "+err.Error())
		return
	}

	log.Printf("update applied: %s -> %s", result.CurrentVersion, result.LatestVersion)
	writeSuccess(w, http.StatusOK, map[string]string{
		"previousVersion": result.CurrentVersion,
		"version":         result.LatestVersion,
	})
}

// handleDebugSetUpdateStatus sets the update status from a debug request.
func (s *Server) handleDebugSetUpdateStatus(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}
	var event UpdateStatusEvent
	if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if event.Platform == "" {
		event.Platform = runtime.GOOS
	}
	s.SetUpdateStatus(event)
	writeSuccess(w, http.StatusOK, event)
}

// handleDebugClearUpdateStatus clears the update status.
func (s *Server) handleDebugClearUpdateStatus(w http.ResponseWriter, r *http.Request) {
	s.handleClearUpdateStatus(w, r)
}

// handleClearUpdateStatus clears the update status.
func (s *Server) handleClearUpdateStatus(w http.ResponseWriter, r *http.Request) {
	s.ClearUpdateStatus()
	writeSuccess(w, http.StatusOK, map[string]string{"cleared": "true"})
}

// handleNotificationsWSMessage processes an incoming message from a native app
// on the notifications WebSocket.
func (s *Server) handleNotificationsWSMessage(data []byte) {
	var msg wsMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}

	switch msg.Type {
	case "update_status":
		var event UpdateStatusEvent
		if err := json.Unmarshal(msg.Data, &event); err != nil {
			return
		}
		if event.Platform == "" {
			event.Platform = runtime.GOOS
		}
		s.SetUpdateStatus(event)
	}
}
