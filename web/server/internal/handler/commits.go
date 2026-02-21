package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/davidcann/zrate/web/server/internal/agent"
	"github.com/davidcann/zrate/web/server/internal/db"
)

const (
	workingCopyCommitHash        = "working-copy"
	commitWindowLookaheadMs      = int64(5 * 60 * 1000)
	maxCommitsPerProject         = 200
	commitsPageSize              = 20
	currentCommitCoverageVersion = 4
)

var defaultMessageWindowMs = func() int64 {
	if v := os.Getenv("ZRATE_MESSAGE_WINDOW_HOURS"); v != "" {
		if hours, err := strconv.ParseInt(v, 10, 64); err == nil && hours > 0 {
			log.Printf("using custom message window: %d hours", hours)
			return hours * 60 * 60 * 1000
		}
	}
	return int64(7 * 24 * 60 * 60 * 1000) // 7 days
}()

type projectCommitsResponse struct {
	Branch       string                  `json:"branch"`
	Branches     []string                `json:"branches"`
	CurrentUser  string                  `json:"currentUser"`
	CurrentEmail string                  `json:"currentEmail"`
	Summary      projectCommitSummary    `json:"summary"`
	Commits      []projectCommitCoverage `json:"commits"`
}

type agentCoverageSegment struct {
	Agent          string  `json:"agent"`
	LinesFromAgent int     `json:"linesFromAgent"`
	CharsFromAgent int     `json:"charsFromAgent"`
	LinePercent    float64 `json:"linePercent"`
}

type projectCommitSummary struct {
	CommitCount      int                    `json:"commitCount"`
	LinesTotal       int                    `json:"linesTotal"`
	LinesFromAgent   int                    `json:"linesFromAgent"`
	LinePercent      float64                `json:"linePercent"`
	CharsTotal       int                    `json:"charsTotal"`
	CharsFromAgent   int                    `json:"charsFromAgent"`
	CharacterPercent float64                `json:"characterPercent"`
	AgentSegments    []agentCoverageSegment `json:"agentSegments,omitempty"`
}

type projectCommitCoverage struct {
	WorkingCopy      bool                   `json:"workingCopy"`
	ProjectID        string                 `json:"projectId"`
	ProjectLabel     string                 `json:"projectLabel"`
	ProjectPath      string                 `json:"projectPath"`
	ProjectGitID     string                 `json:"projectGitId"`
	CommitHash       string                 `json:"commitHash"`
	Subject          string                 `json:"subject"`
	AuthoredAtUnixMs int64                  `json:"authoredAtUnixMs"`
	LinesTotal       int                    `json:"linesTotal"`
	LinesFromAgent   int                    `json:"linesFromAgent"`
	LinePercent      float64                `json:"linePercent"`
	CharsTotal       int                    `json:"charsTotal"`
	CharsFromAgent   int                    `json:"charsFromAgent"`
	CharacterPercent float64                `json:"characterPercent"`
	LinesAdded       int                    `json:"linesAdded"`
	LinesRemoved     int                    `json:"linesRemoved"`
	AgentSegments    []agentCoverageSegment `json:"agentSegments,omitempty"`
}

type projectCommitDetailResponse struct {
	Branch    string                      `json:"branch"`
	Branches  []string                    `json:"branches"`
	CommitURL string                      `json:"commitUrl"`
	Commit    projectCommitCoverage       `json:"commit"`
	Diff      string                      `json:"diff"`
	Files     []commitFileCoverage        `json:"files"`
	Messages  []commitContributionMessage `json:"messages"`
}

type projectCommitPageResponse struct {
	Branch       string                  `json:"branch"`
	Branches     []string                `json:"branches"`
	Users        []db.UserInfo            `json:"users"`
	UserFilter   string                  `json:"userFilter"`
	CurrentUser  string                  `json:"currentUser"`
	CurrentEmail string                  `json:"currentEmail"`
	Project      db.Project              `json:"project"`
	Refresh      commitRefreshState      `json:"refresh"`
	Summary      projectCommitSummary    `json:"summary"`
	DailySummary []dailyCommitSummary    `json:"dailySummary"`
	Pagination   projectCommitPagination `json:"pagination"`
	Commits      []projectCommitCoverage `json:"commits"`
}

type commitRefreshState struct {
	State          string `json:"state"`
	IsStale        bool   `json:"isStale"`
	LastStartedAt  int64  `json:"lastStartedAt"`
	LastFinishedAt int64  `json:"lastFinishedAt"`
	LastDurationMs int64  `json:"lastDurationMs"`
	LastError      string `json:"lastError"`
}

type dailyCommitSummary struct {
	Date           string                 `json:"date"`
	LinesTotal     int                    `json:"linesTotal"`
	LinesFromAgent int                    `json:"linesFromAgent"`
	LinePercent    float64                `json:"linePercent"`
	AgentSegments  []agentCoverageSegment `json:"agentSegments,omitempty"`
	Commits        []dailyCommitRef       `json:"commits"`
}

type dailyCommitRef struct {
	CommitHash string `json:"commitHash"`
	Subject    string `json:"subject"`
	ProjectID  string `json:"projectId"`
}

type projectCommitPagination struct {
	Page       int `json:"page"`
	PageSize   int `json:"pageSize"`
	Total      int `json:"total"`
	TotalPages int `json:"totalPages"`
}

type commitContributionMessage struct {
	ID                string `json:"id"`
	Timestamp         int64  `json:"timestamp"`
	ConversationID    string `json:"conversationId"`
	ConversationTitle string `json:"conversationTitle"`
	Agent             string `json:"agent"`
	Model             string `json:"model"`
	Content           string `json:"content"`
	LinesMatched      int    `json:"linesMatched"`
	CharsMatched      int    `json:"charsMatched"`
}

type commitFileCoverage struct {
	Path            string                 `json:"path"`
	Added           int                    `json:"added"`
	Removed         int                    `json:"removed"`
	Ignored         bool                   `json:"ignored"`
	Moved           bool                   `json:"moved"`
	MovedFrom       string                 `json:"movedFrom"`
	CopiedFromAgent bool                   `json:"copiedFromAgent"`
	LinesTotal      int                    `json:"linesTotal"`
	LinesFromAgent  int                    `json:"linesFromAgent"`
	LinePercent     float64                `json:"linePercent"`
	AgentSegments   []agentCoverageSegment `json:"agentSegments,omitempty"`
}

type gitIdentity struct {
	Name  string
	Email string
}

type gitCommit struct {
	Hash          string
	Subject       string
	UserName      string
	UserEmail     string
	TimestampUnix int64
}

type projectGroup struct {
	GitID    string
	Projects []db.Project
}

type messageDiff struct {
	ID                string
	Timestamp         int64
	ConversationID    string
	ConversationTitle string
	Agent             string
	Model             string
	Content           string
	Tokens            []diffToken
}

type diffToken struct {
	Path         string
	Sign         byte
	Norm         string
	Key          string
	Chars        int
	Attributable bool
}

type tokenSource struct {
	msgIdx   int
	tokenPos int
}

const maxFormattingWindowLines = 5

func (s *Server) handleListProjectCommits(w http.ResponseWriter, r *http.Request) {
	branch := strings.TrimSpace(r.URL.Query().Get("branch"))
	if branch == "" {
		branch = "main"
	}
	projects, err := db.ListProjects(r.Context(), s.DB, false)
	if err != nil {
		log.Printf("error listing projects for commits: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list projects")
		return
	}

	groups := groupProjectsByGitID(projects)
	all := make([]projectCommitCoverage, 0, 64)

	var currentUser, currentEmail string

	// Build a map from project ID to project for labeling.
	projectMap := make(map[string]*db.Project)
	for _, group := range groups {
		repoProject, err := resolveRepoProject(r.Context(), group)
		if err != nil {
			continue
		}
		projectMap[repoProject.ID] = repoProject

		defaultBranch := ensureProjectDefaultBranch(r.Context(), s.DB, repoProject)
		if defaultBranch != "" && branch == "main" {
			branch = defaultBranch
		}

		identity, err := resolveGitIdentity(r.Context(), repoProject.Path)
		if err != nil {
			continue
		}
		if currentUser == "" {
			currentUser = identity.Name
			currentEmail = identity.Email
		}

		// Trigger default ingestion if needed.
		if err := IngestDefaultCommits(r.Context(), s.DB, repoProject, group, identity, branch); err != nil {
			log.Printf("warning: default commit ingestion failed for %s: %v", repoProject.Path, err)
		}

		// Read commits from database.
		dbCommits, err := db.ListCommitsByProjectIDs(r.Context(), s.DB, projectIDs(group), branch)
		if err != nil {
			log.Printf("error listing db commits for %s: %v", repoProject.Path, err)
			continue
		}
		// Collect commit IDs for bulk agent coverage lookup.
		commitIDs := make([]string, 0, len(dbCommits))
		for _, c := range dbCommits {
			commitIDs = append(commitIDs, c.ID)
		}
		agentCovMap, _ := db.ListCommitAgentCoverageByCommitIDs(r.Context(), s.DB, commitIDs)

		for _, c := range dbCommits {
			rp := projectMap[c.ProjectID]
			if rp == nil {
				rp = repoProject
			}
			cov := dbCommitToCoverage(c, rp)
			if segs := agentSegmentsFromDBCoverage(agentCovMap[c.ID], c.LinesTotal); len(segs) > 0 {
				cov.AgentSegments = segs
			}
			all = append(all, cov)
		}
	}

	sort.SliceStable(all, func(i, j int) bool {
		if all[i].AuthoredAtUnixMs != all[j].AuthoredAtUnixMs {
			return all[i].AuthoredAtUnixMs > all[j].AuthoredAtUnixMs
		}
		return all[i].CommitHash > all[j].CommitHash
	})

	summary := summarizeCommitCoverage(all)
	writeSuccess(w, http.StatusOK, projectCommitsResponse{
		Branch:       branch,
		Branches:     []string{branch},
		CurrentUser:  currentUser,
		CurrentEmail: currentEmail,
		Summary:      summary,
		Commits:      all,
	})
}

