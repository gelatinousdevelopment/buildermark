package handler

import (
	"database/sql"
	"encoding/json"
	"log"
	"mime"
	"net/http"
	"strings"
	"sync"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent"
)

// Server holds dependencies shared across HTTP handlers.
type Server struct {
	DB         *sql.DB
	Agents     *agent.Registry // may be nil if no agents are registered
	DBPath     string          // resolved path to the SQLite database file
	ListenAddr string          // address the server is listening on
	ReadOnly   bool
	ConfigDir  string
	// PluginSourceDir optionally points at the repo-level plugins bundle used
	// by the plugin management endpoints.
	PluginSourceDir string

	// ReloadWatchers is called after settings change to start watchers for
	// any newly added agent homes. It returns the list of new home paths
	// that were added (empty if none). The caller can use these to trigger
	// an immediate scan.
	ReloadWatchers func() []string

	refreshJobs      *jobTracker
	coverageJobs     *jobTracker
	visibilityJobs   *jobTracker
	commitIngestJobs *jobTracker

	commitDetailCache *commitDetailCacheStore
	branchCache       *branchCacheStore

	ws       *wsHub
	importMu sync.Mutex // guards against concurrent imports

	staleScanMu       sync.Mutex
	staleScanInFlight map[string]struct{} // project IDs with pending stale scans
}

