package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"

	"github.com/davidcann/zrate/web/server/internal/db"
)

const defaultIngestCount = 20

// binaryFilePattern matches git's "Binary files ... differ" lines and
// the GIT binary patch header.
var binaryFilePattern = regexp.MustCompile(`(?m)^(Binary files .+ differ|GIT binary patch)`)

type ingestCommitsRequest struct {
	Count int `json:"count"`
}

type ingestCommitsResponse struct {
	Ingested    int    `json:"ingested"`
	ReachedRoot bool   `json:"reachedRoot"`
	Branch      string `json:"branch"`
}

func (s *Server) handleIngestMoreCommits(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.PathValue("id"))
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project id is required")
		return
	}

	if !requireJSON(w, r) {
		return
	}

	var req ingestCommitsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Count <= 0 {
		req.Count = defaultIngestCount
	}
	if req.Count > 500 {
		req.Count = 500
	}

	branch := strings.TrimSpace(r.URL.Query().Get("branch"))
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

	identity, err := resolveGitIdentity(r.Context(), repoProject.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, "git identity not configured for project")
		return
	}

	ingested, reachedRoot, err := ingestMoreCommitsForProject(r.Context(), s.DB, repoProject, group, identity, branch, req.Count)
	if err != nil {
		log.Printf("error ingesting commits for %s: %v", projectID, err)
		writeError(w, http.StatusInternalServerError, "failed to ingest commits")
		return
	}

	writeSuccess(w, http.StatusOK, ingestCommitsResponse{
		Ingested:    ingested,
		ReachedRoot: reachedRoot,
		Branch:      branch,
	})
}

// ingestMoreCommitsForProject fetches `count` more commits older than the oldest
// already-ingested commit and stores them. Returns the number ingested and whether
// we reached the root commit.
func ingestMoreCommitsForProject(
	ctx context.Context,
	database *sql.DB,
	repoProject *db.Project,
	group projectGroup,
	identity gitIdentity,
	branch string,
	count int,
) (int, bool, error) {
	// Get ALL commits from git for this user (oldest first).
	allGitCommits, err := listAllCommitsByIdentity(ctx, repoProject.Path, branch, identity)
	if err != nil {
		return 0, false, fmt.Errorf("list git commits: %w", err)
	}
	if len(allGitCommits) == 0 {
		return 0, true, nil
	}

	// Find the oldest already-ingested commit.
	oldest, err := db.OldestCommitByProject(ctx, database, repoProject.ID, branch)
	if err != nil {
		return 0, false, fmt.Errorf("oldest commit: %w", err)
	}

	// Determine which commits to ingest: those older than our oldest ingested commit.
	var toIngest []gitCommit
	if oldest == nil {
		// No commits ingested yet - take the most recent `count`.
		start := len(allGitCommits) - count
		if start < 0 {
			start = 0
		}
		toIngest = allGitCommits[start:]
	} else {
		// Find the oldest ingested commit in the git log and take `count` commits before it.
		oldestIdx := -1
		for i, c := range allGitCommits {
			if c.Hash == oldest.CommitHash {
				oldestIdx = i
				break
			}
		}
		if oldestIdx <= 0 {
			// Already at root or commit not found in log.
			return 0, true, nil
		}
		start := oldestIdx - count
		if start < 0 {
			start = 0
		}
		toIngest = allGitCommits[start:oldestIdx]
	}

	if len(toIngest) == 0 {
		return 0, true, nil
	}

	ingested, err := ingestCommits(ctx, database, repoProject, group, branch, toIngest)
	if err != nil {
		return 0, false, err
	}

	// Check if we reached the root (first commit in git log).
	reachedRoot := false
	if len(toIngest) > 0 && len(allGitCommits) > 0 {
		reachedRoot = toIngest[0].Hash == allGitCommits[0].Hash
	}

	return ingested, reachedRoot, nil
}

// IngestDefaultCommits ingests the latest `defaultIngestCount` commits for a project
// on each call so commits endpoints always include newly created commits.
func IngestDefaultCommits(
	ctx context.Context,
	database *sql.DB,
	repoProject *db.Project,
	group projectGroup,
	identity gitIdentity,
	branch string,
) error {
	commits, err := listCommitsByIdentity(ctx, repoProject.Path, branch, identity)
	if err != nil {
		return err
	}
	if len(commits) == 0 {
		return nil
	}

	start := len(commits) - defaultIngestCount
	if start < 0 {
		start = 0
	}
	_, err = ingestCommits(ctx, database, repoProject, group, branch, commits[start:])
	return err
}