func (s *Server) handleGetProjectCommit(w http.ResponseWriter, r *http.Request) {
	branch := strings.TrimSpace(r.URL.Query().Get("branch"))
	projectID := strings.TrimSpace(r.PathValue("projectId"))
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project id is required")
		return
	}
	commitHash := strings.TrimSpace(r.PathValue("commitHash"))
	if commitHash == "" {
		writeError(w, http.StatusBadRequest, "commit hash is required")
		return
	}

	project, err := getProjectByID(r.Context(), s.DB, projectID)
	if err != nil {
		log.Printf("error loading project %s: %v", projectID, err)
		writeError(w, http.StatusInternalServerError, "failed to load project")
		return
	}
	if project == nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if branch == "" {
		branch = strings.TrimSpace(project.DefaultBranch)
		if branch == "" {
			branch = "main"
		}
	}

	groups, err := listAllProjectGroups(r.Context(), s.DB)
	if err != nil {
		log.Printf("error listing project groups: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list projects")
		return
	}

	group, ok := findProjectGroupByProjectID(groups, project.ID)
	if !ok {
		writeError(w, http.StatusNotFound, "project group not found")
		return
	}

	repoProject, err := resolveRepoProject(r.Context(), group)
	if err != nil {
		writeError(w, http.StatusNotFound, "repository for project not found")
		return
	}

	defaultBranch := ensureProjectDefaultBranch(r.Context(), s.DB, repoProject)
	if branch == "" && defaultBranch != "" {
		branch = defaultBranch
	}
	branches, _ := listRepoBranches(r.Context(), repoProject.Path, defaultBranch)
	remote := ensureProjectRemote(r.Context(), s.DB, repoProject)

	identity, err := resolveGitIdentity(r.Context(), repoProject.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, "git identity not configured for project")
		return
	}

	commits, err := listCommitsByIdentity(r.Context(), repoProject.Path, branch, identity)
	if err != nil {
		log.Printf("error listing commits for %s: %v", repoProject.Path, err)
		writeError(w, http.StatusInternalServerError, "failed to list commits")
		return
	}

	ignorePatterns := groupIgnoreDiffPatterns(group)

	if commitHash == workingCopyCommitHash {
		coverage, messages, diffText, files, ok := computeWorkingCopyDetail(
			r.Context(),
			s.DB,
			repoProject,
			projectIDs(group),
			ignorePatterns,
			commits,
		)
		if !ok {
			writeError(w, http.StatusNotFound, "working copy is clean")
			return
		}
		writeSuccess(w, http.StatusOK, projectCommitDetailResponse{
			Branch:   branch,
			Commit:   coverage,
			Diff:     diffText,
			Files:    files,
			Messages: messages,
		})
		return
	}

	// Try to load from database first.
	dbCommit, err := db.GetCommitByHash(r.Context(), s.DB, repoProject.ID, branch, commitHash)
	if err != nil {
		log.Printf("error checking db for commit %s: %v", commitHash, err)
	}

	var commit gitCommit
	var commitDiff string
	var tokenDiff string

	if dbCommit != nil {
		// Use stored diff from database.
		commit = gitCommit{
			Hash:          dbCommit.CommitHash,
			Subject:       dbCommit.Subject,
			UserName:    dbCommit.UserName,
			UserEmail:   dbCommit.UserEmail,
			TimestampUnix: dbCommit.AuthoredAt,
		}
		commitDiff = dbCommit.DiffContent
		// Prefer stored unified diff tokens when commit is already ingested.
		tokenDiff = commitDiff
		// If stored diff content is missing, recover from git so detail view
		// and recomputed attribution remain consistent with list coverage.
		if strings.TrimSpace(commitDiff) == "" {
			rawDiff, gitErr := runGit(
				r.Context(),
				repoProject.Path,
				"show",
				"--pretty=format:",
				"-M",
				"-w",
				"--ignore-blank-lines",
				commit.Hash,
			)
			if gitErr == nil {
				commitDiff = stripBinaryDiffs(rawDiff)
			}
			tokenDiff, err = runGit(
				r.Context(),
				repoProject.Path,
				"show",
				"--pretty=format:",
				"--unified=0",
				"-M",
				"-w",
				"--ignore-blank-lines",
				commit.Hash,
			)
			if err != nil {
				tokenDiff = commitDiff
			}
		}
	} else {
		// Fallback to git for commits not yet ingested.
		commitIdx := -1
		for i := range commits {
			if commits[i].Hash == commitHash {
				commitIdx = i
				break
			}
		}
		if commitIdx < 0 {
			writeError(w, http.StatusNotFound, "commit not found for current user")
			return
		}
		commit = commits[commitIdx]

		rawDiff, gitErr := runGit(
			r.Context(),
			repoProject.Path,
			"show",
			"--pretty=format:",
			"-M",
			"-w",
			"--ignore-blank-lines",
			commit.Hash,
		)
		if gitErr != nil {
			log.Printf("error loading commit diff %s: %v", commit.Hash, gitErr)
			writeError(w, http.StatusNotFound, "commit diff not found")
			return
		}
		commitDiff = stripBinaryDiffs(rawDiff)

		// Use unified=0 when available to improve token precision.
		tokenDiff, err = runGit(
			r.Context(),
			repoProject.Path,
			"show",
			"--pretty=format:",
			"--unified=0",
			"-M",
			"-w",
			"--ignore-blank-lines",
			commit.Hash,
		)
		if err != nil {
			// Fall back to the regular commit diff instead of zeroing coverage.
			tokenDiff = commitDiff
		}
	}

	commitTokens := parseUnifiedDiffTokens(tokenDiff, ignorePatterns)

	// Determine the time window for message matching.
	windowStart := commit.TimestampUnix*1000 - defaultMessageWindowMs
	windowEnd := commit.TimestampUnix*1000 + commitWindowLookaheadMs

	messages, err := listDerivedDiffMessages(r.Context(), s.DB, projectIDs(group), windowStart, windowEnd)
	if err != nil {
		log.Printf("error listing derived diff messages: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load matching messages")
		return
	}

	contribMessages, matchedLines, matchedChars, fileAgent, remainingNorms := attributeCommitToMessages(commitTokens, messages, windowStart, windowEnd)
	totalLines, totalChars := tokenTotals(commitTokens)
	files, fallbackLines, fallbackChars := summarizeDiffFiles(commitDiff, ignorePatterns, commitTokens, fileAgent, remainingNorms)
	matchedLines += fallbackLines
	matchedChars += fallbackChars
	detailAdded, detailRemoved := countDiffAddedRemoved(commitDiff)

	agentSegments := agentSegmentsFromContribs(contribMessages, totalLines)
	if len(agentSegments) == 0 && matchedLines > 0 {
		agentSegments = attributeCopiedFromAgentFiles(files, commitTokens, messages, windowStart, windowEnd, totalLines)
	}

	writeSuccess(w, http.StatusOK, projectCommitDetailResponse{
		Branch:    branch,
		Branches:  branches,
		CommitURL: commitURL(remote, commit.Hash),
		Commit: projectCommitCoverage{
			ProjectID:        project.ID,
			ProjectLabel:     project.Label,
			ProjectPath:      project.Path,
			ProjectGitID:     project.GitID,
			CommitHash:       commit.Hash,
			Subject:          commit.Subject,
			AuthoredAtUnixMs: commit.TimestampUnix * 1000,
			LinesTotal:       totalLines,
			LinesFromAgent:   matchedLines,
			LinePercent:      percentage(matchedLines, totalLines),
			CharsTotal:       totalChars,
			CharsFromAgent:   matchedChars,
			CharacterPercent: percentage(matchedChars, totalChars),
			LinesAdded:       detailAdded,
			LinesRemoved:     detailRemoved,
			AgentSegments:    agentSegments,
		},
		Diff:     commitDiff,
		Files:    files,
		Messages: contribMessages,
	})
}

