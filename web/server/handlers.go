package main

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
)

type jsonEnvelope struct {
	OK    bool        `json:"ok"`
	Data  interface{} `json:"data,omitempty"`
	Error string      `json:"error,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, jsonEnvelope{OK: false, Error: msg})
}

func writeSuccess(w http.ResponseWriter, status int, data interface{}) {
	writeJSON(w, status, jsonEnvelope{OK: true, Data: data})
}

type createRatingRequest struct {
	ConversationID string `json:"conversationId"`
	Rating         int    `json:"rating"`
	Note           string `json:"note"`
}

func handleCreateRating(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB

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

		rating, err := InsertRating(db, req.ConversationID, req.Rating, req.Note)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create rating")
			return
		}

		// Resolve real Claude Code sessionId in the background
		go func(ratingID, fallbackCID string, ratingVal int, note string) {
			resolved := resolveSessionID(ratingVal, note, fallbackCID)
			if resolved != fallbackCID {
				if err := UpdateConversationID(db, ratingID, resolved); err != nil {
					log.Printf("error updating conversation_id: %v", err)
				}
			}
		}(rating.ID, req.ConversationID, req.Rating, req.Note)

		writeSuccess(w, http.StatusCreated, rating)
	}
}

func handleListRatings(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		limit := 50
		if l := r.URL.Query().Get("limit"); l != "" {
			if parsed, err := strconv.Atoi(l); err == nil {
				limit = parsed
			}
		}

		ratings, err := ListRatings(db, limit)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to list ratings")
			return
		}

		writeSuccess(w, http.StatusOK, ratings)
	}
}
