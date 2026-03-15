package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClearUpdateStatusEndpointClearsServerState(t *testing.T) {
	s := setupTestServer(t)
	s.SetUpdateStatus(UpdateStatusEvent{
		State:    "available",
		Version:  "v1.2.3",
		Platform: "linux",
	})
	handler := s.Routes()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/update-status", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want %d", rec.Code, http.StatusOK)
	}

	var deleteEnv jsonEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&deleteEnv); err != nil {
		t.Fatalf("decode delete response: %v", err)
	}
	if !deleteEnv.OK {
		t.Fatalf("delete ok = false, error = %q", deleteEnv.Error)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/update-status", nil)
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", getRec.Code, http.StatusOK)
	}

	var getEnv struct {
		OK   bool              `json:"ok"`
		Data UpdateStatusEvent `json:"data"`
	}
	if err := json.NewDecoder(getRec.Body).Decode(&getEnv); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if !getEnv.OK {
		t.Fatal("get ok = false")
	}
	if getEnv.Data.State != "none" {
		t.Fatalf("state = %q, want %q", getEnv.Data.State, "none")
	}
}