func (s *Server) handleListProjectCommitsForProject(w http.ResponseWriter, r *http.Request) {
	branch := strings.TrimSpace(r.URL.Query().Get("branch"))
	projectID := strings.TrimSpace(r.PathValue("projectId"))
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project id is required")
		return
	}

	page := parsePositiveInt(r.URL.Query().Get("page"), 1)
	pageSize := parsePositiveInt(r.URL.Query().Get("pageSize"), commitsPageSize)

	// Client timezone offset in minutes (JS getTimezoneOffset convention).
	// UTC+7 sends -420, UTC-8 sends 480. Default to 0 (UTC).
	tzOffsetMin := 0
	if raw := r.URL.Query().Get("tzOffset"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			tzOffsetMin = v
		}
	}
	clientLoc := time.FixedZone("client", -tzOffsetMin*60)

	userFilter := strings.TrimSpace(r.URL.Query().Get("user"))

	project, err := getProjectByID(r.Context(), s.DB, projectID)
	if err != nil {
		log.Printf("error loading project %s: %v", projectID, err)
		writeError(w, http.StatusInternalServerError, "failed to load project")
		return
	}
	if project == nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if branch == "" {
		branch = strings.TrimSpace(project.DefaultBranch)
		if branch == "" {
			branch = "main"
		}
	}

	groups, err := listAllProjectGroups(r.Context(), s.DB)
	if err != nil {
		log.Printf("error listing project groups: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list projects")
		return
	}
	group, ok := findProjectGroupByProjectID(groups, project.ID)
	if !ok {
		writeError(w, http.StatusNotFound, "project group not found")
		return
	}

	repoProject, err := resolveRepoProject(r.Context(), group)
	if err != nil {
		writeError(w, http.StatusNotFound, "repository for project not found")
		return
	}
	defaultBranch := ensureProjectDefaultBranch(r.Context(), s.DB, repoProject)
	if branch == "" && defaultBranch != "" {
		branch = defaultBranch
	}
	branches, _ := listRepoBranches(r.Context(), repoProject.Path, defaultBranch)
	users, _ := db.ListDistinctUsers(r.Context(), s.DB, repoProject.ID, branch)

	identity, err := resolveGitIdentity(r.Context(), repoProject.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, "git identity not configured for project")
		return
	}

	syncState, err := db.GetCommitSyncState(r.Context(), s.DB, repoProject.ID, branch)
	if err != nil {
		log.Printf("error loading commit sync state for %s: %v", repoProject.Path, err)
	}

	// Read commits from database.
	total, err := db.CountCommitsByProject(r.Context(), s.DB, repoProject.ID, branch)
	if err != nil {
		log.Printf("error counting commits for %s: %v", repoProject.Path, err)
		writeError(w, http.StatusInternalServerError, "failed to count commits")
		return
	}
	if total == 0 {
		if err := IngestDefaultCommits(r.Context(), s.DB, repoProject, group, identity, branch); err != nil {
			log.Printf("warning: seed commit ingestion failed for %s: %v", repoProject.Path, err)
		}
		total, err = db.CountCommitsByProject(r.Context(), s.DB, repoProject.ID, branch)
		if err != nil {
			log.Printf("error counting commits after seed ingest for %s: %v", repoProject.Path, err)
			writeError(w, http.StatusInternalServerError, "failed to count commits")
			return
		}
	}

	if head, headErr := latestCommitByIdentity(r.Context(), repoProject.Path, branch, identity); headErr == nil && head != nil {
		latest, getErr := db.GetCommitByHash(r.Context(), s.DB, repoProject.ID, branch, head.Hash)
		if getErr != nil || latest == nil {
			if err := IngestDefaultCommits(r.Context(), s.DB, repoProject, group, identity, branch); err != nil {
				log.Printf("warning: latest-head ingestion failed for %s: %v", repoProject.Path, err)
			} else if recount, countErr := db.CountCommitsByProject(r.Context(), s.DB, repoProject.ID, branch); countErr == nil {
				total = recount
			}
		}
	}

	staleCoverage := false
	staleCoverage, err = db.HasStaleCommitCoverage(r.Context(), s.DB, repoProject.ID, branch, currentCommitCoverageVersion)
	if err != nil {
		log.Printf("error checking stale coverage for %s: %v", repoProject.Path, err)
		staleCoverage = false
	} else if staleCoverage {
		if _, recErr := recomputeCommitCoverageForProject(r.Context(), s.DB, repoProject, group, branch); recErr != nil {
			log.Printf("error recomputing stale coverage for %s: %v", repoProject.Path, recErr)
		} else {
			staleCoverage = false
		}
	}

	// Compute filtered total for pagination when an author filter is active.
	filteredTotal := total
	if userFilter != "" {
		filteredTotal, err = db.CountCommitsByProjectAndUser(r.Context(), s.DB, repoProject.ID, branch, userFilter)
		if err != nil {
			log.Printf("error counting filtered commits for %s: %v", repoProject.Path, err)
			writeError(w, http.StatusInternalServerError, "failed to count commits")
			return
		}
	}

	totalPages := 0
	if filteredTotal > 0 {
		totalPages = (filteredTotal + pageSize - 1) / pageSize
	}
	if totalPages > 0 && page > totalPages {
		page = totalPages
	}
	offset := (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}

	dbCommits, err := db.ListCommitsByProjectAndUser(r.Context(), s.DB, repoProject.ID, branch, userFilter, pageSize, offset)
	if err != nil {
		log.Printf("error listing commits from db for %s: %v", repoProject.Path, err)
		writeError(w, http.StatusInternalServerError, "failed to list commits")
		return
	}

	// Collect all commit IDs for agent coverage lookup.
	allDBCommits, err := db.ListCommitsByProjectAndUser(r.Context(), s.DB, repoProject.ID, branch, userFilter, filteredTotal, 0)
	if err != nil {
		log.Printf("error listing all commits from db for %s: %v", repoProject.Path, err)
		writeError(w, http.StatusInternalServerError, "failed to list commits")
		return
	}
	allCommitIDs := make([]string, 0, len(allDBCommits))
	for _, c := range allDBCommits {
		allCommitIDs = append(allCommitIDs, c.ID)
	}
	agentCovMap, _ := db.ListCommitAgentCoverageByCommitIDs(r.Context(), s.DB, allCommitIDs)

	// Convert DB commits to coverage structs.
	paged := make([]projectCommitCoverage, 0, len(dbCommits))
	for _, c := range dbCommits {
		cov := dbCommitToCoverage(c, repoProject)
		if segs := agentSegmentsFromDBCoverage(agentCovMap[c.ID], c.LinesTotal); len(segs) > 0 {
			cov.AgentSegments = segs
		} else if c.LinesFromAgent > 0 {
			cov.AgentSegments = []agentCoverageSegment{{
				Agent: "unknown", LinesFromAgent: c.LinesFromAgent,
				LinePercent: percentage(c.LinesFromAgent, c.LinesTotal),
			}}
		}
		paged = append(paged, cov)
	}

	// Compute summary from all DB commits.
	allCoverage := make([]projectCommitCoverage, 0, len(allDBCommits))
	for _, c := range allDBCommits {
		cov := dbCommitToCoverage(c, repoProject)
		if segs := agentSegmentsFromDBCoverage(agentCovMap[c.ID], c.LinesTotal); len(segs) > 0 {
			cov.AgentSegments = segs
		} else if c.LinesFromAgent > 0 {
			cov.AgentSegments = []agentCoverageSegment{{
				Agent: "unknown", LinesFromAgent: c.LinesFromAgent,
				LinePercent: percentage(c.LinesFromAgent, c.LinesTotal),
			}}
		}
		allCoverage = append(allCoverage, cov)
	}

	// Add working copy on page 1 when no author filter or filter matches current identity.
	if page == 1 && (userFilter == "" || strings.EqualFold(userFilter, identity.Email)) {
		if wc, ok := hasWorkingCopyChanges(r.Context(), repoProject); ok {
			paged = append([]projectCommitCoverage{wc}, paged...)
		}
	}

	refreshQueued := false
	if shouldQueueCommitRefresh(r.Context(), s.DB, repoProject, identity, branch, total, syncState, staleCoverage) {
		refreshQueued, _ = s.enqueueCommitRefresh(*repoProject, group, identity, branch)
		if refreshQueued {
			syncState = &db.CommitSyncState{
				ProjectID:  repoProject.ID,
				BranchName: branch,
				State:      "queued",
			}
		}
	}

	writeSuccess(w, http.StatusOK, projectCommitPageResponse{
		Branch:       branch,
		Branches:     branches,
		Users:        users,
		UserFilter:   userFilter,
		CurrentUser:  identity.Name,
		CurrentEmail: identity.Email,
		Project:      *project,
		Refresh:      makeCommitRefreshState(syncState, staleCoverage),
		Summary:      summarizeCommitCoverage(allCoverage),
		DailySummary: buildDailySummary(allCoverage, 30, clientLoc),
		Pagination: projectCommitPagination{
			Page:       page,
			PageSize:   pageSize,
			Total:      filteredTotal,
			TotalPages: totalPages,
		},
		Commits: paged,
	})
}

func dbCommitToCoverage(c db.Commit, repoProject *db.Project) projectCommitCoverage {
	return projectCommitCoverage{
		ProjectID:        repoProject.ID,
		ProjectLabel:     repoProject.Label,
		ProjectPath:      repoProject.Path,
		ProjectGitID:     repoProject.GitID,
		CommitHash:       c.CommitHash,
		Subject:          c.Subject,
		AuthoredAtUnixMs: c.AuthoredAt * 1000,
		LinesTotal:       c.LinesTotal,
		LinesFromAgent:   c.LinesFromAgent,
		LinePercent:      percentage(c.LinesFromAgent, c.LinesTotal),
		CharsTotal:       c.CharsTotal,
		CharsFromAgent:   c.CharsFromAgent,
		CharacterPercent: percentage(c.CharsFromAgent, c.CharsTotal),
		LinesAdded:       c.LinesAdded,
		LinesRemoved:     c.LinesRemoved,
	}
}

