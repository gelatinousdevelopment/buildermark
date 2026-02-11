package handler

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/davidcann/zrate/web/server/internal/db"
)

type createRatingRequest struct {
	ConversationID string `json:"conversationId"`
	Rating         int    `json:"rating"`
	Note           string `json:"note"`
	Analysis       string `json:"analysis"`
	Agent          string `json:"agent"`
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

	agentName := req.Agent
	if agentName == "" {
		agentName = "claude"
	}

	rating, err := db.InsertRating(r.Context(), s.DB, req.ConversationID, req.Rating, req.Note, req.Analysis)
	if err != nil {
		log.Printf("error inserting rating: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to create rating")
		return
	}

	// Resolve real session in the background, then collect and store all conversation turns.
	go func(ratingID, fallbackCID string, ratingVal int, note, agent string) {
		if s.Agents == nil {
			return
		}
		res := s.Agents.Resolver(agent)
		if res == nil {
			return
		}

		result := res.ResolveSession(ratingVal, note, fallbackCID)

		if result.SessionID == fallbackCID {
			// Resolution failed — don't create phantom conversation/project.
			// The watcher will reconcile the orphaned rating when it sees the /zrate entry.
			return
		}

		if err := db.UpdateConversationID(context.Background(), s.DB, ratingID, result.SessionID); err != nil {
			log.Printf("error updating conversation_id: %v", err)
		}

		ctx := context.Background()

		projectPath := result.Project
		if projectPath == "" {
			projectPath = "unknown"
		}

		projectID, err := db.EnsureProject(ctx, s.DB, projectPath)
		if err != nil {
			log.Printf("error ensuring project: %v", err)
			return
		}

		if err := db.EnsureConversation(ctx, s.DB, result.SessionID, projectID, agent); err != nil {
			log.Printf("error ensuring conversation: %v", err)
			return
		}

		if len(result.Entries) == 0 {
			return
		}

		turns := make([]db.Turn, len(result.Entries))
		for i, e := range result.Entries {
			turns[i] = db.Turn{
				Timestamp:      e.Timestamp,
				ProjectID:      projectID,
				ConversationID: result.SessionID,
				Role:           e.Role,
				Content:        e.Display,
				RawJSON:        e.RawJSON,
			}
		}

		if err := db.InsertTurns(ctx, s.DB, turns); err != nil {
			log.Printf("error inserting turns: %v", err)
		}
	}(rating.ID, req.ConversationID, req.Rating, req.Note, agentName)

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
