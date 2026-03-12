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
	"sync"
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
		if err := IngestDefaultCommits(r.Context(), s.DB, repoProject, group, identity, s.loadExtraLocalUserEmails(), branch, nil); err != nil {
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
		commit = gitCommit{
			Hash:          dbCommit.CommitHash,
			Subject:       dbCommit.Subject,
			UserName:      dbCommit.UserName,
			UserEmail:     dbCommit.UserEmail,
			TimestampUnix: dbCommit.AuthoredAt,
		}
		commitDiff = dbCommit.DiffContent
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
		}
	} else {
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
	}

	detailAdded, detailRemoved := countDiffAddedRemoved(commitDiff)

	var files []commitFileCoverage
	var agentSegments []agentCoverageSegment
	var contribMessages []commitContributionMessage
	var matchedLines, exactMatchedLines, fallbackLines, totalLines int

	// Fast path: use pre-computed detail data from the DB.
	if dbCommit != nil && dbCommit.DetailFiles != "" {
		if err := json.Unmarshal([]byte(dbCommit.DetailFiles), &files); err != nil {
			log.Printf("warning: failed to unmarshal detail_files for %s: %v", commitHash, err)
		}
		if dbCommit.DetailMessages != "" {
			if err := json.Unmarshal([]byte(dbCommit.DetailMessages), &contribMessages); err != nil {
				log.Printf("warning: failed to unmarshal detail_messages for %s: %v", commitHash, err)
			}
		}
		if dbCommit.DetailAgentSegments != "" {
			if err := json.Unmarshal([]byte(dbCommit.DetailAgentSegments), &agentSegments); err != nil {
				log.Printf("warning: failed to unmarshal detail_agent_segments for %s: %v", commitHash, err)
			}
		}
		exactMatchedLines = dbCommit.DetailExactMatched
		fallbackLines = dbCommit.DetailFallbackLines
		matchedLines = exactMatchedLines + fallbackLines
		totalLines = dbCommit.LinesTotal
	} else {
		// Fallback: compute on-the-fly, then persist for next time.
		stub := db.Commit{
			ProjectID:  repoProject.ID,
			BranchName: branch,
			CommitHash: commit.Hash,
			Subject:    commit.Subject,
			UserName:   commit.UserName,
			UserEmail:  commit.UserEmail,
			AuthoredAt: commit.TimestampUnix,
		}
		if dbCommit != nil {
			stub.ID = dbCommit.ID
			stub.DiffContent = dbCommit.DiffContent
		}

		windowStart := commit.TimestampUnix*1000 - defaultMessageWindowMs
		windowEnd := commit.TimestampUnix*1000 + commitWindowLookaheadMs
		messages, msgErr := listDerivedDiffMessages(r.Context(), s.DB, projectIDs(group), windowStart, windowEnd)
		if msgErr != nil {
			log.Printf("error listing derived diff messages: %v", msgErr)
			writeError(w, http.StatusInternalServerError, "failed to load matching messages")
			return
		}

		result, recompErr := recomputeSingleCommit(r.Context(), repoProject.Path, stub, ignorePatterns, messages, nil, nil, nil, nil)
		if recompErr != nil {
			log.Printf("error recomputing commit %s: %v", commitHash, recompErr)
			writeError(w, http.StatusInternalServerError, "failed to compute attribution")
			return
		}

		files = result.Files
		agentSegments = result.AgentSegments
		contribMessages = result.ContribMsgs
		exactMatchedLines = result.ExactMatched
		fallbackLines = result.FallbackLines
		matchedLines = result.Commit.LinesFromAgent
		totalLines = result.Commit.LinesTotal
		commitDiff = result.Commit.DiffContent

		// Persist detail data to DB for next load.
		if dbCommit != nil {
			result.Commit.ID = dbCommit.ID
			if err := db.UpsertCommit(r.Context(), s.DB, result.Commit); err != nil {
				log.Printf("warning: failed to persist detail cache for %s: %v", commitHash, err)
			}
		}
	}

	detailLinePercent := percentage(matchedLines, totalLines)
	var detailOverrideMap map[string]int
	if dbCommit != nil && dbCommit.OverrideAgentPercents != nil && *dbCommit.OverrideAgentPercents != "" {
		if err := json.Unmarshal([]byte(*dbCommit.OverrideAgentPercents), &detailOverrideMap); err != nil {
			log.Printf("warning: failed to unmarshal override_agent_percents for %s: %v", commitHash, err)
		} else {
			total := 0
			for _, v := range detailOverrideMap {
				total += v
			}
			detailLinePercent = float64(total)
			// Build agent segments from override map.
			overrideSegments := make([]agentCoverageSegment, 0, len(detailOverrideMap))
			overrideAgentNames := make([]string, 0, len(detailOverrideMap))
			for a := range detailOverrideMap {
				overrideAgentNames = append(overrideAgentNames, a)
			}
			sort.Strings(overrideAgentNames)
			for _, a := range overrideAgentNames {
				overrideSegments = append(overrideSegments, agentCoverageSegment{
					Agent:       a,
					LinePercent: float64(detailOverrideMap[a]),
				})
			}
			agentSegments = overrideSegments
		}
	}

	// Collect sorted agent names from segments.
	agentNameSet := make(map[string]struct{})
	for _, seg := range agentSegments {
		agentNameSet[seg.Agent] = struct{}{}
	}
	detailAgents := make([]string, 0, len(agentNameSet))
	for a := range agentNameSet {
		detailAgents = append(detailAgents, a)
	}
	sort.Strings(detailAgents)

	writeSuccess(w, http.StatusOK, projectCommitDetailResponse{
		Branch:    branch,
		Branches:  branches,
		CommitURL: commitURL(remote, commit.Hash),
		Commit: projectCommitCoverage{
			ProjectID:             project.ID,
			ProjectLabel:          project.Label,
			ProjectPath:           project.Path,
			ProjectGitID:          project.GitID,
			CommitHash:            commit.Hash,
			Subject:               commit.Subject,
			AuthoredAtUnixMs:      commit.TimestampUnix * 1000,
			LinesTotal:            totalLines,
			LinesFromAgent:        matchedLines,
			LinePercent:           detailLinePercent,
			LinesAdded:            detailAdded,
			LinesRemoved:          detailRemoved,
			AgentSegments:         agentSegments,
			OverrideAgentPercents: detailOverrideMap,
		},
		Attribution: commitAttribution{
			ExactMatchedLines:    exactMatchedLines,
			FallbackMatchedLines: fallbackLines,
			HasFallback:          fallbackLines > 0,
			MatchedMessagesCount: len(contribMessages),
		},
		Diff:     commitDiff,
		Files:    files,
		Messages: contribMessages,
		Agents:   detailAgents,
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

	// Run git commands (branches, identity) in parallel — these are needed
	// for the response but are independent of DB queries.
	var (
		branches    []string
		identity    gitIdentity
		identityErr error
	)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		branches, _ = s.listRepoBranches(r.Context(), repoProject.Path, defaultBranch)
	}()
	go func() {
		defer wg.Done()
		identity, identityErr = resolveGitIdentity(r.Context(), repoProject.Path)
	}()
	wg.Wait()

	if identityErr != nil {
		writeError(w, http.StatusNotFound, "git identity not configured for project")
		return
	}

	// Resolve @me+agents sentinel to actual email addresses.
	for i, e := range userEmails {
		if e == "@me+agents" {
			resolved := []string{identity.Email}
			resolved = append(resolved, s.loadExtraLocalUserEmails()...)
			userEmails = append(userEmails[:i], append(resolved, userEmails[i+1:]...)...)
			break
		}
	}

	// All data queries use branch_name from the DB — no git hash list needed.
	users, _ := db.ListDistinctUsers(r.Context(), s.DB, repoProject.ID, branch)
	total, _ := db.CountCommitsByBranchAndUsers(r.Context(), s.DB, repoProject.ID, branch, nil, 0, 0)

	filteredTotal, err := db.CountCommitsByBranchAndUsers(r.Context(), s.DB, repoProject.ID, branch, userEmails, dateFromSec, dateToSec)
	if err != nil {
		log.Printf("error counting filtered commits for %s: %v", repoProject.Path, err)
		writeError(w, http.StatusInternalServerError, "failed to count commits")
		return
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
	// Search filter: when a search term is present, we need hash-based filtering
	// since FTS is indexed by content, not branch.
	if searchTerm != "" {
		allHashes, hashErr := listBranchCommitHashes(r.Context(), repoProject.Path, branch)
		if hashErr != nil {
			log.Printf("error listing branch hashes for search %s: %v", repoProject.Path, hashErr)
			writeError(w, http.StatusInternalServerError, "failed to search commits")
			return
		}
		filteredHashes, searchErr := db.FilterCommitHashesBySearch(r.Context(), s.DB, repoProject.ID, allHashes, searchTerm)
		if searchErr != nil {
			log.Printf("error filtering commit hashes by search for %s: %v", repoProject.Path, searchErr)
			writeError(w, http.StatusInternalServerError, "failed to search commits")
			return
		}
		// For search, fall back to hash-based pagination.
		dbCommits, err := db.ListCommitsByHashesAndUserOrdered(r.Context(), s.DB, repoProject.ID, filteredHashes, userEmails, pageSize, offset, orderAsc)
		if err != nil {
			log.Printf("error listing search-filtered commits for %s: %v", repoProject.Path, err)
			writeError(w, http.StatusInternalServerError, "failed to list commits")
			return
		}
		filteredTotal = len(filteredHashes)
		totalPages = 0
		if filteredTotal > 0 {
			totalPages = (filteredTotal + pageSize - 1) / pageSize
		}
		// Use the search results as the page commits and fall through to response.
		s.writeCommitResponse(w, r, dbCommits, repoProject, project, group, branch, branches, users, identity,
			userEmails, agentFilter, total, filteredTotal, totalPages, page, pageSize,
			dailyWindowDays, clientLoc, dailyWindowEnd, enforceDailyMinWindow,
			dateFromSec, dateToSec, orderAsc)
		return
	}
	dbCommits, err := db.ListCommitsByBranchAndUsers(r.Context(), s.DB, repoProject.ID, branch, userEmails, pageSize, offset, orderAsc, dateFromSec, dateToSec)
	if err != nil {
		log.Printf("error listing commits from db for %s: %v", repoProject.Path, err)
		writeError(w, http.StatusInternalServerError, "failed to list commits")
		return
	}
	// Fast ingestion check: only look at HEAD hash (~1ms git rev-parse)
	// instead of listing all branch hashes (~12ms git log).
	s.maybeIngestBranchHead(r.Context(), repoProject, group, branch, "")

	s.writeCommitResponse(w, r, dbCommits, repoProject, project, group, branch, branches, users, identity,
		userEmails, agentFilter, total, filteredTotal, totalPages, page, pageSize,
		dailyWindowDays, clientLoc, dailyWindowEnd, enforceDailyMinWindow,
		dateFromSec, dateToSec, orderAsc)
}

// maybeIngestBranchHead checks if the HEAD commit of a branch is ingested.
// If not, it fetches the recent hashes and enqueues ingestion for missing ones.
// This is much faster than listing all branch hashes (~1ms vs ~12ms).
func (s *Server) maybeIngestBranchHead(ctx context.Context, repoProject *db.Project, group projectGroup, branch, headHash string) {
	if strings.TrimSpace(headHash) == "" {
		var err error
		headHash, err = runGit(ctx, repoProject.Path, "rev-parse", branch)
		if err != nil {
			return
		}
	}
	headHash = strings.TrimSpace(headHash)
	if headHash == "" {
		return
	}
	existing, err := db.ExistingCommitHashes(ctx, s.DB, repoProject.ID, []string{headHash})
	if err != nil {
		return
	}
	if existing[headHash] {
		return // HEAD is already ingested
	}
	// HEAD is missing — fetch recent hashes (uncached) for ingestion.
	out, err := runGit(ctx, repoProject.Path, "log", branch, "--format=%H", fmt.Sprintf("--max-count=%d", defaultIngestCount))
	if err != nil {
		return
	}
	var recentHashes []string
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if h := strings.TrimSpace(line); h != "" {
			recentHashes = append(recentHashes, h)
		}
	}
	if len(recentHashes) == 0 {
		return
	}
	existingAll, err := db.ExistingCommitHashes(ctx, s.DB, repoProject.ID, recentHashes)
	if err != nil {
		return
	}
	var missingHashes []string
	for _, h := range recentHashes {
		if !existingAll[h] {
			missingHashes = append(missingHashes, h)
		}
	}
	if len(missingHashes) > 0 {
		s.enqueueCommitIngestion(repoProject.ID, branch, missingHashes)
	}
}