func ingestCommits(
	ctx context.Context,
	database *sql.DB,
	repoProject *db.Project,
	group projectGroup,
	branch string,
	toIngest []gitCommit,
) (int, error) {
	if len(toIngest) == 0 {
		return 0, nil
	}

	ignorePatterns := groupIgnoreDiffPatterns(group)
	pIDs := projectIDs(group)

	firstTs := toIngest[0].TimestampUnix*1000 - defaultMessageWindowMs
	lastTs := toIngest[len(toIngest)-1].TimestampUnix*1000 + commitWindowLookaheadMs
	messages, err := listDerivedDiffMessages(ctx, database, pIDs, firstTs, lastTs)
	if err != nil {
		return 0, fmt.Errorf("list derived diff messages: %w", err)
	}

	dbCommits := make([]db.Commit, 0, len(toIngest))
	// Per-commit, per-agent coverage: map from commit hash to agent->lines/chars.
	type agentStats struct {
		lines int
		chars int
	}
	perCommitAgent := make(map[string]map[string]*agentStats)

	for _, gc := range toIngest {
		rawDiff, err := runGit(ctx, repoProject.Path, "show", "--pretty=format:", "-M", "-w", "--ignore-blank-lines", gc.Hash)
		if err != nil {
			log.Printf("warning: could not get diff for commit %s: %v", gc.Hash, err)
			continue
		}

		cleanDiff := stripBinaryDiffs(rawDiff)

		// Compute coverage using the unified=0 diff for token matching.
		tokenDiff, err := runGit(ctx, repoProject.Path, "show", "--pretty=format:", "--unified=0", "-w", "--ignore-blank-lines", gc.Hash)
		if err != nil {
			tokenDiff = ""
		}
		commitTokens := parseUnifiedDiffTokens(tokenDiff, ignorePatterns)
		totalLines, totalChars := tokenTotals(commitTokens)

		matchedLines := 0
		matchedChars := 0
		if len(commitTokens) > 0 && len(messages) > 0 {
			windowStart := gc.TimestampUnix*1000 - defaultMessageWindowMs
			windowEnd := gc.TimestampUnix*1000 + commitWindowLookaheadMs
			contribs, ml, mc, _, _ := attributeCommitToMessages(commitTokens, messages, windowStart, windowEnd)
			matchedLines = ml
			matchedChars = mc

			// Aggregate contribution messages by agent.
			if len(contribs) > 0 {
				byAgent := make(map[string]*agentStats)
				for _, cm := range contribs {
					agent := cm.Agent
					if agent == "" {
						agent = "unknown"
					}
					s := byAgent[agent]
					if s == nil {
						s = &agentStats{}
						byAgent[agent] = s
					}
					s.lines += cm.LinesMatched
					s.chars += cm.CharsMatched
				}
				perCommitAgent[gc.Hash] = byAgent
			}
		}

		dbCommits = append(dbCommits, db.Commit{
			ProjectID:      repoProject.ID,
			BranchName:     branch,
			CommitHash:     gc.Hash,
			Subject:        gc.Subject,
			AuthorName:     gc.AuthorName,
			AuthorEmail:    gc.AuthorEmail,
			AuthoredAt:     gc.TimestampUnix,
			DiffContent:    cleanDiff,
			LinesTotal:     totalLines,
			CharsTotal:     totalChars,
			LinesFromAgent: matchedLines,
			CharsFromAgent: matchedChars,
		})
	}

	if err := db.UpsertCommits(ctx, database, dbCommits); err != nil {
		return 0, fmt.Errorf("upsert commits: %w", err)
	}

	// Store per-agent coverage. We need the commit IDs from the DB.
	if len(perCommitAgent) > 0 {
		var agentCoverageRows []db.CommitAgentCoverage
		for _, c := range dbCommits {
			byAgent, ok := perCommitAgent[c.CommitHash]
			if !ok {
				continue
			}
			// We need the DB commit ID. Since UpsertCommits may have set it on conflict,
			// look it up by hash.
			dbCommit, err := db.GetCommitByHash(ctx, database, c.ProjectID, branch, c.CommitHash)
			if err != nil || dbCommit == nil {
				continue
			}
			for agent, stats := range byAgent {
				agentCoverageRows = append(agentCoverageRows, db.CommitAgentCoverage{
					CommitID:       dbCommit.ID,
					Agent:          agent,
					LinesFromAgent: stats.lines,
					CharsFromAgent: stats.chars,
				})
			}
		}
		if err := db.UpsertCommitAgentCoverage(ctx, database, agentCoverageRows); err != nil {
			log.Printf("warning: failed to upsert agent coverage: %v", err)
		}
	}

	return len(dbCommits), nil
}

