package db

import (
	"context"
	"errors"
	"testing"
)

func TestListProjects(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	// Create projects in non-alphabetical order.
	for _, path := range []string{"/z/project", "/a/project", "/m/project"} {
		if _, err := EnsureProject(ctx, db, path); err != nil {
			t.Fatalf("EnsureProject %s: %v", path, err)
		}
	}

	projects, err := ListProjects(ctx, db, false)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 3 {
		t.Fatalf("got %d projects, want 3", len(projects))
	}

	// Should be ordered by path.
	if projects[0].Path != "/a/project" {
		t.Errorf("first project path = %q, want %q", projects[0].Path, "/a/project")
	}
	if projects[1].Path != "/m/project" {
		t.Errorf("second project path = %q, want %q", projects[1].Path, "/m/project")
	}
	if projects[2].Path != "/z/project" {
		t.Errorf("third project path = %q, want %q", projects[2].Path, "/z/project")
	}
}

func TestListProjectsEmpty(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	projects, err := ListProjects(ctx, db, false)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if projects == nil {
		t.Fatal("expected non-nil empty slice")
	}
	if len(projects) != 0 {
		t.Errorf("got %d projects, want 0", len(projects))
	}
}

func TestListProjectsFiltersByIgnored(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	pid1, err := EnsureProject(ctx, db, "/active/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	pid2, err := EnsureProject(ctx, db, "/ignored/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	// Ignore the second project.
	if err := SetProjectIgnored(ctx, db, pid2, true); err != nil {
		t.Fatalf("SetProjectIgnored: %v", err)
	}

	// Non-ignored should return only the active project.
	active, err := ListProjects(ctx, db, false)
	if err != nil {
		t.Fatalf("ListProjects(false): %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("got %d non-ignored projects, want 1", len(active))
	}
	if active[0].ID != pid1 {
		t.Errorf("active project id = %q, want %q", active[0].ID, pid1)
	}

	// Ignored should return only the ignored project.
	ignored, err := ListProjects(ctx, db, true)
	if err != nil {
		t.Fatalf("ListProjects(true): %v", err)
	}
	if len(ignored) != 1 {
		t.Fatalf("got %d ignored projects, want 1", len(ignored))
	}
	if ignored[0].ID != pid2 {
		t.Errorf("ignored project id = %q, want %q", ignored[0].ID, pid2)
	}
	if !ignored[0].Ignored {
		t.Error("expected Ignored = true")
	}
}

func TestSetProjectIgnored(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	pid, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	// Ignore the project.
	if err := SetProjectIgnored(ctx, db, pid, true); err != nil {
		t.Fatalf("SetProjectIgnored(true): %v", err)
	}

	// Verify it's ignored.
	detail, err := GetProjectDetail(ctx, db, pid)
	if err != nil {
		t.Fatalf("GetProjectDetail: %v", err)
	}
	if !detail.Ignored {
		t.Error("expected Ignored = true after setting")
	}

	// Un-ignore the project.
	if err := SetProjectIgnored(ctx, db, pid, false); err != nil {
		t.Fatalf("SetProjectIgnored(false): %v", err)
	}

	detail, err = GetProjectDetail(ctx, db, pid)
	if err != nil {
		t.Fatalf("GetProjectDetail: %v", err)
	}
	if detail.Ignored {
		t.Error("expected Ignored = false after unsetting")
	}
}

func TestSetProjectIgnoredNotFound(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	err := SetProjectIgnored(ctx, db, "nonexistent", true)
	if err == nil {
		t.Fatal("expected error for nonexistent project")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetProjectDetail(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	pid, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	// Add two conversations.
	if err := EnsureConversation(ctx, db, "conv-1", pid, "claude"); err != nil {
		t.Fatalf("EnsureConversation conv-1: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-2", pid, "claude"); err != nil {
		t.Fatalf("EnsureConversation conv-2: %v", err)
	}

	detail, err := GetProjectDetail(ctx, db, pid)
	if err != nil {
		t.Fatalf("GetProjectDetail: %v", err)
	}
	if detail == nil {
		t.Fatal("expected non-nil project detail")
	}
	if detail.Path != "/test/project" {
		t.Errorf("Path = %q, want %q", detail.Path, "/test/project")
	}
	if len(detail.Conversations) != 2 {
		t.Fatalf("got %d conversations, want 2", len(detail.Conversations))
	}
}

func TestGetProjectDetailNotFound(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	detail, err := GetProjectDetail(ctx, db, "nonexistent")
	if err != nil {
		t.Fatalf("GetProjectDetail: %v", err)
	}
	if detail != nil {
		t.Errorf("expected nil for nonexistent project, got %+v", detail)
	}
}

func TestGetProjectDetailWithRatings(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	pid, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-1", pid, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}

	// Add ratings for this conversation.
	if _, err := InsertRating(ctx, db, "conv-1", 4, "good", ""); err != nil {
		t.Fatalf("InsertRating 1: %v", err)
	}
	if _, err := InsertRating(ctx, db, "conv-1", 5, "great", ""); err != nil {
		t.Fatalf("InsertRating 2: %v", err)
	}

	detail, err := GetProjectDetail(ctx, db, pid)
	if err != nil {
		t.Fatalf("GetProjectDetail: %v", err)
	}
	if detail == nil {
		t.Fatal("expected non-nil project detail")
	}

	if len(detail.Conversations) != 1 {
		t.Fatalf("got %d conversations, want 1", len(detail.Conversations))
	}

	conv := detail.Conversations[0]
	if len(conv.Ratings) != 2 {
		t.Fatalf("got %d ratings, want 2", len(conv.Ratings))
	}

	// Ratings should be ordered by created_at DESC (newest first).
	if conv.Ratings[0].Note != "great" {
		t.Errorf("first rating note = %q, want %q", conv.Ratings[0].Note, "great")
	}
}

func TestGetProjectDetailPageSortsByLastMessageTimestampAndPaginates(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	pid, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	for _, id := range []string{"conv-old", "conv-new", "conv-none"} {
		if err := EnsureConversation(ctx, db, id, pid, "codex"); err != nil {
			t.Fatalf("EnsureConversation %s: %v", id, err)
		}
	}

	msgs := []Message{
		{Timestamp: 1000, ProjectID: pid, ConversationID: "conv-old", Role: "user", Content: "old", RawJSON: "{}"},
		{Timestamp: 5000, ProjectID: pid, ConversationID: "conv-old", Role: "agent", Content: "old follow-up", RawJSON: "{}"},
		{Timestamp: 3000, ProjectID: pid, ConversationID: "conv-new", Role: "user", Content: "new", RawJSON: "{}"},
	}
	if err := InsertMessages(ctx, db, msgs); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	page1, err := GetProjectDetailPage(ctx, db, pid, 1, 2, ConversationFilters{})
	if err != nil {
		t.Fatalf("GetProjectDetailPage page1: %v", err)
	}
	if len(page1.Conversations) != 2 {
		t.Fatalf("page1 conversations = %d, want 2", len(page1.Conversations))
	}
	if got := page1.Conversations[0].ID; got != "conv-old" {
		t.Fatalf("page1 first conversation = %q, want %q", got, "conv-old")
	}
	if got := page1.Conversations[1].ID; got != "conv-new" {
		t.Fatalf("page1 second conversation = %q, want %q", got, "conv-new")
	}
	if got := page1.ConversationPagination.Total; got != 3 {
		t.Fatalf("page1 total = %d, want 3", got)
	}
	if got := page1.ConversationPagination.TotalPages; got != 2 {
		t.Fatalf("page1 totalPages = %d, want 2", got)
	}

	page2, err := GetProjectDetailPage(ctx, db, pid, 2, 2, ConversationFilters{})
	if err != nil {
		t.Fatalf("GetProjectDetailPage page2: %v", err)
	}
	if len(page2.Conversations) != 1 {
		t.Fatalf("page2 conversations = %d, want 1", len(page2.Conversations))
	}
	if got := page2.Conversations[0].ID; got != "conv-none" {
		t.Fatalf("page2 conversation = %q, want %q", got, "conv-none")
	}
	if got := page2.Conversations[0].LastMessageTimestamp; got != 0 {
		t.Fatalf("conv-none lastMessageTimestamp = %d, want 0", got)
	}
}

func TestGetProjectDetailPageHiddenFilter(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	pid, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	for _, id := range []string{"conv-visible", "conv-hidden"} {
		if err := EnsureConversation(ctx, db, id, pid, "codex"); err != nil {
			t.Fatalf("EnsureConversation %s: %v", id, err)
		}
	}
	if err := SetConversationHidden(ctx, db, "conv-hidden", true); err != nil {
		t.Fatalf("SetConversationHidden: %v", err)
	}

	visible, err := GetProjectDetailPage(ctx, db, pid, 1, 10, ConversationFilters{})
	if err != nil {
		t.Fatalf("GetProjectDetailPage visible: %v", err)
	}
	if len(visible.Conversations) != 1 || visible.Conversations[0].ID != "conv-visible" {
		t.Fatalf("visible conversations = %+v, want only conv-visible", visible.Conversations)
	}
	if len(visible.Agents) != 1 || visible.Agents[0] != "codex" {
		t.Fatalf("visible agents = %+v, want [codex]", visible.Agents)
	}

	hiddenOnly, err := GetProjectDetailPage(ctx, db, pid, 1, 10, ConversationFilters{HiddenOnly: true})
	if err != nil {
		t.Fatalf("GetProjectDetailPage hidden: %v", err)
	}
	if len(hiddenOnly.Conversations) != 1 || hiddenOnly.Conversations[0].ID != "conv-hidden" {
		t.Fatalf("hidden conversations = %+v, want only conv-hidden", hiddenOnly.Conversations)
	}
	if !hiddenOnly.Conversations[0].Hidden {
		t.Fatalf("hidden conversation flag = false, want true")
	}
}

func TestListProjectsReturnsLabel(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	if _, err := EnsureProject(ctx, db, "/home/user/myproject"); err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	projects, err := ListProjects(ctx, db, false)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("got %d projects, want 1", len(projects))
	}
	if projects[0].Label != "myproject" {
		t.Errorf("label = %q, want %q", projects[0].Label, "myproject")
	}
	if projects[0].OldPaths != "" {
		t.Errorf("oldPaths = %q, want empty", projects[0].OldPaths)
	}
	if projects[0].IgnoreDiffPaths != "" {
		t.Errorf("ignoreDiffPaths = %q, want empty", projects[0].IgnoreDiffPaths)
	}
}

func TestSetProjectLabel(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	pid, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	if err := SetProjectLabel(ctx, db, pid, "My Project"); err != nil {
		t.Fatalf("SetProjectLabel: %v", err)
	}

	detail, err := GetProjectDetail(ctx, db, pid)
	if err != nil {
		t.Fatalf("GetProjectDetail: %v", err)
	}
	if detail.Label != "My Project" {
		t.Errorf("label = %q, want %q", detail.Label, "My Project")
	}
}

func TestSetProjectLabelNotFound(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	err := SetProjectLabel(ctx, db, "nonexistent", "label")
	if err == nil {
		t.Fatal("expected error for nonexistent project")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestSetProjectOldPaths(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	pid, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	oldPaths := "/old/path/one\n/old/path/two"
	if err := SetProjectOldPaths(ctx, db, pid, oldPaths); err != nil {
		t.Fatalf("SetProjectOldPaths: %v", err)
	}

	detail, err := GetProjectDetail(ctx, db, pid)
	if err != nil {
		t.Fatalf("GetProjectDetail: %v", err)
	}
	if detail.OldPaths != oldPaths {
		t.Errorf("oldPaths = %q, want %q", detail.OldPaths, oldPaths)
	}
}

func TestSetProjectOldPathsNotFound(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	err := SetProjectOldPaths(ctx, db, "nonexistent", "/old/path")
	if err == nil {
		t.Fatal("expected error for nonexistent project")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestReassignProjectDataByPath(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	targetID, err := EnsureProject(ctx, db, "/new/path")
	if err != nil {
		t.Fatalf("EnsureProject target: %v", err)
	}
	sourceID, err := EnsureProject(ctx, db, "/old/path")
	if err != nil {
		t.Fatalf("EnsureProject source: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-1", sourceID, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}
	if err := InsertMessages(ctx, db, []Message{
		{Timestamp: 1000, ProjectID: sourceID, ConversationID: "conv-1", Role: "user", Content: "hello", RawJSON: "{}"},
	}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}

	moved, err := ReassignProjectDataByPath(ctx, db, targetID, "/old/path")
	if err != nil {
		t.Fatalf("ReassignProjectDataByPath: %v", err)
	}
	if moved != 1 {
		t.Fatalf("moved conversations = %d, want 1", moved)
	}

	detail, err := GetConversationDetail(ctx, db, "conv-1")
	if err != nil {
		t.Fatalf("GetConversationDetail: %v", err)
	}
	if detail.ProjectID != targetID {
		t.Fatalf("conversation project_id = %q, want %q", detail.ProjectID, targetID)
	}

	var msgProjectID string
	if err := db.QueryRow("SELECT project_id FROM messages WHERE conversation_id = ? LIMIT 1", "conv-1").Scan(&msgProjectID); err != nil {
		t.Fatalf("query message project_id: %v", err)
	}
	if msgProjectID != targetID {
		t.Fatalf("message project_id = %q, want %q", msgProjectID, targetID)
	}
}

func TestSetProjectIgnoreDiffPaths(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	pid, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	ignoreDiffPaths := "TODO.md\nAGENTS.md\n**/*.generated.go"
	if err := SetProjectIgnoreDiffPaths(ctx, db, pid, ignoreDiffPaths); err != nil {
		t.Fatalf("SetProjectIgnoreDiffPaths: %v", err)
	}

	detail, err := GetProjectDetail(ctx, db, pid)
	if err != nil {
		t.Fatalf("GetProjectDetail: %v", err)
	}
	if detail.IgnoreDiffPaths != ignoreDiffPaths {
		t.Errorf("ignoreDiffPaths = %q, want %q", detail.IgnoreDiffPaths, ignoreDiffPaths)
	}
}

func TestSetProjectIgnoreDiffPathsNotFound(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	err := SetProjectIgnoreDiffPaths(ctx, db, "nonexistent", "TODO.md")
	if err == nil {
		t.Fatal("expected error for nonexistent project")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdateProjectGitID(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	pid, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	if err := UpdateProjectGitID(ctx, db, pid, "abc123"); err != nil {
		t.Fatalf("UpdateProjectGitID: %v", err)
	}

	detail, err := GetProjectDetail(ctx, db, pid)
	if err != nil {
		t.Fatalf("GetProjectDetail: %v", err)
	}
	if detail.GitID != "abc123" {
		t.Errorf("gitId = %q, want %q", detail.GitID, "abc123")
	}
}

func TestListProjectsWithoutGitID(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	pid1, err := EnsureProject(ctx, db, "/project/a")
	if err != nil {
		t.Fatalf("EnsureProject a: %v", err)
	}
	if _, err := EnsureProject(ctx, db, "/project/b"); err != nil {
		t.Fatalf("EnsureProject b: %v", err)
	}

	// Set git_id on the first project only.
	if err := UpdateProjectGitID(ctx, db, pid1, "abc123"); err != nil {
		t.Fatalf("UpdateProjectGitID: %v", err)
	}

	projects, err := ListProjectsWithoutGitID(ctx, db)
	if err != nil {
		t.Fatalf("ListProjectsWithoutGitID: %v", err)
	}
	if len(projects) != 1 {
		t.Fatalf("got %d projects, want 1", len(projects))
	}
	if projects[0].Path != "/project/b" {
		t.Errorf("path = %q, want %q", projects[0].Path, "/project/b")
	}
}

func TestGetProjectDetailNoConversations(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	pid, err := EnsureProject(ctx, db, "/empty/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}

	detail, err := GetProjectDetail(ctx, db, pid)
	if err != nil {
		t.Fatalf("GetProjectDetail: %v", err)
	}
	if detail == nil {
		t.Fatal("expected non-nil project detail")
	}
	if detail.Conversations == nil {
		t.Error("expected non-nil empty Conversations slice")
	}
	if len(detail.Conversations) != 0 {
		t.Errorf("expected 0 conversations, got %d", len(detail.Conversations))
	}
}

func TestDeleteProject(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	pid, err := EnsureProject(ctx, db, "/test/project")
	if err != nil {
		t.Fatalf("EnsureProject: %v", err)
	}
	if err := EnsureConversation(ctx, db, "conv-1", pid, "claude"); err != nil {
		t.Fatalf("EnsureConversation: %v", err)
	}
	if err := InsertMessages(ctx, db, []Message{
		{Timestamp: 1000, ProjectID: pid, ConversationID: "conv-1", Role: "user", Content: "hello", RawJSON: "{}"},
	}); err != nil {
		t.Fatalf("InsertMessages: %v", err)
	}
	if _, err := InsertRating(ctx, db, "conv-1", 4, "good", ""); err != nil {
		t.Fatalf("InsertRating: %v", err)
	}

	if err := DeleteProject(ctx, db, pid); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	// Verify project is gone.
	detail, err := GetProjectDetail(ctx, db, pid)
	if err != nil {
		t.Fatalf("GetProjectDetail after delete: %v", err)
	}
	if detail != nil {
		t.Error("expected nil project detail after delete")
	}

	// Verify conversations are gone.
	var convCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM conversations WHERE project_id = ?", pid).Scan(&convCount); err != nil {
		t.Fatalf("count conversations: %v", err)
	}
	if convCount != 0 {
		t.Errorf("expected 0 conversations, got %d", convCount)
	}

	// Verify messages are gone.
	var msgCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM messages WHERE project_id = ?", pid).Scan(&msgCount); err != nil {
		t.Fatalf("count messages: %v", err)
	}
	if msgCount != 0 {
		t.Errorf("expected 0 messages, got %d", msgCount)
	}

	// Verify ratings are gone.
	var ratCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM ratings WHERE conversation_id = 'conv-1'").Scan(&ratCount); err != nil {
		t.Fatalf("count ratings: %v", err)
	}
	if ratCount != 0 {
		t.Errorf("expected 0 ratings, got %d", ratCount)
	}
}

func TestDeleteProjectNotFound(t *testing.T) {
	db := setupTestDB(t)
	ctx := context.Background()

	err := DeleteProject(ctx, db, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent project")
	}
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}
