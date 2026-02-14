package handler

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
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
	mainBranch              = "main"
	workingCopyCommitHash   = "working-copy"
	defaultMessageWindowMs  = int64(24 * 60 * 60 * 1000)
	commitWindowLookaheadMs = int64(5 * 60 * 1000)
	maxCommitsPerProject    = 200
	commitsPageSize         = 20
)

type projectCommitsResponse struct {
	Branch       string                  `json:"branch"`
	CurrentUser  string                  `json:"currentUser"`
	CurrentEmail string                  `json:"currentEmail"`
	Summary      projectCommitSummary    `json:"summary"`
	Commits      []projectCommitCoverage `json:"commits"`
}

type projectCommitSummary struct {
	CommitCount      int     `json:"commitCount"`
	LinesTotal       int     `json:"linesTotal"`
	LinesFromAgent   int     `json:"linesFromAgent"`
	LinePercent      float64 `json:"linePercent"`
	CharsTotal       int     `json:"charsTotal"`
	CharsFromAgent   int     `json:"charsFromAgent"`
	CharacterPercent float64 `json:"characterPercent"`
}

type projectCommitCoverage struct {
	WorkingCopy      bool    `json:"workingCopy"`
	ProjectID        string  `json:"projectId"`
	ProjectLabel     string  `json:"projectLabel"`
	ProjectPath      string  `json:"projectPath"`
	ProjectGitID     string  `json:"projectGitId"`
	CommitHash       string  `json:"commitHash"`
	Subject          string  `json:"subject"`
	AuthoredAtUnixMs int64   `json:"authoredAtUnixMs"`
	LinesTotal       int     `json:"linesTotal"`
	LinesFromAgent   int     `json:"linesFromAgent"`
	LinePercent      float64 `json:"linePercent"`
	CharsTotal       int     `json:"charsTotal"`
	CharsFromAgent   int     `json:"charsFromAgent"`
	CharacterPercent float64 `json:"characterPercent"`
}

type projectCommitDetailResponse struct {
	Branch   string                      `json:"branch"`
	Commit   projectCommitCoverage       `json:"commit"`
	Diff     string                      `json:"diff"`
	Files    []commitFileCoverage        `json:"files"`
	Messages []commitContributionMessage `json:"messages"`
}

type projectCommitPageResponse struct {
	Branch       string                  `json:"branch"`
	CurrentUser  string                  `json:"currentUser"`
	CurrentEmail string                  `json:"currentEmail"`
	Project      db.Project              `json:"project"`
	Summary      projectCommitSummary    `json:"summary"`
	Pagination   projectCommitPagination `json:"pagination"`
	Commits      []projectCommitCoverage `json:"commits"`
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
	Model             string `json:"model"`
	Content           string `json:"content"`
	LinesMatched      int    `json:"linesMatched"`
	CharsMatched      int    `json:"charsMatched"`
}

type commitFileCoverage struct {
	Path            string  `json:"path"`
	Added           int     `json:"added"`
	Removed         int     `json:"removed"`
	Ignored         bool    `json:"ignored"`
	Moved           bool    `json:"moved"`
	MovedFrom       string  `json:"movedFrom"`
	CopiedFromAgent bool    `json:"copiedFromAgent"`
	LinesTotal      int     `json:"linesTotal"`
	LinesFromAgent  int     `json:"linesFromAgent"`
	LinePercent     float64 `json:"linePercent"`
}

type gitIdentity struct {
	Name  string
	Email string
}

type gitCommit struct {
	Hash          string
	Subject       string
	AuthorName    string
	AuthorEmail   string
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
	Model             string
	Content           string
	Tokens            []diffToken
}

type diffToken struct {
	Path  string
	Norm  string
	Key   string
	Chars int
}

func (s *Server) handleListProjectCommits(w http.ResponseWriter, r *http.Request) {
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

		identity, err := resolveGitIdentity(r.Context(), repoProject.Path)
		if err != nil {
			continue
		}
		if currentUser == "" {
			currentUser = identity.Name
			currentEmail = identity.Email
		}

		// Trigger default ingestion if needed.
		if err := IngestDefaultCommits(r.Context(), s.DB, repoProject, group, identity); err != nil {
			log.Printf("warning: default commit ingestion failed for %s: %v", repoProject.Path, err)
		}

		// Read commits from database.
		dbCommits, err := db.ListCommitsByProjectIDs(r.Context(), s.DB, projectIDs(group))
		if err != nil {
			log.Printf("error listing db commits for %s: %v", repoProject.Path, err)
			continue
		}
		for _, c := range dbCommits {
			rp := projectMap[c.ProjectID]
			if rp == nil {
				rp = repoProject
			}
			all = append(all, dbCommitToCoverage(c, rp))
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
		Branch:       mainBranch,
		CurrentUser:  currentUser,
		CurrentEmail: currentEmail,
		Summary:      summary,
		Commits:      all,
	})
}

