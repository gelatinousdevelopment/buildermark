package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/davidcann/zrate/web/server/internal/db"
	"github.com/davidcann/zrate/web/server/internal/history"
)

type createRatingRequest struct {
	ConversationID string `json:"conversationId"`
	Rating         int    `json:"rating"`
	Note           string `json:"note"`
}

func (s *Server) handleCreateRating(w http.ResponseWriter, r *http.Request) {
	if ct := r.Header.Get("Content-Type"); ct != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB

	var req createRatingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.ConversationID == "" {
		writeError(w, http.StatusBadRequest, "conversationId is required")
		return
	}

	if req.Rating < 0 || req.Rating > 5 {
		writeError(w, http.StatusBadRequest, "rating must be between 0 and 5")
		return
	}

	rating, err := db.InsertRating(r.Context(), s.DB, req.ConversationID, req.Rating, req.Note)
	if err != nil {
		log.Printf("error inserting rating: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create rating")
		return
	}

	// Resolve real Claude Code sessionId in the background.
	go func(ratingID, fallbackCID string, ratingVal int, note string) {
		resolved := history.ResolveSessionID(ratingVal, note, fallbackCID)
		if resolved != fallbackCID {
			if err := db.UpdateConversationID(context.Background(), s.DB, ratingID, resolved); err != nil {
				log.Printf("error updating conversation_id: %v", err)
			}
		}
	}(rating.ID, req.ConversationID, req.Rating, req.Note)

	writeSuccess(w, http.StatusCreated, rating)
}

func (s *Server) handleListRatings(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}

	ratings, err := db.ListRatings(r.Context(), s.DB, limit)
	if err != nil {
		log.Printf("error listing ratings: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list ratings")
		return
	}

	writeSuccess(w, http.StatusOK, ratings)
}