func listAllProjectGroups(ctx context.Context, database *sql.DB) ([]projectGroup, error) {
	active, err := db.ListProjects(ctx, database, false)
	if err != nil {
		return nil, err
	}
	ignored, err := db.ListProjects(ctx, database, true)
	if err != nil {
		return nil, err
	}
	all := append(active, ignored...)
	return groupProjectsByGitID(all), nil
}

func findProjectGroupByProjectID(groups []projectGroup, projectID string) (projectGroup, bool) {
	for _, g := range groups {
		for _, p := range g.Projects {
			if p.ID == projectID {
				return g, true
			}
		}
	}
	return projectGroup{}, false
}

func getProjectByID(ctx context.Context, database *sql.DB, projectID string) (*db.Project, error) {
	var p db.Project
	err := database.QueryRowContext(ctx, "SELECT id, path, old_paths, label, git_id, default_branch, remote, ignored, ignore_diff_paths, ignore_default_diff_paths FROM projects WHERE id = ?", projectID).Scan(
		&p.ID,
		&p.Path,
		&p.OldPaths,
		&p.Label,
		&p.GitID,
		&p.DefaultBranch,
		&p.Remote,
		&p.Ignored,
		&p.IgnoreDiffPaths,
		&p.IgnoreDefaultDiffPaths,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query project: %w", err)
	}
	return &p, nil
}

func ensureProjectRemote(ctx context.Context, database *sql.DB, project *db.Project) string {
	if project == nil {
		return ""
	}
	if project.Remote != "" {
		return project.Remote
	}
	remote, err := runGit(ctx, project.Path, "remote", "get-url", "origin")
	if err != nil {
		return ""
	}
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return ""
	}
	if err := db.UpdateProjectRemote(ctx, database, project.ID, remote); err == nil {
		project.Remote = remote
	}
	return remote
}

func ensureProjectLocalUser(ctx context.Context, database *sql.DB, p *db.ProjectDetail) {
	if p.LocalUser != "" {
		return
	}
	name, _ := runGit(ctx, p.Path, "config", "--get", "user.name")
	email, _ := runGit(ctx, p.Path, "config", "--get", "user.email")
	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)
	if name == "" && email == "" {
		return
	}
	if err := db.UpdateProjectLocalUser(ctx, database, p.ID, name, email); err == nil {
		p.LocalUser = name
		p.LocalEmail = email
	}
}

func groupProjectsByGitID(projects []db.Project) []projectGroup {
	groups := make(map[string][]db.Project)
	for _, p := range projects {
		gitID := strings.TrimSpace(p.GitID)
		if gitID == "" {
			continue
		}
		groups[gitID] = append(groups[gitID], p)
	}

	ids := make([]string, 0, len(groups))
	for gitID := range groups {
		ids = append(ids, gitID)
	}
	sort.Strings(ids)

	out := make([]projectGroup, 0, len(ids))
	for _, gitID := range ids {
		out = append(out, projectGroup{GitID: gitID, Projects: groups[gitID]})
	}
	return out
}

func projectIDs(group projectGroup) []string {
	ids := make([]string, 0, len(group.Projects))
	for _, p := range group.Projects {
		ids = append(ids, p.ID)
	}
	return ids
}

func resolveRepoProject(ctx context.Context, group projectGroup) (*db.Project, error) {
	for i := range group.Projects {
		p := group.Projects[i]
		root, err := gitRootCommit(ctx, p.Path)
		if err != nil {
			continue
		}
		if root == group.GitID {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("no repo path matched git id %q", group.GitID)
}

func gitRootCommit(ctx context.Context, path string) (string, error) {
	out, err := runGit(ctx, path, "rev-list", "--max-parents=0", "HEAD")
	if err != nil {
		return "", err
	}
	line := strings.TrimSpace(out)
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	if line == "" {
		return "", fmt.Errorf("empty root commit")
	}
	return line, nil
}

func resolveGitIdentity(ctx context.Context, path string) (gitIdentity, error) {
	name, _ := runGit(ctx, path, "config", "--get", "user.name")
	email, _ := runGit(ctx, path, "config", "--get", "user.email")
	id := gitIdentity{Name: strings.TrimSpace(name), Email: strings.TrimSpace(email)}
	if id.Name == "" && id.Email == "" {
		return gitIdentity{}, fmt.Errorf("missing git identity for %q", path)
	}
	return id, nil
}

func listCommitsByIdentity(ctx context.Context, path, branch string, identity gitIdentity) ([]gitCommit, error) {
	out, err := runGit(ctx, path,
		"log", branch,
		"--pretty=format:%H%x1f%an%x1f%ae%x1f%ct%x1f%s%x1e",
		"--reverse",
		fmt.Sprintf("--max-count=%d", maxCommitsPerProject),
	)
	if err != nil {
		return nil, err
	}

	records := strings.Split(out, "\x1e")
	commits := make([]gitCommit, 0, len(records))
	for _, rec := range records {
		rec = strings.TrimSpace(rec)
		if rec == "" {
			continue
		}
		parts := strings.Split(rec, "\x1f")
		if len(parts) < 5 {
			continue
		}
		ts, err := strconv.ParseInt(strings.TrimSpace(parts[3]), 10, 64)
		if err != nil {
			continue
		}
		c := gitCommit{
			Hash:          strings.TrimSpace(parts[0]),
			UserName:    strings.TrimSpace(parts[1]),
			UserEmail:   strings.TrimSpace(parts[2]),
			TimestampUnix: ts,
			Subject:       strings.TrimSpace(parts[4]),
		}
		if commitMatchesIdentity(c, identity) {
			commits = append(commits, c)
		}
	}
	return commits, nil
}

func latestCommitByIdentity(ctx context.Context, path, branch string, identity gitIdentity) (*gitCommit, error) {
	commits, err := listCommitsByIdentity(ctx, path, branch, identity)
	if err != nil {
		return nil, err
	}
	if len(commits) == 0 {
		return nil, nil
	}
	return &commits[len(commits)-1], nil
}

func shouldQueueCommitRefresh(
	ctx context.Context,
	database *sql.DB,
	repoProject *db.Project,
	identity gitIdentity,
	branch string,
	total int,
	syncState *db.CommitSyncState,
	staleCoverage bool,
) bool {
	if syncState != nil && (syncState.State == "queued" || syncState.State == "running") {
		return false
	}
	if total == 0 || staleCoverage {
		return true
	}
	head, err := latestCommitByIdentity(ctx, repoProject.Path, branch, identity)
	if err != nil || head == nil {
		return false
	}
	if syncState == nil {
		latest, getErr := db.GetCommitByHash(ctx, database, repoProject.ID, branch, head.Hash)
		return getErr != nil || latest == nil
	}
	if syncState.LastProcessedHeadHash == "" {
		latest, getErr := db.GetCommitByHash(ctx, database, repoProject.ID, branch, head.Hash)
		return getErr != nil || latest == nil
	}
	return syncState.LastProcessedHeadHash != head.Hash
}

func makeCommitRefreshState(syncState *db.CommitSyncState, stale bool) commitRefreshState {
	if syncState == nil {
		return commitRefreshState{
			State:   "idle",
			IsStale: stale,
		}
	}
	state := strings.TrimSpace(syncState.State)
	if state == "" {
		state = "idle"
	}
	return commitRefreshState{
		State:          state,
		IsStale:        stale || state == "failed",
		LastStartedAt:  syncState.LastStartedAtMs,
		LastFinishedAt: syncState.LastFinishedAtMs,
		LastDurationMs: syncState.LastDurationMs,
		LastError:      syncState.LastError,
	}
}

func commitMatchesIdentity(c gitCommit, identity gitIdentity) bool {
	if identity.Email != "" {
		return strings.EqualFold(strings.TrimSpace(c.UserEmail), identity.Email)
	}
	if identity.Name != "" {
		return strings.TrimSpace(c.UserName) == identity.Name
	}
	return false
}

func computeCoverageForRepo(
	ctx context.Context,
	database *sql.DB,
	repoProject *db.Project,
	projectIDs []string,
	ignorePatterns []string,
	commits []gitCommit,
) ([]projectCommitCoverage, error) {
	if len(commits) == 0 {
		return nil, nil
	}

	firstTs := commits[0].TimestampUnix*1000 - defaultMessageWindowMs
	lastTs := commits[len(commits)-1].TimestampUnix*1000 + commitWindowLookaheadMs
	messages, err := listDerivedDiffMessages(ctx, database, projectIDs, firstTs, lastTs)
	if err != nil {
		return nil, err
	}

	coverage := make([]projectCommitCoverage, 0, len(commits))
	for _, c := range commits {
		commitDiff, err := runGit(ctx, repoProject.Path, "show", "--pretty=format:", "--unified=0", "-w", "--ignore-blank-lines", c.Hash)
		if err != nil {
			continue
		}
		commitTokens := parseUnifiedDiffTokens(commitDiff, ignorePatterns)
		if len(commitTokens) == 0 {
			continue
		}

		windowStart := c.TimestampUnix*1000 - defaultMessageWindowMs
		windowEnd := c.TimestampUnix*1000 + commitWindowLookaheadMs

		totalLines, totalChars := tokenTotals(commitTokens)
		_, matchedLines, matchedChars, fileAgent, remainingNorms := attributeCommitToMessages(commitTokens, messages, windowStart, windowEnd)
		_, fallbackLines, fallbackChars := summarizeDiffFiles(commitDiff, ignorePatterns, commitTokens, fileAgent, remainingNorms)
		matchedLines += fallbackLines
		matchedChars += fallbackChars

		cAdded, cRemoved := countDiffAddedRemoved(commitDiff)
		coverage = append(coverage, projectCommitCoverage{
			ProjectID:        repoProject.ID,
			ProjectLabel:     repoProject.Label,
			ProjectPath:      repoProject.Path,
			ProjectGitID:     repoProject.GitID,
			CommitHash:       c.Hash,
			Subject:          c.Subject,
			AuthoredAtUnixMs: c.TimestampUnix * 1000,
			LinesTotal:       totalLines,
			LinesFromAgent:   matchedLines,
			LinePercent:      percentage(matchedLines, totalLines),
			CharsTotal:       totalChars,
			CharsFromAgent:   matchedChars,
			CharacterPercent: percentage(matchedChars, totalChars),
			LinesAdded:       cAdded,
			LinesRemoved:     cRemoved,
		})
	}

	return coverage, nil
}

func listDerivedDiffMessages(ctx context.Context, database *sql.DB, projectIDs []string, minTs, maxTs int64) ([]messageDiff, error) {
	if len(projectIDs) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(projectIDs)), ",")
	query := fmt.Sprintf(
		`SELECT m.id, m.timestamp, m.conversation_id, c.title, c.agent, m.model, m.content, m.raw_json
		 FROM messages m
		 JOIN conversations c ON c.id = m.conversation_id
		 WHERE m.role = 'agent'
		   AND m.timestamp BETWEEN ? AND ?
		   AND m.project_id IN (%s)
		 ORDER BY m.timestamp, m.id`,
		placeholders,
	)
	args := make([]any, 0, len(projectIDs)+2)
	args = append(args, minTs, maxTs)
	for _, id := range projectIDs {
		args = append(args, id)
	}

	rows, err := database.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query derived diff messages: %w", err)
	}
	defer rows.Close()

	messages := make([]messageDiff, 0, 64)
	for rows.Next() {
		var m messageDiff
		var rawJSON string
		if err := rows.Scan(&m.ID, &m.Timestamp, &m.ConversationID, &m.ConversationTitle, &m.Agent, &m.Model, &m.Content, &rawJSON); err != nil {
			return nil, fmt.Errorf("scan derived diff message: %w", err)
		}
		if strings.TrimSpace(m.Model) == "" {
			m.Model = detectModelFromJSON(rawJSON)
		}

		diff, ok := agent.ExtractReliableDiff(m.Content)
		if !ok {
			diff, ok = agent.ExtractReliableDiffFromJSON(rawJSON)
		}
		if !ok {
			continue
		}
		m.Tokens = parseUnifiedDiffTokens(diff, nil)
		if len(m.Tokens) == 0 {
			continue
		}
		messages = append(messages, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate derived diff messages: %w", err)
	}
	return messages, nil
}

