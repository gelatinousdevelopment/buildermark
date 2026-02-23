package handler

import (
	"log"
	"net/http"
	"strings"

	"github.com/davidcann/zrate/web/server/internal/db"
)

func (s *Server) handleGetCommitConversationLinks(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.PathValue("projectId"))
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project id is required")
		return
	}

	commitHashesRaw := strings.TrimSpace(r.URL.Query().Get("commitHashes"))
	conversationIDsRaw := strings.TrimSpace(r.URL.Query().Get("conversationIds"))

	if commitHashesRaw == "" {
		writeError(w, http.StatusBadRequest, "commitHashes query parameter is required")
		return
	}

	commitHashes := splitCSV(commitHashesRaw)
	if len(commitHashes) > 200 {
		commitHashes = commitHashes[:200]
	}

	var conversationIDs []string
	if conversationIDsRaw != "" {
		conversationIDs = splitCSV(conversationIDsRaw)
		if len(conversationIDs) > 200 {
			conversationIDs = conversationIDs[:200]
		}
	}

	// Resolve the project group to get all related project IDs.
	groups, err := listAllProjectGroups(r.Context(), s.DB)
	if err != nil {
		log.Printf("error listing project groups: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list projects")
		return
	}

	group, ok := findProjectGroupByProjectID(groups, projectID)
	if !ok {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}

	links, err := db.FindCommitConversationLinks(
		r.Context(),
		s.DB,
		projectIDs(group),
		commitHashes,
		conversationIDs,
		defaultMessageWindowMs,
		commitWindowLookaheadMs,
	)
	if err != nil {
		log.Printf("error finding commit-conversation links: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to find links")
		return
	}

	writeSuccess(w, http.StatusOK, links)
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
