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
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
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

type recomputeCommitCoverageResponse struct {
	Recomputed int    `json:"recomputed"`
	Branch     string `json:"branch"`
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

	ingested, reachedRoot, err := ingestMoreCommitsForProject(r.Context(), s.DB, repoProject, group, identity, s.loadExtraLocalUserEmails(), branch, req.Count)
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

func (s *Server) handleRecomputeCommitCoverage(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.PathValue("id"))
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

	identity, _ := resolveGitIdentity(r.Context(), repoProject.Path)
	n, err := recomputeCommitCoverageForProject(r.Context(), s.DB, repoProject, group, branch, &identity, s.loadExtraLocalUserEmails())
	if err != nil {
		log.Printf("error recomputing commit coverage for %s: %v", projectID, err)
		writeError(w, http.StatusInternalServerError, "failed to recompute commit coverage")
		return
	}

	writeSuccess(w, http.StatusOK, recomputeCommitCoverageResponse{
		Recomputed: n,
		Branch:     branch,
	})
}

// ingestMoreCommitsForProject fetches `count` more commits older than the oldest
// already-ingested commit and stores them. Returns the number ingested and whether
// we reached the root commit. All users' commits are ingested (no identity filter).
func ingestMoreCommitsForProject(
	ctx context.Context,
	database *sql.DB,
	repoProject *db.Project,
	group projectGroup,
	identity gitIdentity,
	extraEmails []string,
	branch string,
	count int,
) (int, bool, error) {
	// Get ALL commits from git (oldest first, no identity filter).
	allGitCommits, err := listBranchCommits(ctx, repoProject.Path, branch, 0)
	if err != nil {
		return 0, false, fmt.Errorf("list git commits: %w", err)
	}
	if len(allGitCommits) == 0 {
		return 0, true, nil
	}

	// Build existing hash set to find the oldest ingested commit.
	allHashes := make([]string, len(allGitCommits))
	for i, c := range allGitCommits {
		allHashes[i] = c.Hash
	}
	existing, err := db.ExistingCommitHashes(ctx, database, repoProject.ID, allHashes)
	if err != nil {
		return 0, false, fmt.Errorf("existing commit hashes: %w", err)
	}

	// Find oldest ingested commit in git order (oldest first).
	oldestIdx := -1
	for i, c := range allGitCommits {
		if existing[c.Hash] {
			oldestIdx = i
			break
		}
	}

	// Determine which commits to ingest: those older than our oldest ingested commit.
	var toIngest []gitCommit
	if oldestIdx < 0 {
		// No commits ingested yet - take the most recent `count`.
		start := len(allGitCommits) - count
		if start < 0 {
			start = 0
		}
		toIngest = allGitCommits[start:]
	} else if oldestIdx == 0 {
		// Already at root.
		return 0, true, nil
	} else {
		// Take `count` commits before the oldest ingested.
		start := oldestIdx - count
		if start < 0 {
			start = 0
		}
		toIngest = allGitCommits[start:oldestIdx]
	}

	if len(toIngest) == 0 {
		return 0, true, nil
	}

	ingested, err := ingestCommits(ctx, database, repoProject, group, branch, toIngest, &identity, extraEmails)
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
// All users' commits are ingested (no identity filter).
func IngestDefaultCommits(
	ctx context.Context,
	database *sql.DB,
	repoProject *db.Project,
	group projectGroup,
	identity gitIdentity,
	extraEmails []string,
	branch string,
) error {
	commits, err := listBranchCommits(ctx, repoProject.Path, branch, maxCommitsPerProject)
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
	_, err = ingestCommits(ctx, database, repoProject, group, branch, commits[start:], &identity, extraEmails)
	return err
}

// IngestCommitsForWindow ingests commits on the given branch either for the
// full history (includeAll=true) or since the provided cutoff timestamp.
func IngestCommitsForWindow(
	ctx context.Context,
	database *sql.DB,
	repoProject *db.Project,
	group projectGroup,
	branch string,
	since time.Time,
	includeAll bool,
	identity *gitIdentity,
	extraEmails []string,
) (int, error) {
	commits, err := listBranchCommits(ctx, repoProject.Path, branch, 0)
	if err != nil {
		return 0, err
	}
	if len(commits) == 0 {
		return 0, nil
	}

	toIngest := commits
	if !includeAll {
		cutoffMs := since.UnixMilli()
		toIngest = toIngest[:0]
		for _, commit := range commits {
			if commit.TimestampUnix*1000 < cutoffMs {
				continue
			}
			toIngest = append(toIngest, commit)
		}
	}

	return ingestCommits(ctx, database, repoProject, group, branch, toIngest, identity, extraEmails)
}

func ingestCommits(
	ctx context.Context,
	database *sql.DB,
	repoProject *db.Project,
	group projectGroup,
	branch string,
	toIngest []gitCommit,
	identity *gitIdentity,
	extraEmails []string,
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
	perCommitAgent := make(map[string]map[string]*agentStats)

	for _, gc := range toIngest {
		rawDiff, err := runGit(ctx, repoProject.Path, "show", "--pretty=format:", "-M", "-w", "--ignore-blank-lines", gc.Hash)
		if err != nil {
			log.Printf("warning: could not get diff for commit %s: %v", gc.Hash, err)
			continue
		}

		cleanDiff := stripBinaryDiffs(rawDiff)

		// Compute coverage using the unified=0 diff for token matching.
		tokenDiff, err := runGit(ctx, repoProject.Path, "show", "--pretty=format:", "--unified=0", "-M", "-w", "--ignore-blank-lines", gc.Hash)
		if err != nil {
			tokenDiff = ""
		}
		commitTokens := parseUnifiedDiffTokens(tokenDiff, ignorePatterns)
		totalLines, totalChars := tokenTotals(commitTokens)

		matchedLines := 0
		matchedChars := 0
		matchesIdentity := identity == nil || commitMatchesExpandedIdentity(gc.UserEmail, *identity, extraEmails)
		if matchesIdentity && len(commitTokens) > 0 && len(messages) > 0 {
			windowStart := gc.TimestampUnix*1000 - defaultMessageWindowMs
			windowEnd := gc.TimestampUnix*1000 + commitWindowLookaheadMs
			_, ml, mc, fileAgent, remainingNorms := attributeCommitToMessages(commitTokens, messages, windowStart, windowEnd)
			files, fallbackLines, fallbackChars := summarizeDiffFiles(tokenDiff, ignorePatterns, commitTokens, fileAgent, remainingNorms)
			matchedLines = ml + fallbackLines
			matchedChars = mc + fallbackChars

			// Aggregate agent attribution from both exact matches and copied-from-agent files.
			segs := attributeCopiedFromAgentFiles(files, commitTokens, messages, windowStart, windowEnd, totalLines)
			if len(segs) > 0 {
				byAgent := make(map[string]*agentStats)
				for _, seg := range segs {
					byAgent[seg.Agent] = &agentStats{lines: seg.LinesFromAgent}
				}
				perCommitAgent[gc.Hash] = byAgent
			}
		}

		ingestAdded, ingestRemoved := countDiffAddedRemoved(cleanDiff)
		dbCommits = append(dbCommits, db.Commit{
			ProjectID:       repoProject.ID,
			BranchName:      branch,
			CommitHash:      gc.Hash,
			Subject:         gc.Subject,
			UserName:        gc.UserName,
			UserEmail:       gc.UserEmail,
			AuthoredAt:      gc.TimestampUnix,
			DiffContent:     cleanDiff,
			LinesTotal:      totalLines,
			CharsTotal:      totalChars,
			LinesFromAgent:  matchedLines,
			CharsFromAgent:  matchedChars,
			LinesAdded:      ingestAdded,
			LinesRemoved:    ingestRemoved,
			CoverageVersion: currentCommitCoverageVersion,
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
			dbCommit, err := db.GetCommitByProjectAndHash(ctx, database, c.ProjectID, c.CommitHash)
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

// ingestMissingCommits fetches metadata from git for each missing hash and ingests them.
func ingestMissingCommits(
	ctx context.Context,
	database *sql.DB,
	repoProject *db.Project,
	group projectGroup,
	branch string,
	missingHashes []string,
	identity *gitIdentity,
	extraEmails []string,
) (int, error) {
	if len(missingHashes) == 0 {
		return 0, nil
	}
	var commits []gitCommit
	for _, hash := range missingHashes {
		gc, err := getCommitMetadata(ctx, repoProject.Path, hash)
		if err != nil {
			continue
		}
		commits = append(commits, *gc)
	}
	if len(commits) == 0 {
		return 0, nil
	}
	return ingestCommits(ctx, database, repoProject, group, branch, commits, identity, extraEmails)
}

func recomputeCommitCoverageForProject(
	ctx context.Context,
	database *sql.DB,
	repoProject *db.Project,
	group projectGroup,
	branch string,
	identity *gitIdentity,
	extraEmails []string,
) (int, error) {
	return recomputeCommitCoverageForProjectWithChangedPatterns(ctx, database, repoProject, group, branch, nil, identity, extraEmails)
}

// recomputeSingleCommit recomputes coverage for one commit.
func recomputeSingleCommit(
	ctx context.Context,
	repoPath string,
	c db.Commit,
	ignorePatterns []string,
	messages []messageDiff,
	identity *gitIdentity,
	extraEmails []string,
) (db.Commit, map[string]agentStats, error) {
	cleanDiff := c.DiffContent
	rawDiff, rawErr := runGit(ctx, repoPath, "show", "--pretty=format:", "-M", "-w", "--ignore-blank-lines", c.CommitHash)
	if rawErr == nil {
		cleanDiff = stripBinaryDiffs(rawDiff)
	}

	tokenDiff, err := runGit(ctx, repoPath, "show", "--pretty=format:", "--unified=0", "-M", "-w", "--ignore-blank-lines", c.CommitHash)
	if err != nil {
		tokenDiff = cleanDiff
	}
	commitTokens := parseUnifiedDiffTokens(tokenDiff, ignorePatterns)
	totalLines, totalChars := tokenTotals(commitTokens)

	matchesIdent := identity == nil || commitMatchesExpandedIdentity(c.UserEmail, *identity, extraEmails)

	matchedLines := 0
	matchedChars := 0
	var files []commitFileCoverage
	fallbackLines := 0
	fallbackChars := 0

	windowStart := c.AuthoredAt*1000 - defaultMessageWindowMs
	windowEnd := c.AuthoredAt*1000 + commitWindowLookaheadMs
	if matchesIdent {
		var fileAgent map[string]commitFileCoverage
		var remainingNorms map[string]int
		_, matchedLines, matchedChars, fileAgent, remainingNorms = attributeCommitToMessages(commitTokens, messages, windowStart, windowEnd)
		files, fallbackLines, fallbackChars = summarizeDiffFiles(tokenDiff, ignorePatterns, commitTokens, fileAgent, remainingNorms)
		matchedLines += fallbackLines
		matchedChars += fallbackChars
	}

	recompAdded, recompRemoved := countDiffAddedRemoved(cleanDiff)
	updated := db.Commit{
		ID:              c.ID,
		ProjectID:       c.ProjectID,
		BranchName:      c.BranchName,
		CommitHash:      c.CommitHash,
		Subject:         c.Subject,
		UserName:        c.UserName,
		UserEmail:       c.UserEmail,
		AuthoredAt:      c.AuthoredAt,
		DiffContent:     cleanDiff,
		LinesTotal:      totalLines,
		CharsTotal:      totalChars,
		LinesFromAgent:  matchedLines,
		CharsFromAgent:  matchedChars,
		LinesAdded:      recompAdded,
		LinesRemoved:    recompRemoved,
		CoverageVersion: currentCommitCoverageVersion,
	}

	var byAgent map[string]agentStats
	segs := attributeCopiedFromAgentFiles(files, commitTokens, messages, windowStart, windowEnd, totalLines)
	if len(segs) > 0 {
		byAgent = make(map[string]agentStats, len(segs))
		for _, seg := range segs {
			byAgent[seg.Agent] = agentStats{lines: seg.LinesFromAgent}
		}
	}
	return updated, byAgent, nil
}

// persistRecomputedCommits upserts recomputed commits and agent coverage.
func persistRecomputedCommits(
	ctx context.Context,
	database *sql.DB,
	commits []db.Commit,
	originals []db.Commit,
	perCommitAgent map[string]map[string]agentStats,
) error {
	if err := db.UpsertCommits(ctx, database, commits); err != nil {
		return fmt.Errorf("upsert recomputed commits: %w", err)
	}
	for _, c := range originals {
		if err := db.DeleteCommitAgentCoverageByCommitID(ctx, database, c.ID); err != nil {
			return err
		}
		byAgent := perCommitAgent[c.ID]
		if len(byAgent) == 0 {
			continue
		}
		rows := make([]db.CommitAgentCoverage, 0, len(byAgent))
		for agentName, stats := range byAgent {
			rows = append(rows, db.CommitAgentCoverage{
				CommitID:       c.ID,
				Agent:          agentName,
				LinesFromAgent: stats.lines,
				CharsFromAgent: stats.chars,
			})
		}
		if err := db.UpsertCommitAgentCoverage(ctx, database, rows); err != nil {
			return fmt.Errorf("upsert recomputed commit agent coverage: %w", err)
		}
	}
	return nil
}

func recomputeCommitCoverageForProjectHashes(
	ctx context.Context,
	database *sql.DB,
	repoProject *db.Project,
	group projectGroup,
	hashes []string,
	progress func(message string, processed int),
	identity *gitIdentity,
	extraEmails []string,
) (int, error) {
	if len(hashes) == 0 {
		return 0, nil
	}
	commits, err := db.ListCommitsByHashes(ctx, database, repoProject.ID, hashes, len(hashes), 0)
	if err != nil {
		return 0, fmt.Errorf("list commits by hashes: %w", err)
	}
	if len(commits) == 0 {
		return 0, nil
	}

	minTs := commits[0].AuthoredAt * 1000
	maxTs := commits[0].AuthoredAt * 1000
	for _, c := range commits {
		ts := c.AuthoredAt * 1000
		if ts < minTs {
			minTs = ts
		}
		if ts > maxTs {
			maxTs = ts
		}
	}
	messages, err := listDerivedDiffMessages(ctx, database, projectIDs(group), minTs-defaultMessageWindowMs, maxTs+commitWindowLookaheadMs)
	if err != nil {
		return 0, fmt.Errorf("list derived diff messages: %w", err)
	}
	ignorePatterns := groupIgnoreDiffPatterns(group)

	updatedCommits := make([]db.Commit, 0, len(commits))
	perCommitAgent := make(map[string]map[string]agentStats)

	for _, c := range commits {
		if progress != nil {
			progress(fmt.Sprintf("Recomputing commit %s...", c.CommitHash), len(updatedCommits))
		}
		updated, byAgent, err := recomputeSingleCommit(ctx, repoProject.Path, c, ignorePatterns, messages, identity, extraEmails)
		if err != nil {
			continue
		}
		updatedCommits = append(updatedCommits, updated)
		if len(byAgent) > 0 {
			perCommitAgent[c.ID] = byAgent
		}
	}

	if err := persistRecomputedCommits(ctx, database, updatedCommits, commits, perCommitAgent); err != nil {
		return 0, err
	}
	return len(updatedCommits), nil
}

func recomputeCommitCoverageForProjectWithChangedPatterns(
	ctx context.Context,
	database *sql.DB,
	repoProject *db.Project,
	group projectGroup,
	branch string,
	changedPatterns []string,
	identity *gitIdentity,
	extraEmails []string,
) (int, error) {
	// Get branch hashes and query DB by hash list so recompute works across branches.
	branchHashes, err := listBranchCommitHashes(ctx, repoProject.Path, branch)
	if err != nil {
		return 0, fmt.Errorf("list branch hashes: %w", err)
	}
	if len(branchHashes) == 0 {
		return 0, nil
	}
	total, err := db.CountCommitsByHashes(ctx, database, repoProject.ID, branchHashes)
	if err != nil {
		return 0, fmt.Errorf("count commits: %w", err)
	}
	if total == 0 {
		return 0, nil
	}
	commits, err := db.ListCommitsByHashes(ctx, database, repoProject.ID, branchHashes, total, 0)
	if err != nil {
		return 0, fmt.Errorf("list commits: %w", err)
	}
	if len(commits) == 0 {
		return 0, nil
	}
	if len(changedPatterns) > 0 {
		filtered := make([]db.Commit, 0, len(commits))
		for _, c := range commits {
			existing, err := db.GetCommitByProjectAndHash(ctx, database, c.ProjectID, c.CommitHash)
			if err != nil || existing == nil {
				continue
			}
			if commitDiffTouchesChangedPatterns(existing.DiffContent, changedPatterns) {
				filtered = append(filtered, c)
			}
		}
		commits = filtered
		if len(commits) == 0 {
			return 0, nil
		}
	}

	minTs := commits[0].AuthoredAt * 1000
	maxTs := commits[0].AuthoredAt * 1000
	for _, c := range commits {
		ts := c.AuthoredAt * 1000
		if ts < minTs {
			minTs = ts
		}
		if ts > maxTs {
			maxTs = ts
		}
	}
	messages, err := listDerivedDiffMessages(ctx, database, projectIDs(group), minTs-defaultMessageWindowMs, maxTs+commitWindowLookaheadMs)
	if err != nil {
		return 0, fmt.Errorf("list derived diff messages: %w", err)
	}
	ignorePatterns := groupIgnoreDiffPatterns(group)

	updatedCommits := make([]db.Commit, 0, len(commits))
	perCommitAgent := make(map[string]map[string]agentStats)

	for _, c := range commits {
		updated, byAgent, err := recomputeSingleCommit(ctx, repoProject.Path, c, ignorePatterns, messages, identity, extraEmails)
		if err != nil {
			continue
		}
		updatedCommits = append(updatedCommits, updated)
		if len(byAgent) > 0 {
			perCommitAgent[c.ID] = byAgent
		}
	}

	if err := persistRecomputedCommits(ctx, database, updatedCommits, commits, perCommitAgent); err != nil {
		return 0, err
	}
	return len(updatedCommits), nil
}

func commitDiffTouchesChangedPatterns(diffText string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}

	diffText = strings.ReplaceAll(diffText, "\r\n", "\n")
	lines := strings.Split(diffText, "\n")

	checkPath := func(p string) bool {
		p = strings.TrimSpace(p)
		if p == "" {
			return false
		}
		return shouldIgnoreDiffPath(p, patterns)
	}

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			parts := strings.Fields(line)
			if len(parts) >= 4 {
				if checkPath(parseDiffPath(parts[2])) || checkPath(parseDiffPath(parts[3])) {
					return true
				}
			}
		case strings.HasPrefix(line, "rename from "):
			if checkPath(parseDiffPath(strings.TrimPrefix(line, "rename from "))) {
				return true
			}
		case strings.HasPrefix(line, "rename to "):
			if checkPath(parseDiffPath(strings.TrimPrefix(line, "rename to "))) {
				return true
			}
		case strings.HasPrefix(line, "--- "):
			if checkPath(parseDiffPath(strings.TrimPrefix(line, "--- "))) {
				return true
			}
		case strings.HasPrefix(line, "+++ "):
			if checkPath(parseDiffPath(strings.TrimPrefix(line, "+++ "))) {
				return true
			}
		}
	}
	return false
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
			UserName:      strings.TrimSpace(parts[1]),
			UserEmail:     strings.TrimSpace(parts[2]),
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
	IngestedCount         int    `json:"ingestedCount"`
	TotalGitCommits       int    `json:"totalGitCommits"`
	EstimatedTotalCommits int    `json:"estimatedTotalCommits"`
	ReachedRoot           bool   `json:"reachedRoot"`
	State                 string `json:"state"`
	LastStartedAt         int64  `json:"lastStartedAt"`
	LastFinishedAt        int64  `json:"lastFinishedAt"`
	LastDurationMs        int64  `json:"lastDurationMs"`
	LastError             string `json:"lastError"`
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

	// Use git hash list as source of truth for branch membership.
	branchHashes, hashErr := listBranchCommitHashes(r.Context(), project.Path, branch)
	if hashErr != nil {
		log.Printf("error listing branch hashes for %s: %v", projectID, hashErr)
	}

	ingestedCount := 0
	if len(branchHashes) > 0 {
		ingestedCount, err = db.CountCommitsByHashes(r.Context(), s.DB, project.ID, branchHashes)
		if err != nil {
			log.Printf("error counting commits for %s: %v", projectID, err)
			writeError(w, http.StatusInternalServerError, "failed to count commits")
			return
		}
	}

	totalGit := len(branchHashes)

	syncState, err := db.GetCommitSyncState(r.Context(), s.DB, project.ID, branch)
	if err != nil {
		log.Printf("error loading commit sync state for %s: %v", projectID, err)
	}

	state := "idle"
	var lastStarted, lastFinished, lastDuration int64
	lastError := ""
	if syncState != nil {
		state = strings.TrimSpace(syncState.State)
		if state == "" {
			state = "idle"
		}
		lastStarted = syncState.LastStartedAtMs
		lastFinished = syncState.LastFinishedAtMs
		lastDuration = syncState.LastDurationMs
		lastError = syncState.LastError
	}

	reachedRoot := false
	if totalGit > 0 {
		reachedRoot = ingestedCount >= totalGit
	} else if ingestedCount == 0 {
		reachedRoot = true
	}

	writeSuccess(w, http.StatusOK, commitIngestionStatusResponse{
		IngestedCount:         ingestedCount,
		TotalGitCommits:       totalGit,
		EstimatedTotalCommits: totalGit,
		ReachedRoot:           reachedRoot,
		State:                 state,
		LastStartedAt:         lastStarted,
		LastFinishedAt:        lastFinished,
		LastDurationMs:        lastDuration,
		LastError:             lastError,
	})
}

func parseTimestampStr(s string) (int64, error) {
	var ts int64
	_, err := fmt.Sscanf(s, "%d", &ts)
	return ts, err
}