// countDiffAddedRemoved counts the total lines added and removed from a unified diff.
func countDiffAddedRemoved(diffText string) (added, removed int) {
	for _, line := range strings.Split(diffText, "\n") {
		switch {
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			added++
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			removed++
		}
	}
	return added, removed
}

func tokenTotals(tokens []diffToken) (int, int) {
	lines := 0
	chars := 0
	for _, tok := range tokens {
		if !tok.Attributable {
			continue
		}
		lines++
		chars += tok.Chars
	}
	return lines, chars
}

func attributeCommitToMessages(
	commitTokens []diffToken,
	messages []messageDiff,
	windowStart, windowEnd int64,
) ([]commitContributionMessage, int, int, map[string]commitFileCoverage, map[string]int) {
	// Sort messages newest-first so that when multiple messages contain the
	// same token, the most recent message wins attribution.
	sort.SliceStable(messages, func(i, j int) bool {
		if messages[i].Timestamp != messages[j].Timestamp {
			return messages[i].Timestamp > messages[j].Timestamp
		}
		return messages[i].ID > messages[j].ID
	})

	matchedLines := 0
	matchedChars := 0
	tokenSources := make(map[string][]tokenSource)
	messageTokensByBucket := make(map[int]map[string][]int)
	messageTokenUsed := make(map[int][]bool)
	commitMatched := make([]bool, len(commitTokens))
	// Keep a full multiset of normalized message lines for copied-file detection.
	// This must not be decremented by exact path matches, otherwise copied files
	// can be severely under-attributed when the same lines also appear elsewhere.
	normSources := make(map[string]int)
	for i, msg := range messages {
		if msg.Timestamp <= windowStart || msg.Timestamp > windowEnd {
			continue
		}
		pathTokens := make(map[string][]int)
		messageTokenUsed[i] = make([]bool, len(msg.Tokens))
		for pos, tok := range msg.Tokens {
			if !tok.Attributable {
				continue
			}
			tokenSources[tok.Key] = append(tokenSources[tok.Key], tokenSource{msgIdx: i, tokenPos: pos})
			pathTokens[tokenBucketKey(tok.Path, tok.Sign)] = append(pathTokens[tokenBucketKey(tok.Path, tok.Sign)], pos)
			if tok.Norm != "" {
				normSources[tok.Norm]++
			}
		}
		messageTokensByBucket[i] = pathTokens
	}

	contribByIndex := make(map[int]*commitContributionMessage)
	fileCoverageByPath := make(map[string]commitFileCoverage)
	type fileAgentStats struct {
		lines int
		chars int
	}
	fileAgentByPath := make(map[string]map[string]*fileAgentStats)
	recordFileAgentMatch := func(filePath string, msgIdx int, chars int) {
		if filePath == "" {
			filePath = "(unknown)"
		}
		agent := strings.TrimSpace(messages[msgIdx].Agent)
		if agent == "" {
			agent = "unknown"
		}
		byAgent := fileAgentByPath[filePath]
		if byAgent == nil {
			byAgent = make(map[string]*fileAgentStats)
			fileAgentByPath[filePath] = byAgent
		}
		stats := byAgent[agent]
		if stats == nil {
			stats = &fileAgentStats{}
			byAgent[agent] = stats
		}
		stats.lines++
		stats.chars += chars
	}
	for tokIdx, tok := range commitTokens {
		if !tok.Attributable {
			continue
		}
		path := tok.Path
		if path == "" {
			path = "(unknown)"
		}
		fileCov := fileCoverageByPath[path]
		fileCov.Path = path
		fileCov.Added++

		sources := tokenSources[tok.Key]
		if len(sources) == 0 {
			fileCoverageByPath[path] = fileCov
			continue
		}
		source := sources[0]
		tokenSources[tok.Key] = sources[1:]
		messageTokenUsed[source.msgIdx][source.tokenPos] = true
		commitMatched[tokIdx] = true

		matchedLines++
		matchedChars += tok.Chars
		fileCov.Removed++
		fileCoverageByPath[path] = fileCov
		recordFileAgentMatch(path, source.msgIdx, tok.Chars)
		contrib := contribByIndex[source.msgIdx]
		if contrib == nil {
			msg := messages[source.msgIdx]
			contrib = &commitContributionMessage{
				ID:                msg.ID,
				Timestamp:         msg.Timestamp,
				ConversationID:    msg.ConversationID,
				ConversationTitle: msg.ConversationTitle,
				Agent:             msg.Agent,
				Model:             msg.Model,
				Content:           msg.Content,
			}
			contribByIndex[source.msgIdx] = contrib
		}
		contrib.LinesMatched++
		contrib.CharsMatched += tok.Chars
	}

	// Second pass: recover attribution for formatting-only changes that alter
	// line breaks. We compare normalized windows (up to 5 lines on either side)
	// within the same file path and allow different line counts when the joined
	// normalized content is identical.
	type tokenBucket struct {
		path string
		sign byte
	}
	commitByPath := make(map[tokenBucket][]int)
	for i, tok := range commitTokens {
		if tok.Path == "" || tok.Norm == "" || commitMatched[i] || !tok.Attributable {
			continue
		}
		commitByPath[tokenBucket{path: tok.Path, sign: tok.Sign}] = append(commitByPath[tokenBucket{path: tok.Path, sign: tok.Sign}], i)
	}

	for bucket, indices := range commitByPath {
		path := bucket.path
		bucketKey := tokenBucketKey(path, bucket.sign)
		for cursor := 0; cursor < len(indices); {
			matchedWindow := false
			maxCommitWindow := maxFormattingWindowLines
			if remaining := len(indices) - cursor; remaining < maxCommitWindow {
				maxCommitWindow = remaining
			}

			for commitWindow := maxCommitWindow; commitWindow >= 1 && !matchedWindow; commitWindow-- {
				commitNorm := concatCommitNorms(commitTokens, indices[cursor:cursor+commitWindow])
				if commitNorm == "" {
					continue
				}

				for msgIdx, msg := range messages {
					positions := messageTokensByBucket[msgIdx][bucketKey]
					if len(positions) == 0 {
						continue
					}
					maxMessageWindow := maxFormattingWindowLines
					if len(positions) < maxMessageWindow {
						maxMessageWindow = len(positions)
					}
					for messageWindow := 1; messageWindow <= maxMessageWindow && !matchedWindow; messageWindow++ {
						for start := 0; start+messageWindow <= len(positions); start++ {
							windowPositions := positions[start : start+messageWindow]
							if !messageWindowAvailable(messageTokenUsed[msgIdx], windowPositions) {
								continue
							}
							if concatMessageNorms(msg.Tokens, windowPositions) != commitNorm {
								continue
							}

							for _, idx := range indices[cursor : cursor+commitWindow] {
								commitMatched[idx] = true
								matchedLines++
								matchedChars += commitTokens[idx].Chars
								fileCov := fileCoverageByPath[path]
								fileCov.Path = path
								fileCov.Removed++
								fileCoverageByPath[path] = fileCov
								recordFileAgentMatch(path, msgIdx, commitTokens[idx].Chars)
							}

							for _, pos := range windowPositions {
								messageTokenUsed[msgIdx][pos] = true
							}

							contrib := contribByIndex[msgIdx]
							if contrib == nil {
								contrib = &commitContributionMessage{
									ID:                msg.ID,
									Timestamp:         msg.Timestamp,
									ConversationID:    msg.ConversationID,
									ConversationTitle: msg.ConversationTitle,
									Agent:             msg.Agent,
									Model:             msg.Model,
									Content:           msg.Content,
								}
								contribByIndex[msgIdx] = contrib
							}
							for _, idx := range indices[cursor : cursor+commitWindow] {
								contrib.LinesMatched++
								contrib.CharsMatched += commitTokens[idx].Chars
							}

							cursor += commitWindow
							matchedWindow = true
							break
						}
					}
					if matchedWindow {
						break
					}
				}
			}

			if !matchedWindow {
				cursor++
			}
		}
	}

	out := make([]commitContributionMessage, 0, len(contribByIndex))
	for _, contrib := range contribByIndex {
		out = append(out, *contrib)
	}
	// Sort output by ascending timestamp for consistent chronological display.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Timestamp != out[j].Timestamp {
			return out[i].Timestamp < out[j].Timestamp
		}
		return out[i].ID < out[j].ID
	})
	for filePath, byAgent := range fileAgentByPath {
		fileCov := fileCoverageByPath[filePath]
		agents := make([]string, 0, len(byAgent))
		for agent := range byAgent {
			agents = append(agents, agent)
		}
		sort.Strings(agents)
		segments := make([]agentCoverageSegment, 0, len(agents))
		for _, agent := range agents {
			stats := byAgent[agent]
			segments = append(segments, agentCoverageSegment{
				Agent:          agent,
				LinesFromAgent: stats.lines,
				CharsFromAgent: stats.chars,
			})
		}
		fileCov.AgentSegments = segments
		fileCoverageByPath[filePath] = fileCov
	}

	return out, matchedLines, matchedChars, fileCoverageByPath, normSources
}