// writeCommitResponse builds the full response with agent coverage, summary,
// and working copy info, then writes it. It also triggers background ingestion.
func (s *Server) writeCommitResponse(
	w http.ResponseWriter, r *http.Request,
	dbCommits []db.Commit, repoProject *db.Project, project *db.Project,
	group projectGroup, branch string, branches []string, users []db.UserInfo,
	identity gitIdentity, userEmails []string, agentFilter string,
	total, filteredTotal, totalPages, page, pageSize int,
	dailyWindowDays int, clientLoc *time.Location, dailyWindowEnd *time.Time, enforceDailyMinWindow bool,
	dateFromSec, dateToSec int64, orderAsc bool,
) {
	// Get all branch commits for summary and agent coverage.
	branchCommits, err := db.ListCommitsByBranchAndUsers(r.Context(), s.DB, repoProject.ID, branch, userEmails, 10000, 0, false, dateFromSec, dateToSec)
	if err != nil {
		log.Printf("error listing branch commits for summary %s: %v", repoProject.Path, err)
		writeError(w, http.StatusInternalServerError, "failed to list commits")
		return
	}

	// Collect commit IDs for agent coverage lookup.
	allCommitIDs := make([]string, 0, len(branchCommits))
	for _, c := range branchCommits {
		allCommitIDs = append(allCommitIDs, c.ID)
	}
	agentCovMap, _ := db.ListCommitAgentCoverageByCommitIDs(r.Context(), s.DB, allCommitIDs)

	// Get distinct agents for the filter dropdown.
	agents, _ := db.ListDistinctAgentsByBranch(r.Context(), s.DB, repoProject.ID, branch, userEmails, dateFromSec, dateToSec)

	// Apply agent filter: recompute pagination from agent-filtered commits.
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

		filteredTotal = len(branchCommits)
		totalPages = 0
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
		// Re-query page from agent-filtered hashes.
		filteredHashes := make([]string, 0, len(branchCommits))
		for _, c := range branchCommits {
			filteredHashes = append(filteredHashes, c.CommitHash)
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

	// Build a set for fast user email lookup.
	userEmailSet := make(map[string]bool, len(userEmails))
	for _, e := range userEmails {
		userEmailSet[strings.ToLower(e)] = true
	}

	// Add working copy on page 1 when no author filter or filter includes current identity.
	if page == 1 && (len(userEmails) == 0 || userEmailSet[strings.ToLower(identity.Email)]) {
		if wc, ok := hasWorkingCopyChanges(r.Context(), repoProject); ok {
			paged = append([]projectCommitCoverage{wc}, paged...)
		}
	}

	// Check stale coverage and refresh state.
	staleCoverage, _ := db.HasStaleCommitCoverageByBranch(r.Context(), s.DB, repoProject.ID, branch, currentCommitCoverageVersion)
	syncState, _ := db.GetCommitSyncState(r.Context(), s.DB, repoProject.ID, branch)

	if shouldQueueCommitRefresh(r.Context(), s.DB, repoProject, identity, branch, total, syncState) {
		if refreshQueued, _ := s.enqueueCommitRefresh(repoProject.ID, branch); refreshQueued {
			syncState = &db.CommitSyncState{
				ProjectID:  repoProject.ID,
				BranchName: branch,
				State:      "queued",
			}
		}
	}

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
		Summary:              summarizeCommitCoverage(allCoverage),
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

func normalizeCommitIngestKey(projectID, branch string) string {
	return projectID + ":" + branch
}

func uniqueHashes(hashes []string) []string {
	seen := make(map[string]struct{}, len(hashes))
	out := make([]string, 0, len(hashes))
	for _, hash := range hashes {
		hash = strings.TrimSpace(hash)
		if hash == "" {
			continue
		}
		if _, ok := seen[hash]; ok {
			continue
		}
		seen[hash] = struct{}{}
		out = append(out, hash)
	}
	return out
}

func mergeUniqueHashes(existing, incoming []string) []string {
	merged := append([]string{}, existing...)
	seen := make(map[string]struct{}, len(existing))
	for _, hash := range existing {
		hash = strings.TrimSpace(hash)
		if hash == "" {
			continue
		}
		seen[hash] = struct{}{}
	}
	for _, hash := range incoming {
		hash = strings.TrimSpace(hash)
		if hash == "" {
			continue
		}
		if _, ok := seen[hash]; ok {
			continue
		}
		seen[hash] = struct{}{}
		merged = append(merged, hash)
	}
	return merged
}

func (s *Server) reserveCommitIngestion(projectID, branch string, missingHashes []string) (string, []string, bool, int) {
	if s.commitIngestJobs == nil {
		s.commitIngestJobs = newJobTracker()
	}
	if s.pendingCommitIngest == nil {
		s.pendingCommitIngest = make(map[string][]string)
	}
	missingHashes = uniqueHashes(missingHashes)
	if len(missingHashes) == 0 {
		return normalizeCommitIngestKey(projectID, branch), nil, false, 0
	}
	key := normalizeCommitIngestKey(projectID, branch)

	s.commitIngestMu.Lock()
	defer s.commitIngestMu.Unlock()

	if !s.commitIngestJobs.tryStart(key) {
		s.pendingCommitIngest[key] = mergeUniqueHashes(s.pendingCommitIngest[key], missingHashes)
		return key, nil, false, len(s.pendingCommitIngest[key])
	}
	return key, missingHashes, true, 0
}

func (s *Server) releaseCommitIngestion(key string) ([]string, bool) {
	if s.commitIngestJobs == nil {
		return nil, false
	}

	s.commitIngestMu.Lock()
	defer s.commitIngestMu.Unlock()

	s.commitIngestJobs.finish(key)
	pending := uniqueHashes(s.pendingCommitIngest[key])
	delete(s.pendingCommitIngest, key)
	if len(pending) == 0 {
		return nil, false
	}
	if !s.commitIngestJobs.tryStart(key) {
		s.pendingCommitIngest[key] = mergeUniqueHashes(s.pendingCommitIngest[key], pending)
		return nil, false
	}
	return pending, true
}

func (s *Server) enqueueCommitIngestion(projectID, branch string, missingHashes []string) (bool, bool, int) {
	key, missingHashes, started, pendingCount := s.reserveCommitIngestion(projectID, branch, missingHashes)
	if !started {
		if pendingCount > 0 {
			return true, false, pendingCount
		}
		return false, false, 0
	}

	go s.runCommitIngestion(projectID, branch, missingHashes, key)
	return true, true, 0
}

func (s *Server) runCommitIngestion(projectID, branch string, missingHashes []string, key string) {
	defer func() {
		nextHashes, restart := s.releaseCommitIngestion(key)
		if restart {
			go s.runCommitIngestion(projectID, branch, nextHashes, key)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	startedAt := time.Now().UnixMilli()

	covCtx, err := s.loadProjectCoverageContext(ctx, projectID)
	if err != nil {
		log.Printf("async commit ingestion context load failed for %s: %v", projectID, err)
		_ = db.UpsertCommitSyncState(context.Background(), s.DB, db.CommitSyncState{
			ProjectID:        projectID,
			BranchName:       branch,
			State:            "failed",
			LastStartedAtMs:  startedAt,
			LastFinishedAtMs: time.Now().UnixMilli(),
			LastDurationMs:   time.Now().UnixMilli() - startedAt,
			LastError:        err.Error(),
		})
		if s.ws != nil {
			s.ws.broadcastEvent("job_status", jobStatusEvent{
				JobType:   "commit_ingest",
				State:     "error",
				Message:   "Commit ingestion failed to load project context",
				ProjectID: projectID,
				Branch:    branch,
			})
		}
		return
	}

	headHash := ""
	if out, headErr := runGit(ctx, covCtx.repoProject.Path, "rev-parse", branch); headErr == nil {
		headHash = strings.TrimSpace(out)
	}
	_ = db.UpsertCommitSyncState(ctx, s.DB, db.CommitSyncState{
		ProjectID:           projectID,
		BranchName:          branch,
		State:               "running",
		LatestKnownHeadHash: headHash,
		LastStartedAtMs:     startedAt,
	})

	if s.ws != nil {
		s.ws.broadcastEvent("job_status", jobStatusEvent{
			JobType:   "commit_ingest",
			State:     "running",
			Message:   fmt.Sprintf("Ingesting %d commit(s) for %s...", len(missingHashes), db.RepoLabel(covCtx.repoProject.Path)),
			ProjectID: projectID,
			Branch:    branch,
		})
	}

	var ingestedCommits []db.Commit
	_, err = s.runStableCoverageStage(
		ctx,
		projectID,
		"commit_ingest",
		func() {
			if s.ws != nil {
				s.ws.broadcastEvent("job_status", jobStatusEvent{
					JobType:   "commit_ingest",
					State:     "running",
					Message:   "Project diff settings changed during commit ingestion; re-running with latest settings...",
					ProjectID: projectID,
					Branch:    branch,
				})
			}
		},
		func(stageCtx *projectCoverageContext) error {
			_, ingestErr := ingestMissingCommits(
				ctx,
				s.DB,
				stageCtx.repoProject,
				stageCtx.group,
				branch,
				missingHashes,
				&stageCtx.identity,
				stageCtx.extraEmails,
				func(c []db.Commit) { ingestedCommits = append(ingestedCommits, c...) },
			)
			return ingestErr
		},
	)
	if err == nil && len(ingestedCommits) > 0 {
		s.notifyIngestedCommits(ingestedCommits, db.RepoLabel(covCtx.repoProject.Path))
	}
	finishedAt := time.Now().UnixMilli()
	duration := finishedAt - startedAt
	if err != nil {
		log.Printf("async commit ingestion failed for %s: %v", projectID, err)
		_ = db.UpsertCommitSyncState(context.Background(), s.DB, db.CommitSyncState{
			ProjectID:           projectID,
			BranchName:          branch,
			State:               "failed",
			LatestKnownHeadHash: headHash,
			LastStartedAtMs:     startedAt,
			LastFinishedAtMs:    finishedAt,
			LastDurationMs:      duration,
			LastError:           err.Error(),
		})
		if s.ws != nil {
			s.ws.broadcastEvent("job_status", jobStatusEvent{
				JobType:   "commit_ingest",
				State:     "error",
				Message:   fmt.Sprintf("Commit ingestion failed for %s", db.RepoLabel(covCtx.repoProject.Path)),
				ProjectID: projectID,
				Branch:    branch,
			})
		}
		return
	}

	_ = db.UpsertCommitSyncState(context.Background(), s.DB, db.CommitSyncState{
		ProjectID:             projectID,
		BranchName:            branch,
		State:                 "idle",
		LatestKnownHeadHash:   headHash,
		LastProcessedHeadHash: headHash,
		LastStartedAtMs:       startedAt,
		LastFinishedAtMs:      finishedAt,
		LastDurationMs:        duration,
	})

	if s.ws != nil {
		s.ws.broadcastEvent("job_status", jobStatusEvent{
			JobType:   "commit_ingest",
			State:     "complete",
			Message:   fmt.Sprintf("Ingested %d commit(s) for %s", len(missingHashes), db.RepoLabel(covCtx.repoProject.Path)),
			ProjectID: projectID,
			Branch:    branch,
		})
	}
}

func (s *Server) handleSetCommitOverrideAgentPercents(w http.ResponseWriter, r *http.Request) {
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
		OverrideAgentPercents map[string]int `json:"overrideAgentPercents"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	sum := 0
	for agent, pct := range body.OverrideAgentPercents {
		if pct < 0 {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("agent %q percentage must be >= 0", agent))
			return
		}
		sum += pct
	}
	if sum > 100 {
		writeError(w, http.StatusBadRequest, "sum of agent percentages must be <= 100")
		return
	}

	if err := db.SetCommitOverrideAgentPercents(r.Context(), s.DB, projectID, commitHash, body.OverrideAgentPercents); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			writeError(w, http.StatusNotFound, "commit not found")
			return
		}
		log.Printf("error setting commit override agent percents: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to set override")
		return
	}

	writeSuccess(w, http.StatusOK, map[string]any{
		"projectId":             projectID,
		"commitHash":            commitHash,
		"overrideAgentPercents": body.OverrideAgentPercents,
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
) bool {
	if syncState != nil && (syncState.State == "queued" || syncState.State == "running") {
		return false
	}
	if total == 0 {
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
		parsed := parseUnifiedDiffTokensWithFiles(commitDiff, ignorePatterns)
		commitTokens := parsed.Tokens
		if len(commitTokens) == 0 {
			continue
		}

		windowStart := c.TimestampUnix*1000 - defaultMessageWindowMs
		windowEnd := c.TimestampUnix*1000 + commitWindowLookaheadMs

		totalLines := tokenTotals(commitTokens)
		_, matchedLines, fileAgent, remainingNorms := attributeCommitToMessages(commitTokens, messages, windowStart, windowEnd)
		_, fallbackLines := summarizeDiffFiles(parsed.Files, commitTokens, fileAgent, remainingNorms)
		matchedLines += fallbackLines

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
	parsed := parseUnifiedDiffTokensWithFiles(diffText, ignorePatterns)
	commitTokens := parsed.Tokens
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

	totalLines := tokenTotals(commitTokens)
	contribMessages, matchedLines, fileAgent, remainingNorms := attributeCommitToMessages(commitTokens, messages, windowStart, windowEnd)
	exactMatchedLines := matchedLines
	files, fallbackLines := summarizeDiffFiles(parsed.Files, commitTokens, fileAgent, remainingNorms)
	matchedLines += fallbackLines
	wcAdded, wcRemoved := countDiffAddedRemoved(diffText)

	agentSegments := attributeCopiedFromAgentFiles(files, commitTokens, messages, windowStart, windowEnd, totalLines)

	attribution := commitAttribution{
		ExactMatchedLines:    exactMatchedLines,
		FallbackMatchedLines: fallbackLines,
		HasFallback:          fallbackLines > 0,
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
