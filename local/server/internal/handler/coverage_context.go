package handler

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

const maxCoverageStageAttempts = 3

type projectCoverageContext struct {
	repoProject *db.Project
	group       projectGroup
	identity    gitIdentity
	extraEmails []string
	fingerprint string
}

func (s *Server) loadProjectCoverageContext(ctx context.Context, projectID string) (*projectCoverageContext, error) {
	groups, err := listAllProjectGroups(ctx, s.DB)
	if err != nil {
		return nil, fmt.Errorf("list project groups: %w", err)
	}
	group, ok := findProjectGroupByProjectID(groups, projectID)
	if !ok {
		return nil, fmt.Errorf("project group not found for %s", projectID)
	}
	repoProject, err := resolveRepoProject(ctx, group)
	if err != nil {
		return nil, err
	}
	identity, err := resolveGitIdentity(ctx, repoProject.Path)
	if err != nil {
		return nil, err
	}

	return &projectCoverageContext{
		repoProject: repoProject,
		group:       group,
		identity:    identity,
		extraEmails: s.loadExtraLocalUserEmails(),
		fingerprint: projectGroupFingerprint(group),
	}, nil
}

func projectGroupFingerprint(group projectGroup) string {
	projects := append([]db.Project(nil), group.Projects...)
	sort.Slice(projects, func(i, j int) bool {
		return projects[i].ID < projects[j].ID
	})

	var b strings.Builder
	b.WriteString(strings.TrimSpace(group.GitID))
	b.WriteByte('\n')
	for _, project := range projects {
		b.WriteString(project.ID)
		b.WriteByte('\n')
		b.WriteString(strconv.FormatBool(project.Ignored))
		b.WriteByte('\n')
		b.WriteString(strconv.FormatBool(project.IgnoreDefaultDiffPaths))
		b.WriteByte('\n')
		b.WriteString(strings.TrimSpace(project.IgnoreDiffPaths))
		b.WriteByte('\n')
	}
	return b.String()
}

func (s *Server) runStableCoverageStage(
	ctx context.Context,
	projectID string,
	stage string,
	onMismatch func(),
	run func(*projectCoverageContext) error,
) (*projectCoverageContext, error) {
	var latest *projectCoverageContext
	for attempt := 0; attempt < maxCoverageStageAttempts; attempt++ {
		covCtx, err := s.loadProjectCoverageContext(ctx, projectID)
		if err != nil {
			return nil, err
		}
		if err := run(covCtx); err != nil {
			return nil, err
		}
		if s.afterCoverageStage != nil {
			s.afterCoverageStage(projectID, stage)
		}
		latest, err = s.loadProjectCoverageContext(ctx, projectID)
		if err != nil {
			return nil, err
		}
		if latest.fingerprint == covCtx.fingerprint {
			return latest, nil
		}
		if onMismatch != nil {
			onMismatch()
		}
	}
	return nil, fmt.Errorf("%s superseded by repeated project settings changes", stage)
}