func concatCommitNorms(tokens []diffToken, indices []int) string {
	if len(indices) == 0 {
		return ""
	}
	var b strings.Builder
	for _, idx := range indices {
		norm := tokens[idx].Norm
		if norm == "" {
			return ""
		}
		b.WriteString(norm)
	}
	return b.String()
}

func concatMessageNorms(tokens []diffToken, positions []int) string {
	if len(positions) == 0 {
		return ""
	}
	var b strings.Builder
	for _, pos := range positions {
		norm := tokens[pos].Norm
		if norm == "" {
			return ""
		}
		b.WriteString(norm)
	}
	return b.String()
}

func tokenBucketKey(path string, sign byte) string {
	return path + "\x1f" + string(sign)
}

func messageWindowAvailable(used []bool, positions []int) bool {
	for _, pos := range positions {
		if used[pos] {
			return false
		}
	}
	return true
}

func detectModelFromJSON(rawJSON string) string {
	rawJSON = strings.TrimSpace(rawJSON)
	if rawJSON == "" {
		return ""
	}
	var v any
	if err := json.Unmarshal([]byte(rawJSON), &v); err != nil {
		return ""
	}
	return findModelInJSON(v)
}

func findModelInJSON(v any) string {
	switch t := v.(type) {
	case map[string]any:
		for _, k := range []string{"model", "modelName", "model_name", "model_slug", "modelSlug"} {
			if s, ok := t[k].(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					return s
				}
			}
		}
		for _, nested := range t {
			if m := findModelInJSON(nested); m != "" {
				return m
			}
		}
	case []any:
		for _, item := range t {
			if m := findModelInJSON(item); m != "" {
				return m
			}
		}
	}
	return ""
}

func parseUnifiedDiffTokens(diff string, ignorePatterns []string) []diffToken {
	diff = strings.ReplaceAll(diff, "\r\n", "\n")
	lines := strings.Split(diff, "\n")

	oldPath := ""
	newPath := ""
	tokens := make([]diffToken, 0, 64)

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				oldPath = parseDiffPath(parts[2])
				newPath = parseDiffPath(parts[3])
			}
		case strings.HasPrefix(line, "--- "):
			oldPath = parseDiffPath(strings.TrimPrefix(line, "--- "))
		case strings.HasPrefix(line, "+++ "):
			newPath = parseDiffPath(strings.TrimPrefix(line, "+++ "))
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			if shouldIgnoreDiffPath(newPath, ignorePatterns) {
				continue
			}
			if tok, ok := makeDiffToken(newPath, '+', line[1:]); ok {
				tokens = append(tokens, tok)
			}
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			if shouldIgnoreDiffPath(oldPath, ignorePatterns) {
				continue
			}
			if tok, ok := makeDiffToken(oldPath, '-', line[1:]); ok {
				tokens = append(tokens, tok)
			}
		}
	}

	return tokens
}

func parseDiffPath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "/dev/null" {
		return ""
	}
	if i := strings.IndexAny(raw, "\t "); i >= 0 {
		raw = raw[:i]
	}
	raw = strings.TrimPrefix(raw, "a/")
	raw = strings.TrimPrefix(raw, "b/")
	return raw
}

// DefaultIgnoreDiffPaths is the hardcoded list of glob patterns ignored when
// the "Ignore default paths" option is enabled for a project.
var DefaultIgnoreDiffPaths = []string{
	"**/.git/**",
	"**/.next/**",
	"**/.nuxt/**",
	"**/__pycache__/**",
	"**/node_modules/**",
	"*.map",
	"*.min.css",
	"*.min.js",
	"bun.lockb",
	"Cargo.lock",
	"composer.lock",
	"Gemfile.lock",
	"go.sum",
	"npm-shrinkwrap.json",
	"package-lock.json",
	"packages.lock.json",
	"paket.lock",
	"pdm.lock",
	"Pipfile.lock",
	"pnpm-lock.yaml",
	"poetry.lock",
	"poetry.lock",
	"yarn.lock",
}

func groupIgnoreDiffPatterns(group projectGroup) []string {
	patternSet := make(map[string]struct{})
	patterns := make([]string, 0, 8)

	// Include default patterns if any project in the group has the flag enabled.
	for _, p := range group.Projects {
		if p.IgnoreDefaultDiffPaths {
			for _, pattern := range DefaultIgnoreDiffPaths {
				if _, exists := patternSet[pattern]; exists {
					continue
				}
				patternSet[pattern] = struct{}{}
				patterns = append(patterns, pattern)
			}
			break
		}
	}

	for _, p := range group.Projects {
		for _, pattern := range splitIgnoreDiffPatterns(p.IgnoreDiffPaths) {
			if _, exists := patternSet[pattern]; exists {
				continue
			}
			patternSet[pattern] = struct{}{}
			patterns = append(patterns, pattern)
		}
	}
	return patterns
}

func splitIgnoreDiffPatterns(raw string) []string {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		pattern := strings.TrimSpace(strings.ReplaceAll(line, "\\", "/"))
		pattern = strings.TrimPrefix(pattern, "./")
		pattern = strings.TrimPrefix(pattern, "/")
		if pattern == "" {
			continue
		}
		out = append(out, pattern)
	}
	return out
}

func shouldIgnoreDiffPath(diffPath string, patterns []string) bool {
	p := strings.TrimSpace(strings.ReplaceAll(diffPath, "\\", "/"))
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, "/")
	if p == "" || len(patterns) == 0 {
		return false
	}
	for _, pattern := range patterns {
		if globMatchPath(pattern, p) {
			return true
		}
	}
	return false
}

func globMatchPath(pattern, p string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return false
	}

	if !strings.Contains(pattern, "/") {
		for _, seg := range strings.Split(p, "/") {
			ok, err := path.Match(pattern, seg)
			if err == nil && ok {
				return true
			}
		}
	}

	return globMatchSegments(splitPathSegments(pattern), splitPathSegments(p))
}

func splitPathSegments(s string) []string {
	s = strings.Trim(strings.ReplaceAll(s, "\\", "/"), "/")
	if s == "" {
		return nil
	}
	return strings.Split(s, "/")
}

func globMatchSegments(patternSegs, pathSegs []string) bool {
	var match func(pi, si int) bool
	match = func(pi, si int) bool {
		if pi == len(patternSegs) {
			return si == len(pathSegs)
		}
		if patternSegs[pi] == "**" {
			if pi == len(patternSegs)-1 {
				return true
			}
			for skip := si; skip <= len(pathSegs); skip++ {
				if match(pi+1, skip) {
					return true
				}
			}
			return false
		}
		if si >= len(pathSegs) {
			return false
		}
		ok, err := path.Match(patternSegs[pi], pathSegs[si])
		if err != nil || !ok {
			return false
		}
		return match(pi+1, si+1)
	}
	return match(0, 0)
}