// listAllCommitsByIdentity is like listCommitsByIdentity but without the max-count limit,
// so we can see all commits for pagination/ingestion purposes.
func listAllCommitsByIdentity(ctx context.Context, path, branch string, identity gitIdentity) ([]gitCommit, error) {
	out, err := runGit(ctx, path,
		"log", branch,
		"--pretty=format:%H%x1f%an%x1f%ae%x1f%ct%x1f%s%x1e",
		"--reverse",
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
		ts, err := parseTimestampStr(strings.TrimSpace(parts[3]))
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

// stripBinaryDiffs removes binary file diff blocks from a unified diff string.
// It preserves the text diffs for non-binary files.
func stripBinaryDiffs(diff string) string {
	diff = strings.ReplaceAll(diff, "\r\n", "\n")
	lines := strings.Split(diff, "\n")

	var result []string
	inBinaryBlock := false
	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			inBinaryBlock = false
		}
		if binaryFilePattern.MatchString(line) {
			inBinaryBlock = true
			// Walk back to remove the diff --git header for this binary block.
			for len(result) > 0 {
				last := result[len(result)-1]
				result = result[:len(result)-1]
				if strings.HasPrefix(last, "diff --git ") {
					break
				}
			}
			continue
		}
		if inBinaryBlock {
			continue
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}

type commitIngestionStatusResponse struct {
	IngestedCount   int  `json:"ingestedCount"`
	TotalGitCommits int  `json:"totalGitCommits"`
	ReachedRoot     bool `json:"reachedRoot"`
}

func (s *Server) handleCommitIngestionStatus(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.PathValue("projectId"))
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project id is required")
		return
	}

	branch := strings.TrimSpace(r.URL.Query().Get("branch"))
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

	ingestedCount, err := db.CountCommitsByProject(r.Context(), s.DB, project.ID, branch)
	if err != nil {
		log.Printf("error counting commits for %s: %v", projectID, err)
		writeError(w, http.StatusInternalServerError, "failed to count commits")
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
		writeSuccess(w, http.StatusOK, commitIngestionStatusResponse{
			IngestedCount:   ingestedCount,
			TotalGitCommits: 0,
			ReachedRoot:     true,
		})
		return
	}

	repoProject, err := resolveRepoProject(r.Context(), group)
	if err != nil {
		writeSuccess(w, http.StatusOK, commitIngestionStatusResponse{
			IngestedCount:   ingestedCount,
			TotalGitCommits: 0,
			ReachedRoot:     true,
		})
		return
	}

	identity, err := resolveGitIdentity(r.Context(), repoProject.Path)
	if err != nil {
		writeSuccess(w, http.StatusOK, commitIngestionStatusResponse{
			IngestedCount:   ingestedCount,
			TotalGitCommits: 0,
			ReachedRoot:     true,
		})
		return
	}

	allGitCommits, err := listAllCommitsByIdentity(r.Context(), repoProject.Path, branch, identity)
	if err != nil {
		log.Printf("error listing all git commits for %s: %v", projectID, err)
		writeSuccess(w, http.StatusOK, commitIngestionStatusResponse{
			IngestedCount:   ingestedCount,
			TotalGitCommits: 0,
			ReachedRoot:     true,
		})
		return
	}

	reachedRoot := ingestedCount >= len(allGitCommits)
	if !reachedRoot && ingestedCount > 0 {
		oldest, err := db.OldestCommitByProject(r.Context(), s.DB, project.ID, branch)
		if err == nil && oldest != nil && len(allGitCommits) > 0 {
			reachedRoot = oldest.CommitHash == allGitCommits[0].Hash
		}
	}

	writeSuccess(w, http.StatusOK, commitIngestionStatusResponse{
		IngestedCount:   ingestedCount,
		TotalGitCommits: len(allGitCommits),
		ReachedRoot:     reachedRoot,
	})
}

func parseTimestampStr(s string) (int64, error) {
	var ts int64
	_, err := fmt.Sscanf(s, "%d", &ts)
	return ts, err
}
