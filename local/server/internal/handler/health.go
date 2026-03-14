package handler

import (
	"encoding/json"
	"net/http"
	"runtime"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]any{
		"ok":       true,
		"platform": runtime.GOOS,
	}
	if s.version != "" {
		resp["version"] = s.version
	}
	json.NewEncoder(w).Encode(resp)
}