func makeDiffToken(path string, sign byte, line string) (diffToken, bool) {
	norm := normalizeWhitespace(line)
	if norm == "" {
		return diffToken{}, false
	}
	return diffToken{
		Path:         path,
		Sign:         sign,
		Norm:         norm,
		Key:          path + "\x1f" + string(sign) + "\x1f" + norm,
		Chars:        utf8.RuneCountInString(norm),
		Attributable: isAttributionCandidate(norm),
	}, true
}

func isAttributionCandidate(norm string) bool {
	for _, r := range norm {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}

func normalizeWhitespace(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func summarizeCommitCoverage(commits []projectCommitCoverage) projectCommitSummary {
	s := projectCommitSummary{CommitCount: len(commits)}
	agentTotals := make(map[string][2]int) // agent -> [lines, chars]
	for _, c := range commits {
		s.LinesTotal += c.LinesTotal
		s.LinesFromAgent += c.LinesFromAgent
		s.CharsTotal += c.CharsTotal
		s.CharsFromAgent += c.CharsFromAgent
		for _, seg := range c.AgentSegments {
			t := agentTotals[seg.Agent]
			t[0] += seg.LinesFromAgent
			t[1] += seg.CharsFromAgent
			agentTotals[seg.Agent] = t
		}
	}
	s.LinePercent = percentage(s.LinesFromAgent, s.LinesTotal)
	s.CharacterPercent = percentage(s.CharsFromAgent, s.CharsTotal)
	if len(agentTotals) > 0 {
		agents := make([]string, 0, len(agentTotals))
		for a := range agentTotals {
			agents = append(agents, a)
		}
		sort.Strings(agents)
		for _, a := range agents {
			t := agentTotals[a]
			s.AgentSegments = append(s.AgentSegments, agentCoverageSegment{
				Agent:          a,
				LinesFromAgent: t[0],
				CharsFromAgent: t[1],
				LinePercent:    percentage(t[0], s.LinesTotal),
			})
		}
	}
	return s
}

// buildDailySummary buckets commits by date in the given location and produces
// a trailing daily window ending today, newest last.
//
// The chart UX relies on at least 30 day buckets, so shorter requested windows
// are expanded to 30 days.
func buildDailySummary(allCoverage []projectCommitCoverage, days int, loc *time.Location) []dailyCommitSummary {
	if days < 30 {
		days = 30
	}

	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	// Build date-keyed buckets from commits.
	type bucket struct {
		linesTotal     int
		linesFromAgent int
		agentTotals    map[string]int // agent -> lines
		commits        []dailyCommitRef
	}
	buckets := make(map[string]*bucket)
	for _, c := range allCoverage {
		date := time.UnixMilli(c.AuthoredAtUnixMs).In(loc).Format("2006-01-02")
		b := buckets[date]
		if b == nil {
			b = &bucket{agentTotals: make(map[string]int)}
			buckets[date] = b
		}
		b.linesTotal += c.LinesTotal
		b.linesFromAgent += c.LinesFromAgent
		for _, seg := range c.AgentSegments {
			b.agentTotals[seg.Agent] += seg.LinesFromAgent
		}
		b.commits = append(b.commits, dailyCommitRef{
			CommitHash: c.CommitHash,
			Subject:    c.Subject,
			ProjectID:  c.ProjectID,
		})
	}

	out := make([]dailyCommitSummary, days)
	for i := 0; i < days; i++ {
		d := today.AddDate(0, 0, -(days - 1 - i))
		dateStr := d.Format("2006-01-02")
		ds := dailyCommitSummary{
			Date:    dateStr,
			Commits: []dailyCommitRef{},
		}
		if b, ok := buckets[dateStr]; ok {
			ds.LinesTotal = b.linesTotal
			ds.LinesFromAgent = b.linesFromAgent
			ds.LinePercent = percentage(b.linesFromAgent, b.linesTotal)
			ds.Commits = b.commits

			if len(b.agentTotals) > 0 {
				agents := make([]string, 0, len(b.agentTotals))
				for a := range b.agentTotals {
					agents = append(agents, a)
				}
				sort.Strings(agents)
				for _, a := range agents {
					ds.AgentSegments = append(ds.AgentSegments, agentCoverageSegment{
						Agent:          a,
						LinesFromAgent: b.agentTotals[a],
						LinePercent:    percentage(b.agentTotals[a], b.linesTotal),
					})
				}
			}
		}
		out[i] = ds
	}
	return out
}

// agentSegmentsFromContribs builds per-agent segments from contribution messages.
func agentSegmentsFromContribs(contribs []commitContributionMessage, linesTotal int) []agentCoverageSegment {
	if len(contribs) == 0 {
		return nil
	}
	type stats struct {
		lines int
		chars int
	}
	byAgent := make(map[string]*stats)
	for _, cm := range contribs {
		agent := cm.Agent
		if agent == "" {
			agent = "unknown"
		}
		s := byAgent[agent]
		if s == nil {
			s = &stats{}
			byAgent[agent] = s
		}
		s.lines += cm.LinesMatched
		s.chars += cm.CharsMatched
	}
	agents := make([]string, 0, len(byAgent))
	for a := range byAgent {
		agents = append(agents, a)
	}
	sort.Strings(agents)
	out := make([]agentCoverageSegment, 0, len(agents))
	for _, a := range agents {
		s := byAgent[a]
		out = append(out, agentCoverageSegment{
			Agent:          a,
			LinesFromAgent: s.lines,
			CharsFromAgent: s.chars,
			LinePercent:    percentage(s.lines, linesTotal),
		})
	}
	return out
}

// attributeCopiedFromAgentFiles assigns per-agent segments to files marked as
// copiedFromAgent by cross-referencing the file's normalized tokens against a
// norm→agent map built from messages in the commit's time window.
// It also returns the aggregate agent segments across all such files.
func attributeCopiedFromAgentFiles(
	files []commitFileCoverage,
	commitTokens []diffToken,
	messages []messageDiff,
	windowStart, windowEnd int64,
	linesTotal int,
) []agentCoverageSegment {
	// Build norm→agent map: for each normalized line, track the dominant agent.
	type agentCount struct{ counts map[string]int }
	normAgents := make(map[string]*agentCount)
	for _, msg := range messages {
		if msg.Timestamp <= windowStart || msg.Timestamp > windowEnd {
			continue
		}
		agent := strings.TrimSpace(msg.Agent)
		if agent == "" {
			agent = "unknown"
		}
		for _, tok := range msg.Tokens {
			if !tok.Attributable || tok.Norm == "" {
				continue
			}
			ac := normAgents[tok.Norm]
			if ac == nil {
				ac = &agentCount{counts: make(map[string]int)}
				normAgents[tok.Norm] = ac
			}
			ac.counts[agent]++
		}
	}
	dominantAgent := func(norm string) string {
		ac := normAgents[norm]
		if ac == nil {
			return ""
		}
		best, bestN := "", 0
		for a, n := range ac.counts {
			if n > bestN {
				best, bestN = a, n
			}
		}
		return best
	}

	// Build per-file norm lists from commit tokens.
	fileNorms := make(map[string][]string)
	for _, tok := range commitTokens {
		if tok.Path == "" || tok.Norm == "" || !tok.Attributable {
			continue
		}
		fileNorms[tok.Path] = append(fileNorms[tok.Path], tok.Norm)
	}

	overallAgentLines := make(map[string]int)
	for i, f := range files {
		if !f.CopiedFromAgent || f.LinesFromAgent == 0 || len(f.AgentSegments) > 0 {
			continue
		}
		norms := fileNorms[f.Path]
		if len(norms) == 0 {
			continue
		}
		agentLines := make(map[string]int)
		for _, norm := range norms {
			if a := dominantAgent(norm); a != "" {
				agentLines[a]++
			}
		}
		if len(agentLines) == 0 {
			continue
		}
		agents := make([]string, 0, len(agentLines))
		for a := range agentLines {
			agents = append(agents, a)
		}
		sort.Strings(agents)
		segments := make([]agentCoverageSegment, 0, len(agents))
		for _, a := range agents {
			segments = append(segments, agentCoverageSegment{
				Agent:          a,
				LinesFromAgent: agentLines[a],
				LinePercent:    percentage(agentLines[a], len(norms)),
			})
			overallAgentLines[a] += agentLines[a]
		}
		files[i].AgentSegments = segments
	}

	// Also aggregate from non-copied files that already have segments.
	for _, f := range files {
		if f.CopiedFromAgent || f.Ignored || f.Moved {
			continue
		}
		for _, seg := range f.AgentSegments {
			overallAgentLines[seg.Agent] += seg.LinesFromAgent
		}
	}

	if len(overallAgentLines) == 0 {
		return nil
	}
	agents := make([]string, 0, len(overallAgentLines))
	for a := range overallAgentLines {
		agents = append(agents, a)
	}
	sort.Strings(agents)
	out := make([]agentCoverageSegment, 0, len(agents))
	for _, a := range agents {
		out = append(out, agentCoverageSegment{
			Agent:          a,
			LinesFromAgent: overallAgentLines[a],
			LinePercent:    percentage(overallAgentLines[a], linesTotal),
		})
	}
	return out
}

// agentSegmentsFromDBCoverage converts DB agent coverage rows into API segments.
func agentSegmentsFromDBCoverage(rows []db.CommitAgentCoverage, linesTotal int) []agentCoverageSegment {
	if len(rows) == 0 {
		return nil
	}
	out := make([]agentCoverageSegment, 0, len(rows))
	for _, r := range rows {
		out = append(out, agentCoverageSegment{
			Agent:          r.Agent,
			LinesFromAgent: r.LinesFromAgent,
			CharsFromAgent: r.CharsFromAgent,
			LinePercent:    percentage(r.LinesFromAgent, linesTotal),
		})
	}
	return out
}

func percentage(part, total int) float64 {
	if total <= 0 {
		return 0
	}
	return (float64(part) * 100) / float64(total)
}

func parsePositiveInt(raw string, fallback int) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}

func hasWorkingCopyChanges(ctx context.Context, repoProject *db.Project) (projectCommitCoverage, bool) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoProject.Path, "diff", "HEAD", "--quiet")
	if err := cmd.Run(); err == nil {
		return projectCommitCoverage{}, false
	}
	return projectCommitCoverage{
		WorkingCopy:  true,
		ProjectID:    repoProject.ID,
		ProjectLabel: repoProject.Label,
		ProjectPath:  repoProject.Path,
		ProjectGitID: repoProject.GitID,
		CommitHash:   workingCopyCommitHash,
		Subject:      "Working Copy",
	}, true
}

