package handler

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"

)

type deepenCommitResponse struct {
	NeedsParent bool `json:"needsParent"`
	Success     bool `json:"success"`
}

func (s *Server) handleDeepenCommit(w http.ResponseWriter, r *http.Request) {
	projectID := strings.TrimSpace(r.PathValue("projectId"))
	commitHash := strings.TrimSpace(r.PathValue("commitHash"))
	if projectID == "" || commitHash == "" {
		writeError(w, http.StatusBadRequest, "project id and commit hash are required")
		return
	}

	branch := strings.TrimSpace(r.URL.Query().Get("branch"))
	if branch == "" {
		branch = "main"
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

	// Broadcast start.
	if s.ws != nil {
		s.ws.broadcastEvent("job_status", jobStatusEvent{
			JobType:   "commit_deepen",
			State:     "running",
			Message:   fmt.Sprintf("Fetching parent for commit %s...", commitHash[:minInt(12, len(commitHash))]),
			ProjectID: repoProject.ID,
			Branch:    branch,
		})
	}

	// Run git fetch --deepen=2.
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "git", "-C", repoProject.Path, "fetch", "--deepen=2", "origin", branch)
	output, fetchErr := cmd.CombinedOutput()
	outputStr := strings.TrimSpace(string(output))

	if s.ws != nil && outputStr != "" {
		s.ws.broadcastEvent("job_status", jobStatusEvent{
			JobType:   "commit_deepen",
			State:     "running",
			Message:   outputStr,
			ProjectID: repoProject.ID,
			Branch:    branch,
		})
	}

	if fetchErr != nil {
		log.Printf("git fetch --deepen=2 failed for %s: %v (output: %s)", repoProject.Path, fetchErr, outputStr)
		if s.ws != nil {
			s.ws.broadcastEvent("job_status", jobStatusEvent{
				JobType:   "commit_deepen",
				State:     "error",
				Message:   fmt.Sprintf("git fetch failed: %s", outputStr),
				ProjectID: repoProject.ID,
				Branch:    branch,
			})
		}
		writeSuccess(w, http.StatusOK, deepenCommitResponse{NeedsParent: true, Success: false})
		return
	}

	// Check if the commit is still a shallow boundary.
	stillShallow := false
	if shallow := shallowBoundaryHashes(ctx, repoProject.Path); shallow[commitHash] {
		stillShallow = true
	}

	if stillShallow {
		if s.ws != nil {
			s.ws.broadcastEvent("job_status", jobStatusEvent{
				JobType:   "commit_deepen",
				State:     "error",
				Message:   "Commit is still at the shallow boundary after deepening. Try again with a larger depth.",
				ProjectID: repoProject.ID,
				Branch:    branch,
			})
		}
		writeSuccess(w, http.StatusOK, deepenCommitResponse{NeedsParent: true, Success: false})
		return
	}

	// Re-ingest the single commit.
	identity, _ := resolveGitIdentity(ctx, repoProject.Path)
	extraEmails := s.loadExtraLocalUserEmails()

	gc, gcErr := getCommitMetadata(ctx, repoProject.Path, commitHash)
	if gcErr != nil {
		log.Printf("error getting commit metadata after deepen: %v", gcErr)
		if s.ws != nil {
			s.ws.broadcastEvent("job_status", jobStatusEvent{
				JobType:   "commit_deepen",
				State:     "error",
				Message:   "Failed to get commit metadata after deepening",
				ProjectID: repoProject.ID,
				Branch:    branch,
			})
		}
		writeSuccess(w, http.StatusOK, deepenCommitResponse{NeedsParent: false, Success: false})
		return
	}

	if _, err := ingestCommits(ctx, s.DB, repoProject, group, branch, []gitCommit{*gc}, &identity, extraEmails); err != nil {
		log.Printf("error re-ingesting commit after deepen: %v", err)
	}

	if s.ws != nil {
		s.ws.broadcastEvent("job_status", jobStatusEvent{
			JobType:   "commit_deepen",
			State:     "complete",
			Message:   fmt.Sprintf("Successfully fetched parent and recomputed commit %s", commitHash[:minInt(12, len(commitHash))]),
			ProjectID: repoProject.ID,
			Branch:    branch,
		})
	}

	writeSuccess(w, http.StatusOK, deepenCommitResponse{NeedsParent: false, Success: true})
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