// Routes returns an http.Handler with all routes and middleware wired up.
func (s *Server) Routes() http.Handler {
	if s.ws == nil {
		s.ws = newWSHub()
	}
	if s.refreshJobs == nil {
		s.refreshJobs = newJobTracker()
	}
	if s.coverageJobs == nil {
		s.coverageJobs = newJobTracker()
	}
	if s.visibilityJobs == nil {
		s.visibilityJobs = newJobTracker()
	}
	if s.commitIngestJobs == nil {
		s.commitIngestJobs = newJobTracker()
	}
	if s.commitDetailCache == nil {
		s.commitDetailCache = newCommitDetailCacheStore()
	}
	if s.branchCache == nil {
		s.branchCache = newBranchCacheStore()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/ws", s.handleWS)
	mux.HandleFunc("POST /api/v1/rating", s.handleCreateRating)
	mux.HandleFunc("GET /api/v1/ratings", s.handleListRatings)
	mux.HandleFunc("GET /api/v1/projects", s.handleListProjects)
	mux.HandleFunc("GET /api/v1/search/projects", s.handleSearchProjects)
	mux.HandleFunc("GET /api/v1/projects/discover-importable", s.handleDiscoverImportableProjects)
	mux.HandleFunc("GET /api/v1/projects/{id}", s.handleGetProject)
	mux.HandleFunc("POST /api/v1/projects/import", s.handleImportProjects)
	mux.HandleFunc("GET /api/v1/projects/commits", s.handleListProjectCommits)
	mux.HandleFunc("GET /api/v1/projects/{projectId}/commits", s.handleListProjectCommitsForProject)
	mux.HandleFunc("GET /api/v1/projects/{projectId}/commits/{commitHash}", s.handleGetProjectCommit)
	mux.HandleFunc("POST /api/v1/projects/{projectId}/commits/{commitHash}/override-line-percent", s.handleSetCommitOverrideLinePercent)
	mux.HandleFunc("POST /api/v1/projects/{projectId}/commits/{commitHash}/deepen", s.handleDeepenCommit)
	mux.HandleFunc("DELETE /api/v1/projects/{id}", s.handleDeleteProject)
	mux.HandleFunc("POST /api/v1/projects/{id}/ignored", s.handleSetProjectIgnored)
	mux.HandleFunc("POST /api/v1/projects/{id}/label", s.handleSetProjectLabel)
	mux.HandleFunc("POST /api/v1/projects/{id}/path", s.handleSetProjectPath)
	mux.HandleFunc("POST /api/v1/projects/{id}/old-paths", s.handleSetProjectOldPaths)
	mux.HandleFunc("POST /api/v1/projects/{id}/ignore-diff-paths", s.handleSetProjectIgnoreDiffPaths)
	mux.HandleFunc("POST /api/v1/projects/{id}/ignore-default-diff-paths", s.handleSetProjectIgnoreDefaultDiffPaths)
	mux.HandleFunc("GET /api/v1/conversations", s.handleListConversations)
	mux.HandleFunc("GET /api/v1/conversations/batch-detail", s.handleGetConversationsBatchDetail)
	mux.HandleFunc("GET /api/v1/conversations/{id}", s.handleGetConversation)
	mux.HandleFunc("POST /api/v1/conversations/{id}/hidden", s.handleSetConversationHidden)
	mux.HandleFunc("GET /api/v1/conversations/check-url", s.handleCheckConversationURL)
	mux.HandleFunc("POST /api/v1/conversations/import-web", s.handleImportWebConversation)
	mux.HandleFunc("POST /api/v1/projects/{id}/ingest-commits", s.handleIngestMoreCommits)
	mux.HandleFunc("POST /api/v1/projects/{projectId}/refresh-commits", s.handleRefreshProjectCommits)
	mux.HandleFunc("POST /api/v1/projects/{id}/recompute-commit-coverage", s.handleRecomputeCommitCoverage)
	mux.HandleFunc("GET /api/v1/projects/{projectId}/commit-ingestion-status", s.handleCommitIngestionStatus)
	mux.HandleFunc("GET /api/v1/projects/{projectId}/commit-conversation-links", s.handleGetCommitConversationLinks)
	mux.HandleFunc("POST /api/v1/projects/{projectId}/commit-conversation-links", s.handleGetCommitConversationLinks)
	mux.HandleFunc("GET /api/v1/team-servers", s.handleListTeamServers)
	mux.HandleFunc("POST /api/v1/team-servers", s.handleCreateTeamServer)
	mux.HandleFunc("PUT /api/v1/team-servers/{id}", s.handleUpdateTeamServer)
	mux.HandleFunc("DELETE /api/v1/team-servers/{id}", s.handleDeleteTeamServer)
	mux.HandleFunc("POST /api/v1/projects/{id}/team-server", s.handleSetProjectTeamServer)
	mux.HandleFunc("POST /api/v1/history/scan", s.handleHistoryScan)
	mux.HandleFunc("GET /api/v1/settings", s.handleGetLocalSettings)
	mux.HandleFunc("PUT /api/v1/settings", s.handlePutLocalSettings)
	mux.HandleFunc("GET /api/v1/plugins", s.handleGetPlugins)
	mux.HandleFunc("POST /api/v1/plugins", s.handlePostPlugins)
	mux.Handle("GET /_app/", staticFrontendHandler())
	mux.HandleFunc("GET /", s.handleDashboard)
	return corsMiddleware(requestLogMiddleware(securityHeadersMiddleware(s.readOnlyMiddleware(mux))))
}

func (s *Server) readOnlyMiddleware(next http.Handler) http.Handler {
	if !s.ReadOnly {
		return next
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
			if r.URL.Path == "/api/v1/ws" {
				next.ServeHTTP(w, r)
				return
			}
			writeError(w, http.StatusServiceUnavailable, "server is in read-only mode")
			return
		default:
			next.ServeHTTP(w, r)
		}
	})
}

type jsonEnvelope struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		log.Printf("error encoding JSON response: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"ok":false,"error":"internal error"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(data)
}

// requireJSON checks that the request Content-Type is application/json,
// writing an error response and returning false if not.
func requireJSON(w http.ResponseWriter, r *http.Request) bool {
	mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mt != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "Content-Type must be application/json")
		return false
	}
	return true
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, jsonEnvelope{OK: false, Error: msg})
}

func writeSuccess(w http.ResponseWriter, status int, data any) {
	writeJSON(w, status, jsonEnvelope{OK: true, Data: data})
}

func requestLogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip noisy endpoints (WebSocket upgrades, static assets).
		if r.URL.Path != "/api/v1/ws" && !strings.HasPrefix(r.URL.Path, "/_app/") {
			log.Printf("%s %s", r.Method, r.URL.Path)
		}
		next.ServeHTTP(w, r)
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