func (s *Server) handleGetProjectCommit(w http.ResponseWriter, r *http.Request) {
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

	identity, err := resolveGitIdentity(r.Context(), repoProject.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, "git identity not configured for project")
		return
	}

	commits, err := listCommitsByIdentity(r.Context(), repoProject.Path, identity)
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
			Branch:   mainBranch,
			Commit:   coverage,
			Diff:     diffText,
			Files:    files,
			Messages: messages,
		})
		return
	}

	// Try to load from database first.
	dbCommit, err := db.GetCommitByHash(r.Context(), s.DB, repoProject.ID, commitHash)
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
			AuthorName:    dbCommit.AuthorName,
			AuthorEmail:   dbCommit.AuthorEmail,
			TimestampUnix: dbCommit.AuthoredAt,
		}
		commitDiff = dbCommit.DiffContent
		// Use stored diff tokens when commit is already ingested.
		tokenDiff = commitDiff
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
	commitIdx := -1
	for i := range commits {
		if commits[i].Hash == commitHash {
			commitIdx = i
			break
		}
	}
	windowStart := commit.TimestampUnix*1000 - defaultMessageWindowMs
	if commitIdx > 0 {
		prev := commits[commitIdx-1].TimestampUnix * 1000
		if prev > windowStart {
			windowStart = prev
		}
	}
	windowEnd := commit.TimestampUnix*1000 + commitWindowLookaheadMs

	messages, err := listDerivedDiffMessages(r.Context(), s.DB, projectIDs(group), windowStart, windowEnd)
	if err != nil {
		log.Printf("error listing derived diff messages: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to load matching messages")
		return
	}

	contribMessages, matchedLines, matchedChars, fileAgent, remainingNorms := attributeCommitToMessages(commitTokens, messages, windowStart, windowEnd)
	totalLines, totalChars := tokenTotals(commitTokens)
	files := summarizeDiffFiles(commitDiff, ignorePatterns, commitTokens, fileAgent, remainingNorms)

	writeSuccess(w, http.StatusOK, projectCommitDetailResponse{
		Branch: mainBranch,
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
		},
		Diff:     commitDiff,
		Files:    files,
		Messages: contribMessages,
	})
}