func computeWorkingCopyDetail(
	ctx context.Context,
	database *sql.DB,
	repoProject *db.Project,
	projectIDs []string,
	ignorePatterns []string,
	commits []gitCommit,
) (projectCommitCoverage, []commitContributionMessage, string, []commitFileCoverage, bool) {
	diffText, err := runGit(
		ctx,
		repoProject.Path,
		"diff",
		"HEAD",
		"--unified=0",
		"-M",
		"-w",
		"--ignore-blank-lines",
	)
	if err != nil {
		return projectCommitCoverage{}, nil, "", nil, false
	}
	commitTokens := parseUnifiedDiffTokens(diffText, ignorePatterns)
	if len(commitTokens) == 0 {
		return projectCommitCoverage{}, nil, "", nil, false
	}

	nowMs := time.Now().UnixMilli()
	windowStart := nowMs - defaultMessageWindowMs
	if len(commits) > 0 {
		lastCommitTs := commits[len(commits)-1].TimestampUnix * 1000
		if lastCommitTs > windowStart {
			windowStart = lastCommitTs
		}
	}
	windowEnd := nowMs + commitWindowLookaheadMs

	messages, err := listDerivedDiffMessages(ctx, database, projectIDs, windowStart, windowEnd)
	if err != nil {
		return projectCommitCoverage{}, nil, "", nil, false
	}

	totalLines, totalChars := tokenTotals(commitTokens)
	contribMessages, matchedLines, matchedChars, fileAgent, remainingNorms := attributeCommitToMessages(commitTokens, messages, windowStart, windowEnd)
	files, fallbackLines, fallbackChars := summarizeDiffFiles(diffText, ignorePatterns, commitTokens, fileAgent, remainingNorms)
	matchedLines += fallbackLines
	matchedChars += fallbackChars
	wcAdded, wcRemoved := countDiffAddedRemoved(diffText)

	agentSegments := agentSegmentsFromContribs(contribMessages, totalLines)
	if len(agentSegments) == 0 && matchedLines > 0 {
		agentSegments = attributeCopiedFromAgentFiles(files, commitTokens, messages, windowStart, windowEnd, totalLines)
	}

	return projectCommitCoverage{
		WorkingCopy:      true,
		ProjectID:        repoProject.ID,
		ProjectLabel:     repoProject.Label,
		ProjectPath:      repoProject.Path,
		ProjectGitID:     repoProject.GitID,
		CommitHash:       workingCopyCommitHash,
		Subject:          "Working Copy",
		AuthoredAtUnixMs: nowMs,
		LinesTotal:       totalLines,
		LinesFromAgent:   matchedLines,
		LinePercent:      percentage(matchedLines, totalLines),
		CharsTotal:       totalChars,
		CharsFromAgent:   matchedChars,
		CharacterPercent: percentage(matchedChars, totalChars),
		LinesAdded:       wcAdded,
		LinesRemoved:     wcRemoved,
		AgentSegments:    agentSegments,
	}, contribMessages, diffText, files, true
}

func summarizeDiffFiles(
	diffText string,
	ignorePatterns []string,
	commitTokens []diffToken,
	fileAgent map[string]commitFileCoverage,
	remainingNorms map[string]int,
) ([]commitFileCoverage, int, int) {
	diffText = strings.ReplaceAll(diffText, "\r\n", "\n")
	lines := strings.Split(diffText, "\n")

	oldPath := ""
	newPath := ""
	coverageByPath := make(map[string]commitFileCoverage)

	ensure := func(path string) string {
		if path == "" {
			return ""
		}
		c := coverageByPath[path]
		c.Path = path
		c.Ignored = shouldIgnoreDiffPath(path, ignorePatterns)
		coverageByPath[path] = c
		return path
	}

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				oldPath = parseDiffPath(parts[2])
				newPath = parseDiffPath(parts[3])
				if newPath != "" {
					ensure(newPath)
				} else if oldPath != "" {
					ensure(oldPath)
				}
			}
		case strings.HasPrefix(line, "rename from "):
			oldPath = parseDiffPath(strings.TrimPrefix(line, "rename from "))
			if oldPath != "" {
				ensure(oldPath)
			}
		case strings.HasPrefix(line, "rename to "):
			newPath = parseDiffPath(strings.TrimPrefix(line, "rename to "))
			if newPath != "" {
				newPath = ensure(newPath)
				c := coverageByPath[newPath]
				c.Moved = true
				c.MovedFrom = oldPath
				coverageByPath[newPath] = c
			}
		case strings.HasPrefix(line, "--- "):
			oldPath = parseDiffPath(strings.TrimPrefix(line, "--- "))
			if oldPath != "" {
				ensure(oldPath)
			}
		case strings.HasPrefix(line, "+++ "):
			newPath = parseDiffPath(strings.TrimPrefix(line, "+++ "))
			if newPath != "" {
				ensure(newPath)
			}
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			p := newPath
			if p == "" {
				p = oldPath
			}
			p = ensure(p)
			if p == "" {
				continue
			}
			c := coverageByPath[p]
			c.Added++
			coverageByPath[p] = c
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			p := oldPath
			if p == "" {
				p = newPath
			}
			p = ensure(p)
			if p == "" {
				continue
			}
			c := coverageByPath[p]
			c.Removed++
			coverageByPath[p] = c
		}
	}

	filePaths := make([]string, 0, len(coverageByPath))
	for filePath := range coverageByPath {
		filePaths = append(filePaths, filePath)
	}
	sort.Strings(filePaths)

	fileNorms := make(map[string][]string)
	for _, tok := range commitTokens {
		path := tok.Path
		if path == "" || tok.Norm == "" || !tok.Attributable {
			continue
		}
		fileNorms[path] = append(fileNorms[path], tok.Norm)
	}

	var extraLines, extraChars int
	out := make([]commitFileCoverage, 0, len(filePaths))
	for _, filePath := range filePaths {
		c := coverageByPath[filePath]
		c.LinesTotal = c.Added + c.Removed
		if !c.Ignored {
			if agent, ok := fileAgent[filePath]; ok {
				c.LinesFromAgent = agent.Removed
				// Exact attribution uses normalized token totals so whitespace-only
				// diff lines do not lower percentages for otherwise exact matches.
				c.LinePercent = percentage(c.LinesFromAgent, agent.Added)
				if len(agent.AgentSegments) > 0 {
					segments := make([]agentCoverageSegment, 0, len(agent.AgentSegments))
					for _, seg := range agent.AgentSegments {
						if seg.LinesFromAgent <= 0 {
							continue
						}
						seg.LinePercent = percentage(seg.LinesFromAgent, agent.Added)
						segments = append(segments, seg)
					}
					c.AgentSegments = segments
				}
			}
			// Fallback: detect relocated/copied agent code by matching normalized
			// lines independent of file path. For large diffs (>=10 lines) require
			// at least 10 matched lines; for small diffs (<10 lines) require ALL
			// attributable lines to match with a minimum of 2.
			if !c.Moved && c.LinesFromAgent == 0 {
				norms := fileNorms[filePath]
				minMatch := 10
				if c.LinesTotal < 10 && len(norms) >= 2 && len(norms) < 10 {
					minMatch = len(norms)
				}
				if len(norms) >= minMatch {
					fallbackMatched := 0
					fallbackMatchedChars := 0
					for _, norm := range norms {
						if remainingNorms[norm] <= 0 {
							continue
						}
						remainingNorms[norm]--
						fallbackMatched++
						fallbackMatchedChars += utf8.RuneCountInString(norm)
					}
					if fallbackMatched >= minMatch {
						c.LinesFromAgent = fallbackMatched
						c.LinePercent = percentage(c.LinesFromAgent, len(norms))
						c.CopiedFromAgent = true
						extraLines += fallbackMatched
						extraChars += fallbackMatchedChars
					}
				}
			}
		}
		out = append(out, c)
	}
	return out, extraLines, extraChars
}

func runGit(ctx context.Context, repoPath string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repoPath}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
