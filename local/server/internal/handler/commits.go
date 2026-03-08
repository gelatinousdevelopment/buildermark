package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

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
		if err := IngestDefaultCommits(r.Context(), s.DB, repoProject, group, identity, s.loadExtraLocalUserEmails(), branch); err != nil {
			log.Printf("warning: default commit ingestion failed for %s: %v", repoProject.Path, err)
		}

		// Get branch hashes and query DB by hash list.
		branchHashes, hashErr := listBranchCommitHashes(r.Context(), repoProject.Path, branch)
		if hashErr != nil {
			log.Printf("error listing branch hashes for %s: %v", repoProject.Path, hashErr)
			continue
		}

		// Get all project commits from DB and intersect with branch hashes.
		allProjectCommits, listErr := db.ListAllCommitsByProject(r.Context(), s.DB, repoProject.ID, 0, 0)
		if listErr != nil {
			log.Printf("error listing project commits for %s: %v", repoProject.Path, listErr)
			continue
		}
		hashSet := make(map[string]bool, len(branchHashes))
		for _, h := range branchHashes {
			hashSet[h] = true
		}

		var dbCommits []db.Commit
		for _, c := range allProjectCommits {
			if hashSet[c.CommitHash] {
				dbCommits = append(dbCommits, c)
			}
		}

		// Collect commit IDs for bulk agent coverage lookup.
		commitIDs := make([]string, 0, len(dbCommits))
		for _, c := range dbCommits {
			commitIDs = append(commitIDs, c.ID)
		}
		agentCovMap, agentCovErr := db.ListCommitAgentCoverageByCommitIDs(r.Context(), s.DB, commitIDs)
		if agentCovErr != nil {
			log.Printf("warning: failed to list agent coverage: %v", agentCovErr)
		}

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
	branches, _ := s.listRepoBranches(r.Context(), repoProject.Path, defaultBranch)
	remote := ensureProjectRemote(r.Context(), s.DB, repoProject)

	ignorePatterns := groupIgnoreDiffPatterns(group)

	if commitHash == workingCopyCommitHash {
		identity, identErr := resolveGitIdentity(r.Context(), repoProject.Path)
		if identErr != nil {
			writeError(w, http.StatusNotFound, "git identity not configured for project")
			return
		}
		commits, listErr := listCommitsByIdentity(r.Context(), repoProject.Path, branch, identity)
		if listErr != nil {
			log.Printf("error listing commits for working copy: %v", listErr)
		}
		coverage, attribution, messages, diffText, files, ok := computeWorkingCopyDetail(
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
			Branch:      branch,
			Commit:      coverage,
			Attribution: attribution,
			Diff:        diffText,
			Files:       files,
			Messages:    messages,
		})
		return
	}

	// Try to load from database first (no branch filter).
	dbCommit, err := db.GetCommitByProjectAndHash(r.Context(), s.DB, repoProject.ID, commitHash)
	if err != nil {
		log.Printf("error checking db for commit %s: %v", commitHash, err)
	}

	var commit gitCommit
	var commitDiff string
	var tokenDiff string

	if dbCommit != nil && dbCommit.NeedsParent {
		// Shallow boundary commit — return stub response with no diff/attribution.
		writeSuccess(w, http.StatusOK, projectCommitDetailResponse{
			Branch:   branch,
			Branches: branches,
			Commit: projectCommitCoverage{
				ProjectID:        project.ID,
				ProjectLabel:     project.Label,
				ProjectPath:      project.Path,
				ProjectGitID:     project.GitID,
				CommitHash:       dbCommit.CommitHash,
				Subject:          dbCommit.Subject,
				UserName:         dbCommit.UserName,
				UserEmail:        dbCommit.UserEmail,
				AuthoredAtUnixMs: dbCommit.AuthoredAt * 1000,
				NeedsParent:      true,
			},
		})
		return
	}

	if dbCommit != nil {
		// Use stored diff from database.
		commit = gitCommit{
			Hash:          dbCommit.CommitHash,
			Subject:       dbCommit.Subject,
			UserName:      dbCommit.UserName,
			UserEmail:     dbCommit.UserEmail,
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
		// Fallback: get commit metadata directly from git (no identity filter).
		gc, gcErr := getCommitMetadata(r.Context(), repoProject.Path, commitHash)
		if gcErr != nil {
			writeError(w, http.StatusNotFound, "commit not found")
			return
		}
		commit = *gc

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

	detailAdded, detailRemoved := countDiffAddedRemoved(commitDiff)

	// Check attribution cache for immutable commits.
	detailCacheKey := commitDetailCacheKey(project.ID, commit.Hash, ignorePatterns)
	var files []commitFileCoverage
	var agentSegments []agentCoverageSegment
	var contribMessages []commitContributionMessage
	var matchedLines, matchedChars, exactMatchedLines, fallbackLines, fallbackChars, totalLines, totalChars int

	cached, cacheHit := s.commitDetailCache.get(detailCacheKey)

	if cacheHit {
		files = cached.files
		agentSegments = cached.agentSegments
		contribMessages = cached.contribs
		matchedLines = cached.matchedLines
		matchedChars = cached.matchedChars
		exactMatchedLines = cached.exactMatched
		fallbackLines = cached.fallbackLines
		fallbackChars = cached.fallbackChars
		totalLines = cached.totalLines
		totalChars = cached.totalChars
	} else {
		commitTokens := parseUnifiedDiffTokens(tokenDiff, ignorePatterns)

		// Determine the time window for message matching.
		windowStart := commit.TimestampUnix*1000 - defaultMessageWindowMs
		windowEnd := commit.TimestampUnix*1000 + commitWindowLookaheadMs

		messages, msgErr := listDerivedDiffMessages(r.Context(), s.DB, projectIDs(group), windowStart, windowEnd)
		if msgErr != nil {
			log.Printf("error listing derived diff messages: %v", msgErr)
			writeError(w, http.StatusInternalServerError, "failed to load matching messages")
			return
		}

		var fileAgent map[string]commitFileCoverage
		var remainingNorms map[string]int
		contribMessages, matchedLines, matchedChars, fileAgent, remainingNorms = attributeCommitToMessages(commitTokens, messages, windowStart, windowEnd)
		exactMatchedLines = matchedLines
		totalLines, totalChars = tokenTotals(commitTokens)
		files, fallbackLines, fallbackChars = summarizeDiffFiles(commitDiff, ignorePatterns, commitTokens, fileAgent, remainingNorms)
		matchedLines += fallbackLines
		matchedChars += fallbackChars

		agentSegments = attributeCopiedFromAgentFiles(files, commitTokens, messages, windowStart, windowEnd, totalLines)

		s.commitDetailCache.set(detailCacheKey, &commitDetailCacheEntry{
			files:         files,
			agentSegments: agentSegments,
			contribs:      contribMessages,
			matchedLines:  matchedLines,
			matchedChars:  matchedChars,
			exactMatched:  exactMatchedLines,
			fallbackLines: fallbackLines,
			fallbackChars: fallbackChars,
			totalLines:    totalLines,
			totalChars:    totalChars,
			fetchedAt:     time.Now(),
		})
	}

	detailLinePercent := percentage(matchedLines, totalLines)
	var detailOverride *float64
	if dbCommit != nil && dbCommit.OverrideLinePercent != nil {
		detailLinePercent = *dbCommit.OverrideLinePercent
		detailOverride = dbCommit.OverrideLinePercent
	}

	writeSuccess(w, http.StatusOK, projectCommitDetailResponse{
		Branch:    branch,
		Branches:  branches,
		CommitURL: commitURL(remote, commit.Hash),
		Commit: projectCommitCoverage{
			ProjectID:           project.ID,
			ProjectLabel:        project.Label,
			ProjectPath:         project.Path,
			ProjectGitID:        project.GitID,
			CommitHash:          commit.Hash,
			Subject:             commit.Subject,
			AuthoredAtUnixMs:    commit.TimestampUnix * 1000,
			LinesTotal:          totalLines,
			LinesFromAgent:      matchedLines,
			LinePercent:         detailLinePercent,
			CharsTotal:          totalChars,
			CharsFromAgent:      matchedChars,
			CharacterPercent:    percentage(matchedChars, totalChars),
			LinesAdded:          detailAdded,
			LinesRemoved:        detailRemoved,
			AgentSegments:       agentSegments,
			OverrideLinePercent: detailOverride,
		},
		Attribution: commitAttribution{
			ExactMatchedLines:    exactMatchedLines,
			FallbackMatchedLines: fallbackLines,
			HasFallback:          fallbackLines > 0 || fallbackChars > 0,
			MatchedMessagesCount: len(contribMessages),
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
	if pageSize > 1000 {
		pageSize = 1000
	}

	// Client timezone offset in minutes (JS getTimezoneOffset convention).
	// UTC+7 sends -420, UTC-8 sends 480. Default to 0 (UTC).
	tzOffsetMin := 0
	if raw := r.URL.Query().Get("tzOffset"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			tzOffsetMin = v
		}
	}
	clientLoc := time.FixedZone("client", -tzOffsetMin*60)

	// Parse comma-separated user emails for multi-user filtering.
	var userEmails []string
	if raw := strings.TrimSpace(r.URL.Query().Get("user")); raw != "" {
		for _, part := range strings.Split(raw, ",") {
			if e := strings.TrimSpace(part); e != "" {
				userEmails = append(userEmails, e)
			}
		}
	}

	agentFilter := strings.TrimSpace(r.URL.Query().Get("agent"))
	searchTerm := strings.TrimSpace(r.URL.Query().Get("search"))
	orderAsc := strings.TrimSpace(r.URL.Query().Get("order")) == "asc"

	// Optional date range filter (unix ms).
	var dateFromSec, dateToSec int64
	if raw := strings.TrimSpace(r.URL.Query().Get("start")); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
			dateFromSec = v / 1000
		}
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("end")); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
			dateToSec = v / 1000
		}
	}
	dailyWindowDays := 30
	enforceDailyMinWindow := true
	if raw := strings.TrimSpace(r.URL.Query().Get("dailyWindowDays")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			if v > 3650 {
				v = 3650
			}
			dailyWindowDays = v
			enforceDailyMinWindow = false
		}
	}
	var dailyWindowEnd *time.Time
	if raw := strings.TrimSpace(r.URL.Query().Get("dailyWindowEnd")); raw != "" {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
			t := time.UnixMilli(v).In(clientLoc)
			dailyWindowEnd = &t
		}
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
		if current := detectCurrentBranch(r.Context(), project.Path); current != "" {
			branch = current
		} else {
			branch = strings.TrimSpace(project.DefaultBranch)
			if branch == "" {
				branch = "main"
			}
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
	branches, _ := s.listRepoBranches(r.Context(), repoProject.Path, defaultBranch)

	identity, err := resolveGitIdentity(r.Context(), repoProject.Path)
	if err != nil {
		writeError(w, http.StatusNotFound, "git identity not configured for project")
		return
	}

	syncState, err := db.GetCommitSyncState(r.Context(), s.DB, repoProject.ID, branch)
	if err != nil {
		log.Printf("error loading commit sync state for %s: %v", repoProject.Path, err)
	}

	// Use git hash list as source of truth for branch membership.
	allHashes, err := listBranchCommitHashes(r.Context(), repoProject.Path, branch)
	if err != nil {
		log.Printf("error listing branch hashes for %s: %v", repoProject.Path, err)
		writeError(w, http.StatusInternalServerError, "failed to list branch commits")
		return
	}
	if searchTerm != "" {
		filteredHashes, searchErr := db.FilterCommitHashesBySearch(
			r.Context(),
			s.DB,
			repoProject.ID,
			allHashes,
			searchTerm,
		)
		if searchErr != nil {
			log.Printf("error filtering commit hashes by search for %s: %v", repoProject.Path, searchErr)
			writeError(w, http.StatusInternalServerError, "failed to search commits")
			return
		}
		allHashes = filteredHashes
	}

	// Check how many of these hashes exist in DB.
	existing, err := db.ExistingCommitHashes(r.Context(), s.DB, repoProject.ID, allHashes)
	if err != nil {
		log.Printf("error checking existing hashes for %s: %v", repoProject.Path, err)
	}

	// Collect specific missing hashes from recent commits.
	var missingHashes []string
	checkLimit := defaultIngestCount
	if checkLimit > len(allHashes) {
		checkLimit = len(allHashes)
	}
	for _, h := range allHashes[:checkLimit] {
		if !existing[h] {
			missingHashes = append(missingHashes, h)
		}
	}

	// Also check if the current user's latest commit is ingested (for auto-refresh).
	if head, headErr := latestCommitByIdentity(r.Context(), repoProject.Path, branch, identity); headErr == nil && head != nil {
		if !existing[head.Hash] {
			// Avoid duplicates if already in missingHashes.
			found := false
			for _, h := range missingHashes {
				if h == head.Hash {
					found = true
					break
				}
			}
			if !found {
				missingHashes = append(missingHashes, head.Hash)
			}
		}
	}

	// Enqueue async ingestion for missing commits.
	if len(missingHashes) > 0 {
		s.enqueueCommitIngestion(repoProject, group, branch, missingHashes)
	}

	// Check for stale coverage using hash-based query.
	staleCoverage := false
	staleCoverage, err = db.HasStaleCommitCoverageByHashes(r.Context(), s.DB, repoProject.ID, allHashes, currentCommitCoverageVersion)
	if err != nil {
		log.Printf("error checking stale coverage for %s: %v", repoProject.Path, err)
		staleCoverage = false
	}

	// Get users and totals from hash-based queries.
	users, usersErr := db.ListDistinctUsersByHashes(r.Context(), s.DB, repoProject.ID, allHashes)
	if usersErr != nil {
		log.Printf("warning: failed to list distinct users: %v", usersErr)
	}
	total, totalErr := db.CountCommitsByHashes(r.Context(), s.DB, repoProject.ID, allHashes)
	if totalErr != nil {
		log.Printf("warning: failed to count commits: %v", totalErr)
	}

	// Compute filtered total for pagination when an author filter is active.
	filteredTotal := total
	if len(userEmails) > 0 {
		filteredTotal, err = db.CountCommitsByHashesAndUser(r.Context(), s.DB, repoProject.ID, allHashes, userEmails)
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

	// Query the page of commits using hash-based query.
	dbCommits, err := db.ListCommitsByHashesAndUserOrdered(r.Context(), s.DB, repoProject.ID, allHashes, userEmails, pageSize, offset, orderAsc)
	if err != nil {
		log.Printf("error listing commits from db for %s: %v", repoProject.Path, err)
		writeError(w, http.StatusInternalServerError, "failed to list commits")
		return
	}

	// For summary: get all project commits from DB and intersect with git hash set.
	// This avoids huge IN clauses for summary computation.
	allProjectCommits, err := db.ListAllCommitsByProject(r.Context(), s.DB, repoProject.ID, 0, 0)
	if err != nil {
		log.Printf("error listing all project commits for %s: %v", repoProject.Path, err)
		writeError(w, http.StatusInternalServerError, "failed to list commits")
		return
	}

	// Build hash set for fast lookup.
	hashSet := make(map[string]bool, len(allHashes))
	for _, h := range allHashes {
		hashSet[h] = true
	}

	// Build a set for fast user email lookup.
	userEmailSet := make(map[string]bool, len(userEmails))
	for _, e := range userEmails {
		userEmailSet[strings.ToLower(e)] = true
	}

	// Filter to only commits on this branch (and optional date range).
	var branchCommits []db.Commit
	for _, c := range allProjectCommits {
		if hashSet[c.CommitHash] {
			if len(userEmails) == 0 || userEmailSet[strings.ToLower(c.UserEmail)] {
				if dateFromSec > 0 && c.AuthoredAt < dateFromSec {
					continue
				}
				if dateToSec > 0 && c.AuthoredAt >= dateToSec {
					continue
				}
				branchCommits = append(branchCommits, c)
			}
		}
	}

	// When a date filter is active, recompute pagination from date-filtered branchCommits.
	if dateFromSec > 0 || dateToSec > 0 {
		filteredTotal = len(branchCommits)
		totalPages = 0
		if filteredTotal > 0 {
			totalPages = (filteredTotal + pageSize - 1) / pageSize
		}
		if totalPages > 0 && page > totalPages {
			page = totalPages
		}
		offset = (page - 1) * pageSize
		if offset < 0 {
			offset = 0
		}
		// Re-query page from date-filtered hashes.
		dateFilteredHashes := make([]string, 0, len(branchCommits))
		for _, c := range branchCommits {
			dateFilteredHashes = append(dateFilteredHashes, c.CommitHash)
		}
		dbCommits, err = db.ListCommitsByHashesAndUserOrdered(r.Context(), s.DB, repoProject.ID, dateFilteredHashes, nil, pageSize, offset, orderAsc)
		if err != nil {
			log.Printf("error listing date-filtered commits for %s: %v", repoProject.Path, err)
			writeError(w, http.StatusInternalServerError, "failed to list commits")
			return
		}
	}

	// Collect all commit IDs for agent coverage lookup.
	allCommitIDs := make([]string, 0, len(branchCommits))
	for _, c := range branchCommits {
		allCommitIDs = append(allCommitIDs, c.ID)
	}
	agentCovMap, _ := db.ListCommitAgentCoverageByCommitIDs(r.Context(), s.DB, allCommitIDs)

	// Get distinct agents for the filter dropdown.
	agents, _ := db.ListDistinctAgentsByCommitIDs(r.Context(), s.DB, allCommitIDs)

	// Apply agent filter: narrow branchCommits and recompute pagination.
	if agentFilter != "" {
		matchingIDs, agentErr := db.ListCommitIDsByAgent(r.Context(), s.DB, allCommitIDs, agentFilter)
		if agentErr != nil {
			log.Printf("error filtering by agent %s: %v", agentFilter, agentErr)
			writeError(w, http.StatusInternalServerError, "failed to filter by agent")
			return
		}
		var filtered []db.Commit
		for _, c := range branchCommits {
			if matchingIDs[c.ID] {
				filtered = append(filtered, c)
			}
		}
		branchCommits = filtered

		// Rebuild allHashes from filtered commits for hash-based queries below.
		filteredHashSet := make(map[string]bool, len(branchCommits))
		for _, c := range branchCommits {
			filteredHashSet[c.CommitHash] = true
		}
		var filteredHashes []string
		for _, h := range allHashes {
			if filteredHashSet[h] {
				filteredHashes = append(filteredHashes, h)
			}
		}

		// Recompute filtered total and re-query page.
		filteredTotal = len(branchCommits)
		totalPages = 0
		if filteredTotal > 0 {
			totalPages = (filteredTotal + pageSize - 1) / pageSize
		}
		if totalPages > 0 && page > totalPages {
			page = totalPages
		}
		offset = (page - 1) * pageSize
		if offset < 0 {
			offset = 0
		}
		dbCommits, err = db.ListCommitsByHashesAndUserOrdered(r.Context(), s.DB, repoProject.ID, filteredHashes, userEmails, pageSize, offset, orderAsc)
		if err != nil {
			log.Printf("error listing agent-filtered commits for %s: %v", repoProject.Path, err)
			writeError(w, http.StatusInternalServerError, "failed to list commits")
			return
		}
	}

	// Convert DB commits to coverage structs for the current page.
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

	// Compute summary from all branch commits.
	allCoverage := make([]projectCommitCoverage, 0, len(branchCommits))
	for _, c := range branchCommits {
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

	// Add working copy on page 1 when no author filter or filter includes current identity.
	if page == 1 && (len(userEmails) == 0 || userEmailSet[strings.ToLower(identity.Email)]) {
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
	_ = refreshQueued

	writeSuccess(w, http.StatusOK, projectCommitPageResponse{
		Branch:               branch,
		Branches:             branches,
		Users:                users,
		UserFilter:           strings.Join(userEmails, ","),
		Agents:               agents,
		AgentFilter:          agentFilter,
		CurrentUser:          identity.Name,
		CurrentEmail:         identity.Email,
		ExtraLocalUserEmails: s.loadExtraLocalUserEmails(),
		Project:              *project,
		Refresh:              makeCommitRefreshState(syncState, staleCoverage),
		Summary:      summarizeCommitCoverage(allCoverage),
		DailySummary: buildDailySummaryWindow(
			allCoverage,
			dailyWindowDays,
			clientLoc,
			dailyWindowEnd,
			enforceDailyMinWindow,
		),
		Pagination: projectCommitPagination{
			Page:       page,
			PageSize:   pageSize,
			Total:      filteredTotal,
			TotalPages: totalPages,
		},
		Commits: paged,
	})
}

func (s *Server) enqueueCommitIngestion(repoProject *db.Project, group projectGroup, branch string, missingHashes []string) bool {
	key := repoProject.ID + ":" + branch
	if !s.commitIngestJobs.tryStart(key) {
		return false
	}

	go s.runCommitIngestion(*repoProject, group, branch, missingHashes, key)
	return true
}

func (s *Server) runCommitIngestion(repoProject db.Project, group projectGroup, branch string, missingHashes []string, key string) {
	defer s.commitIngestJobs.finish(key)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if s.ws != nil {
		s.ws.broadcastEvent("job_status", jobStatusEvent{
			JobType:   "commit_ingest",
			State:     "running",
			Message:   fmt.Sprintf("Ingesting %d commit(s) for %s...", len(missingHashes), db.RepoLabel(repoProject.Path)),
			ProjectID: repoProject.ID,
			Branch:    branch,
		})
	}

	identity, _ := resolveGitIdentity(ctx, repoProject.Path)
	extraEmails := s.loadExtraLocalUserEmails()
	if _, err := ingestMissingCommits(ctx, s.DB, &repoProject, group, branch, missingHashes, &identity, extraEmails); err != nil {
		log.Printf("async commit ingestion failed for %s: %v", repoProject.Path, err)
		if s.ws != nil {
			s.ws.broadcastEvent("job_status", jobStatusEvent{
				JobType:   "commit_ingest",
				State:     "error",
				Message:   fmt.Sprintf("Commit ingestion failed for %s", db.RepoLabel(repoProject.Path)),
				ProjectID: repoProject.ID,
				Branch:    branch,
			})
		}
		return
	}

	if s.ws != nil {
		s.ws.broadcastEvent("job_status", jobStatusEvent{
			JobType:   "commit_ingest",
			State:     "complete",
			Message:   fmt.Sprintf("Ingested %d commit(s) for %s", len(missingHashes), db.RepoLabel(repoProject.Path)),
			ProjectID: repoProject.ID,
			Branch:    branch,
		})
	}
}

func (s *Server) handleSetCommitOverrideLinePercent(w http.ResponseWriter, r *http.Request) {
	if !requireJSON(w, r) {
		return
	}

	projectID := strings.TrimSpace(r.PathValue("projectId"))
	commitHash := strings.TrimSpace(r.PathValue("commitHash"))
	if projectID == "" || commitHash == "" {
		writeError(w, http.StatusBadRequest, "project id and commit hash are required")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var body struct {
		OverrideLinePercent *float64 `json:"overrideLinePercent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if body.OverrideLinePercent != nil {
		v := *body.OverrideLinePercent
		if v < 0 || v > 100 {
			writeError(w, http.StatusBadRequest, "overrideLinePercent must be between 0 and 100")
			return
		}
	}

	if err := db.SetCommitOverrideLinePercent(r.Context(), s.DB, projectID, commitHash, body.OverrideLinePercent); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "commit not found")
			return
		}
		log.Printf("error setting commit override: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to set override")
		return
	}

	writeSuccess(w, http.StatusOK, map[string]any{
		"projectId":           projectID,
		"commitHash":          commitHash,
		"overrideLinePercent": body.OverrideLinePercent,
	})
}

func (s *Server) handleRecalculateCommitDiffMatch(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.PathValue("projectId"))
	commitHash := strings.TrimSpace(r.PathValue("commitHash"))
	if projectID == "" || commitHash == "" {
		writeError(w, http.StatusBadRequest, "project id and commit hash are required")
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

	identity, _ := resolveGitIdentity(r.Context(), repoProject.Path)
	n, err := recomputeCommitCoverageForProjectHashes(
		r.Context(), s.DB, repoProject, group, []string{commitHash}, nil, &identity, s.loadExtraLocalUserEmails(),
	)
	if err != nil {
		log.Printf("error recalculating diff match for commit %s: %v", commitHash, err)
		writeError(w, http.StatusInternalServerError, "failed to recalculate diff match")
		return
	}

	s.commitDetailCache.clearProject(projectID)

	writeSuccess(w, http.StatusOK, map[string]any{
		"projectId":  projectID,
		"commitHash": commitHash,
		"recomputed": n,
	})
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
		latest, getErr := db.GetCommitByProjectAndHash(ctx, database, repoProject.ID, head.Hash)
		return getErr != nil || latest == nil
	}
	if syncState.LastProcessedHeadHash == "" {
		latest, getErr := db.GetCommitByProjectAndHash(ctx, database, repoProject.ID, head.Hash)
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
	return listDerivedDiffMessagesWithHidden(ctx, database, projectIDs, minTs, maxTs, false)
}

func listDerivedDiffMessagesWithHidden(
	ctx context.Context,
	database *sql.DB,
	projectIDs []string,
	minTs, maxTs int64,
	includeHidden bool,
) ([]messageDiff, error) {
	if len(projectIDs) == 0 {
		return nil, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(projectIDs)), ",")
	hiddenClause := "AND COALESCE(c.hidden, 0) = 0"
	if includeHidden {
		hiddenClause = ""
	}
	query := fmt.Sprintf(
		`SELECT m.id, m.timestamp, m.conversation_id, c.title, c.agent, m.model, m.content, m.raw_json
		 FROM messages m
		 JOIN conversations c ON c.id = m.conversation_id
		 WHERE m.role = 'agent'
		   AND m.timestamp BETWEEN ? AND ?
		   AND m.project_id IN (%s)
		   %s
		 ORDER BY m.timestamp, m.id`,
		placeholders, hiddenClause,
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

func computeWorkingCopyDetail(
	ctx context.Context,
	database *sql.DB,
	repoProject *db.Project,
	projectIDs []string,
	ignorePatterns []string,
	commits []gitCommit,
) (projectCommitCoverage, commitAttribution, []commitContributionMessage, string, []commitFileCoverage, bool) {
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
		return projectCommitCoverage{}, commitAttribution{}, nil, "", nil, false
	}
	commitTokens := parseUnifiedDiffTokens(diffText, ignorePatterns)
	if len(commitTokens) == 0 {
		return projectCommitCoverage{}, commitAttribution{}, nil, "", nil, false
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
		return projectCommitCoverage{}, commitAttribution{}, nil, "", nil, false
	}

	totalLines, totalChars := tokenTotals(commitTokens)
	contribMessages, matchedLines, matchedChars, fileAgent, remainingNorms := attributeCommitToMessages(commitTokens, messages, windowStart, windowEnd)
	exactMatchedLines := matchedLines
	files, fallbackLines, fallbackChars := summarizeDiffFiles(diffText, ignorePatterns, commitTokens, fileAgent, remainingNorms)
	matchedLines += fallbackLines
	matchedChars += fallbackChars
	wcAdded, wcRemoved := countDiffAddedRemoved(diffText)

	agentSegments := attributeCopiedFromAgentFiles(files, commitTokens, messages, windowStart, windowEnd, totalLines)

	attribution := commitAttribution{
		ExactMatchedLines:    exactMatchedLines,
		FallbackMatchedLines: fallbackLines,
		HasFallback:          fallbackLines > 0 || fallbackChars > 0,
		MatchedMessagesCount: len(contribMessages),
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
	}, attribution, contribMessages, diffText, files, true
}

func parsePositiveInt(raw string, fallback int) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || v <= 0 {
		return fallback
	}
	return v
}