func (s *Server) handleListProjectCommitsForProject(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.PathValue("projectId"))
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project id is required")
		return
	}

	page := parsePositiveInt(r.URL.Query().Get("page"), 1)

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
	identity, err := resolveGitIdentity(r.Context(), repoProject.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, "git identity not configured for project")
		return
	}

	// Trigger default ingestion if needed (first page load).
	if err := IngestDefaultCommits(r.Context(), s.DB, repoProject, group, identity); err != nil {
		log.Printf("warning: default commit ingestion failed for %s: %v", repoProject.Path, err)
	}

	// Read commits from database.
	total, err := db.CountCommitsByProject(r.Context(), s.DB, repoProject.ID)
	if err != nil {
		log.Printf("error counting commits for %s: %v", repoProject.Path, err)
		writeError(w, http.StatusInternalServerError, "failed to count commits")
		return
	}

	totalPages := 0
	if total > 0 {
		totalPages = (total + commitsPageSize - 1) / commitsPageSize
	}
	if totalPages > 0 && page > totalPages {
		page = totalPages
	}
	offset := (page - 1) * commitsPageSize
	if offset < 0 {
		offset = 0
	}

	dbCommits, err := db.ListCommitsByProject(r.Context(), s.DB, repoProject.ID, commitsPageSize, offset)
	if err != nil {
		log.Printf("error listing commits from db for %s: %v", repoProject.Path, err)
		writeError(w, http.StatusInternalServerError, "failed to list commits")
		return
	}

	// Convert DB commits to coverage structs.
	paged := make([]projectCommitCoverage, 0, len(dbCommits))
	for _, c := range dbCommits {
		paged = append(paged, dbCommitToCoverage(c, repoProject))
	}

	// Compute summary from all DB commits.
	allDBCommits, err := db.ListCommitsByProject(r.Context(), s.DB, repoProject.ID, total, 0)
	if err != nil {
		log.Printf("error listing all commits from db for %s: %v", repoProject.Path, err)
		writeError(w, http.StatusInternalServerError, "failed to list commits")
		return
	}
	allCoverage := make([]projectCommitCoverage, 0, len(allDBCommits))
	for _, c := range allDBCommits {
		allCoverage = append(allCoverage, dbCommitToCoverage(c, repoProject))
	}

	// Add working copy on page 1.
	if page == 1 {
		ignorePatterns := groupIgnoreDiffPatterns(group)
		gitCommits, _ := listCommitsByIdentity(r.Context(), repoProject.Path, identity)
		workingCopy, ok := computeWorkingCopyCoverage(r.Context(), s.DB, repoProject, projectIDs(group), ignorePatterns, gitCommits)
		if ok {
			paged = append([]projectCommitCoverage{workingCopy}, paged...)
		}
	}

	writeSuccess(w, http.StatusOK, projectCommitPageResponse{
		Branch:       mainBranch,
		CurrentUser:  identity.Name,
		CurrentEmail: identity.Email,
		Project:      *project,
		Summary:      summarizeCommitCoverage(allCoverage),
		Pagination: projectCommitPagination{
			Page:       page,
			PageSize:   commitsPageSize,
			Total:      total,
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
	err := database.QueryRowContext(ctx, "SELECT id, path, label, git_id, ignored, ignore_diff_paths FROM projects WHERE id = ?", projectID).Scan(
		&p.ID,
		&p.Path,
		&p.Label,
		&p.GitID,
		&p.Ignored,
		&p.IgnoreDiffPaths,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query project: %w", err)
	}
	return &p, nil
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

func listCommitsByIdentity(ctx context.Context, path string, identity gitIdentity) ([]gitCommit, error) {
	out, err := runGit(ctx, path,
		"log", mainBranch,
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
			AuthorName:    strings.TrimSpace(parts[1]),
			AuthorEmail:   strings.TrimSpace(parts[2]),
			TimestampUnix: ts,
			Subject:       strings.TrimSpace(parts[4]),
		}
		if commitMatchesIdentity(c, identity) {
			commits = append(commits, c)
		}
	}
	return commits, nil
}

func commitMatchesIdentity(c gitCommit, identity gitIdentity) bool {
	if identity.Email != "" {
		return strings.EqualFold(strings.TrimSpace(c.AuthorEmail), identity.Email)
	}
	if identity.Name != "" {
		return strings.TrimSpace(c.AuthorName) == identity.Name
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
	for i, c := range commits {
		commitDiff, err := runGit(ctx, repoProject.Path, "show", "--pretty=format:", "--unified=0", "-w", "--ignore-blank-lines", c.Hash)
		if err != nil {
			continue
		}
		commitTokens := parseUnifiedDiffTokens(commitDiff, ignorePatterns)
		if len(commitTokens) == 0 {
			continue
		}

		windowStart := c.TimestampUnix*1000 - defaultMessageWindowMs
		if i > 0 {
			prev := commits[i-1].TimestampUnix * 1000
			if prev > windowStart {
				windowStart = prev
			}
		}
		windowEnd := c.TimestampUnix*1000 + commitWindowLookaheadMs

		totalLines, totalChars := tokenTotals(commitTokens)
		_, matchedLines, matchedChars, _, _ := attributeCommitToMessages(commitTokens, messages, windowStart, windowEnd)

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
		`SELECT m.id, m.timestamp, m.conversation_id, c.title, m.model, m.content
		 FROM messages m
		 JOIN conversations c ON c.id = m.conversation_id
		 WHERE m.role = 'agent'
		   AND m.timestamp BETWEEN ? AND ?
		   AND instr(m.raw_json, 'derived_diff') > 0
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
		if err := rows.Scan(&m.ID, &m.Timestamp, &m.ConversationID, &m.ConversationTitle, &m.Model, &m.Content); err != nil {
			return nil, fmt.Errorf("scan derived diff message: %w", err)
		}

		diff, ok := agent.ExtractReliableDiff(m.Content)
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

func tokenTotals(tokens []diffToken) (int, int) {
	lines := 0
	chars := 0
	for _, tok := range tokens {
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
	matchedLines := 0
	matchedChars := 0
	tokenSources := make(map[string][]int)
	normSources := make(map[string]int)
	for i, msg := range messages {
		if msg.Timestamp <= windowStart || msg.Timestamp > windowEnd {
			continue
		}
		for _, tok := range msg.Tokens {
			tokenSources[tok.Key] = append(tokenSources[tok.Key], i)
			if tok.Norm != "" {
				normSources[tok.Norm]++
			}
		}
	}

	contribByIndex := make(map[int]*commitContributionMessage)
	fileCoverageByPath := make(map[string]commitFileCoverage)
	for _, tok := range commitTokens {
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
		msgIdx := sources[0]
		tokenSources[tok.Key] = sources[1:]

		matchedLines++
		matchedChars += tok.Chars
		fileCov.Removed++
		fileCoverageByPath[path] = fileCov
		if tok.Norm != "" && normSources[tok.Norm] > 0 {
			normSources[tok.Norm]--
		}

		contrib := contribByIndex[msgIdx]
		if contrib == nil {
			msg := messages[msgIdx]
			contrib = &commitContributionMessage{
				ID:                msg.ID,
				Timestamp:         msg.Timestamp,
				ConversationID:    msg.ConversationID,
				ConversationTitle: msg.ConversationTitle,
				Model:             msg.Model,
				Content:           msg.Content,
			}
			contribByIndex[msgIdx] = contrib
		}
		contrib.LinesMatched++
		contrib.CharsMatched += tok.Chars
	}

	indices := make([]int, 0, len(contribByIndex))
	for idx := range contribByIndex {
		indices = append(indices, idx)
	}
	sort.Ints(indices)

	out := make([]commitContributionMessage, 0, len(indices))
	for _, idx := range indices {
		out = append(out, *contribByIndex[idx])
	}

	return out, matchedLines, matchedChars, fileCoverageByPath, normSources
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
			if tok, ok := makeDiffToken(newPath, line[1:]); ok {
				tokens = append(tokens, tok)
			}
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			if shouldIgnoreDiffPath(oldPath, ignorePatterns) {
				continue
			}
			if tok, ok := makeDiffToken(oldPath, line[1:]); ok {
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

func groupIgnoreDiffPatterns(group projectGroup) []string {
	patternSet := make(map[string]struct{})
	patterns := make([]string, 0, 8)
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

func makeDiffToken(path, line string) (diffToken, bool) {
	norm := normalizeWhitespace(line)
	if norm == "" {
		return diffToken{}, false
	}
	return diffToken{
		Path:  path,
		Norm:  norm,
		Key:   path + "\x1f" + norm,
		Chars: utf8.RuneCountInString(norm),
	}, true
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
	for _, c := range commits {
		s.LinesTotal += c.LinesTotal
		s.LinesFromAgent += c.LinesFromAgent
		s.CharsTotal += c.CharsTotal
		s.CharsFromAgent += c.CharsFromAgent
	}
	s.LinePercent = percentage(s.LinesFromAgent, s.LinesTotal)
	s.CharacterPercent = percentage(s.CharsFromAgent, s.CharsTotal)
	return s
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

func computeWorkingCopyCoverage(
	ctx context.Context,
	database *sql.DB,
	repoProject *db.Project,
	projectIDs []string,
	ignorePatterns []string,
	commits []gitCommit,
) (projectCommitCoverage, bool) {
	coverage, _, _, _, ok := computeWorkingCopyDetail(ctx, database, repoProject, projectIDs, ignorePatterns, commits)
	return coverage, ok
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
	files := summarizeDiffFiles(diffText, ignorePatterns, commitTokens, fileAgent, remainingNorms)

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
	}, contribMessages, diffText, files, true
}

func summarizeDiffFiles(
	diffText string,
	ignorePatterns []string,
	commitTokens []diffToken,
	fileAgent map[string]commitFileCoverage,
	remainingNorms map[string]int,
) []commitFileCoverage {
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
		if path == "" || tok.Norm == "" {
			continue
		}
		fileNorms[path] = append(fileNorms[path], tok.Norm)
	}

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
			}
			// Fallback: detect relocated/copied agent code by matching normalized
			// lines independent of file path. Require at least 10 lines to reduce
			// small accidental matches.
			if !c.Moved && c.LinesFromAgent == 0 && c.LinesTotal >= 10 {
				norms := fileNorms[filePath]
				if len(norms) >= 10 {
					fallbackMatched := 0
					for _, norm := range norms {
						if remainingNorms[norm] <= 0 {
							continue
						}
						remainingNorms[norm]--
						fallbackMatched++
					}
					if fallbackMatched >= 10 {
						c.LinesFromAgent = fallbackMatched
						c.LinePercent = percentage(c.LinesFromAgent, c.LinesTotal)
						c.CopiedFromAgent = true
					}
				}
			}
		}
		out = append(out, c)
	}
	return out
}

func runGit(ctx context.Context, repoPath string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repoPath}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
