package handler

import (
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/davidcann/zrate/web/server/internal/db"
)

func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}

	conversations, err := db.ListConversations(r.Context(), s.DB, limit)
	if err != nil {
		log.Printf("error listing conversations: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list conversations")
		return
	}
	writeSuccess(w, http.StatusOK, conversations)
}

func (s *Server) handleGetConversation(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "conversation id is required")
		return
	}

	conv, err := db.GetConversationDetail(r.Context(), s.DB, id)
	if err != nil {
		log.Printf("error getting conversation: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to get conversation")
		return
	}
	if conv == nil {
		writeError(w, http.StatusNotFound, "conversation not found")
		return
	}

	writeSuccess(w, http.StatusOK, conv)
}

func (s *Server) handleGetConversationsBatchDetail(w http.ResponseWriter, r *http.Request) {
	idsRaw := strings.TrimSpace(r.URL.Query().Get("ids"))
	if idsRaw == "" {
		writeError(w, http.StatusBadRequest, "ids query parameter is required")
		return
	}
	ids := strings.Split(idsRaw, ",")
	if len(ids) > 200 {
		ids = ids[:200]
	}

	details, err := db.GetConversationsBatchDetail(r.Context(), s.DB, ids)
	if err != nil {
		log.Printf("error getting batch conversation detail: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to get conversation details")
		return
	}
	writeSuccess(w, http.StatusOK, details)
}
