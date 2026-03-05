package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

// CommitConversationLinks holds the bidirectional mapping between commit hashes
// and conversation IDs for a project.
type CommitConversationLinks struct {
	CommitToConversations map[string][]string `json:"commitToConversations"`
	ConversationToCommits map[string][]string `json:"conversationToCommits"`
}

type commitConversationLinksRequest struct {
	CommitHashes    []string `json:"commitHashes"`
	ConversationIDs []string `json:"conversationIds"`
}

func (s *Server) handleGetCommitConversationLinks(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.PathValue("projectId"))
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project id is required")
		return
	}

	var commitHashes []string
	var conversationIDFilter map[string]bool

	if r.Method == http.MethodPost {
		var req commitConversationLinksRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		commitHashes = req.CommitHashes
		if len(req.ConversationIDs) > 0 {
			conversationIDFilter = make(map[string]bool, len(req.ConversationIDs))
			for _, id := range req.ConversationIDs {
				conversationIDFilter[id] = true
			}
		}
	} else {
		commitHashesRaw := strings.TrimSpace(r.URL.Query().Get("commitHashes"))
		if commitHashesRaw == "" {
			writeError(w, http.StatusBadRequest, "commitHashes query parameter is required")
			return
		}
		commitHashes = splitCSV(commitHashesRaw)

		conversationIDsRaw := strings.TrimSpace(r.URL.Query().Get("conversationIds"))
		if conversationIDsRaw != "" {
			ids := splitCSV(conversationIDsRaw)
			conversationIDFilter = make(map[string]bool, len(ids))
			for _, id := range ids {
				conversationIDFilter[id] = true
			}
		}
	}

	if len(commitHashes) == 0 {
		writeSuccess(w, http.StatusOK, &CommitConversationLinks{
			CommitToConversations: map[string][]string{},
			ConversationToCommits: map[string][]string{},
		})
		return
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

	pIDs := projectIDs(group)

	// Use cached commit-conversation links from the database.
	cached, err := db.GetCachedCommitConversationLinks(r.Context(), s.DB, pIDs, commitHashes)
	if err != nil {
		log.Printf("error loading cached commit conversation links: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load commit conversation links")
		return
	}

	// Build bidirectional maps, applying optional conversation ID filter.
	c2c := make(map[string][]string)
	c2commit := make(map[string][]string)

	for commitHash, convIDs := range cached {
		for _, convID := range convIDs {
			if conversationIDFilter != nil && !conversationIDFilter[convID] {
				continue
			}
			c2c[commitHash] = append(c2c[commitHash], convID)
			c2commit[convID] = append(c2commit[convID], commitHash)
		}
	}

	writeSuccess(w, http.StatusOK, &CommitConversationLinks{
		CommitToConversations: c2c,
		ConversationToCommits: c2commit,
	})
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
