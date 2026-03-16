package cursor

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	tmpDir := t.TempDir()
	database, err := db.InitDB(filepath.Join(tmpDir, "test.db"))
	if err != nil {
		t.Fatalf("init test db: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func countRows(t *testing.T, database *sql.DB, table string) int {
	t.Helper()
	var count int
	err := database.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
	if err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

func TestName(t *testing.T) {
	a := newAgent(nil, "", "")
	if a.Name() != "cursor" {
		t.Errorf("Name() = %q, want %q", a.Name(), "cursor")
	}
}

func TestBubbleRole(t *testing.T) {
	tests := []struct {
		typ  int
		want string
	}{
		{1, "user"},
		{2, "agent"},
		{0, "agent"},
		{99, "agent"},
	}
	for _, tt := range tests {
		got := bubbleRole(tt.typ)
		if got != tt.want {
			t.Errorf("bubbleRole(%d) = %q, want %q", tt.typ, got, tt.want)
		}
	}
}

func TestBubbleTimestamp(t *testing.T) {
	// With timingInfo.
	timing := &bubbleTimingInfo{ClientRpcSendTime: 1700000000000}
	got := bubbleTimestamp(timing, 1600000000000, 5)
	if got != 1700000000000 {
		t.Errorf("bubbleTimestamp with timing = %d, want 1700000000000", got)
	}

	// Without timingInfo — uses fallback.
	got = bubbleTimestamp(nil, 1600000000000, 3)
	if got != 1600000000003 {
		t.Errorf("bubbleTimestamp fallback = %d, want 1600000000003", got)
	}

	// Zero timingInfo — uses fallback.
	zeroTiming := &bubbleTimingInfo{}
	got = bubbleTimestamp(zeroTiming, 1600000000000, 7)
	if got != 1600000000007 {
		t.Errorf("bubbleTimestamp zero timing = %d, want 1600000000007", got)
	}
}

func TestExtractBubbleText(t *testing.T) {
	// Plain text.
	got := extractBubbleText("hello world", nil)
	if got != "hello world" {
		t.Errorf("extractBubbleText plain = %q, want %q", got, "hello world")
	}

	// Empty text with tool data.
	toolData, _ := json.Marshal([]map[string]any{
		{"toolName": "readFile", "args": map[string]any{"path": "/foo"}},
	})
	got = extractBubbleText("", toolData)
	if got != "[tool: readFile]" {
		t.Errorf("extractBubbleText tool = %q, want %q", got, "[tool: readFile]")
	}

	// Empty text, no tools.
	got = extractBubbleText("", nil)
	if got != "" {
		t.Errorf("extractBubbleText empty = %q, want %q", got, "")
	}
}

func TestWorkspaceFolderToPath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"file:///Users/david/project", "/Users/david/project"},
		{"file:///Users/david/my.code-workspace", "/Users/david"},
		{"", ""},
		{"/already/plain", "/already/plain"},
	}
	for _, tt := range tests {
		got := workspaceFolderToPath(tt.input)
		if got != tt.want {
			t.Errorf("workspaceFolderToPath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseComposerData(t *testing.T) {
	// New format with inline conversation.
	newFormat := `{
		"composerId": "abc-123",
		"name": "Test Conversation",
		"createdAt": 1700000000000,
		"lastUpdatedAt": 1700000001000,
		"conversation": [
			{"bubbleId": "b1", "type": 1, "text": "hello"},
			{"bubbleId": "b2", "type": 2, "text": "hi there"}
		]
	}`
	cd, err := parseComposerData([]byte(newFormat))
	if err != nil {
		t.Fatalf("parseComposerData new format: %v", err)
	}
	if cd.ComposerID != "abc-123" {
		t.Errorf("ComposerID = %q, want %q", cd.ComposerID, "abc-123")
	}
	if len(cd.Conversation) != 2 {
		t.Errorf("Conversation length = %d, want 2", len(cd.Conversation))
	}

	// Old format with headers only.
	oldFormat := `{
		"composerId": "def-456",
		"createdAt": 1700000000000,
		"lastUpdatedAt": 1700000001000,
		"fullConversationHeadersOnly": [
			{"bubbleId": "b1", "type": 1},
			{"bubbleId": "b2", "type": 2}
		]
	}`
	cd, err = parseComposerData([]byte(oldFormat))
	if err != nil {
		t.Fatalf("parseComposerData old format: %v", err)
	}
	if cd.ComposerID != "def-456" {
		t.Errorf("ComposerID = %q, want %q", cd.ComposerID, "def-456")
	}
	if len(cd.FullConversationHeadersOnly) != 2 {
		t.Errorf("FullConversationHeadersOnly length = %d, want 2", len(cd.FullConversationHeadersOnly))
	}
}

func TestProcessComposerNewFormat(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()

	// Create a fake Cursor global DB.
	globalDBPath := filepath.Join(tmpDir, "state.vscdb")
	globalDB, err := sql.Open("sqlite3", globalDBPath)
	if err != nil {
		t.Fatalf("open global DB: %v", err)
	}
	_, err = globalDB.Exec("CREATE TABLE cursorDiskKV (key TEXT PRIMARY KEY, value TEXT)")
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	now := time.Now().UnixMilli()
	composerID := "test-composer-001"
	cd := map[string]any{
		"composerId":    composerID,
		"name":          "Fix build errors",
		"createdAt":     now - 5000,
		"lastUpdatedAt": now,
		"conversation": []map[string]any{
			{
				"bubbleId": "b1",
				"type":     1,
				"text":     "Please fix the build errors",
				"timingInfo": map[string]any{
					"clientRpcSendTime": now - 3000,
				},
			},
			{
				"bubbleId": "b2",
				"type":     2,
				"text":     "I'll fix those errors for you.",
				"timingInfo": map[string]any{
					"clientRpcSendTime": now - 2000,
				},
			},
			{
				"bubbleId": "b3",
				"type":     1,
				"text":     "Thanks!",
				"timingInfo": map[string]any{
					"clientRpcSendTime": now - 1000,
				},
			},
		},
	}
	cdJSON, _ := json.Marshal(cd)
	_, err = globalDB.Exec("INSERT INTO cursorDiskKV (key, value) VALUES (?, ?)",
		"composerData:"+composerID, string(cdJSON))
	if err != nil {
		t.Fatalf("insert composer data: %v", err)
	}
	globalDB.Close()

	// Re-open read-only as the watcher would.
	globalDB, err = openCursorDB(globalDBPath)
	if err != nil {
		t.Fatalf("open cursor DB: %v", err)
	}
	defer globalDB.Close()

	a := newAgent(database, tmpDir, tmpDir)

	parsed, err := readComposerData(globalDB, composerID)
	if err != nil {
		t.Fatalf("read composer data: %v", err)
	}

	ctx := context.Background()
	ok := a.processComposer(ctx, globalDB, parsed, "/proj/cursor")

	if !ok {
		t.Fatal("processComposer returned false, want true")
	}
	if n := countRows(t, database, "projects"); n != 1 {
		t.Errorf("projects: got %d, want 1", n)
	}
	if n := countRows(t, database, "conversations"); n != 1 {
		t.Errorf("conversations: got %d, want 1", n)
	}
	if n := countRows(t, database, "messages"); n != 3 {
		t.Errorf("messages: got %d, want 3", n)
	}

	// Verify title was set.
	detail, err := db.GetConversationDetail(ctx, database, composerID)
	if err != nil {
		t.Fatalf("GetConversationDetail: %v", err)
	}
	if detail.Title != "Fix build errors" {
		t.Errorf("title = %q, want %q", detail.Title, "Fix build errors")
	}
}

func TestProcessComposerOldFormat(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()

	globalDBPath := filepath.Join(tmpDir, "state.vscdb")
	globalDB, err := sql.Open("sqlite3", globalDBPath)
	if err != nil {
		t.Fatalf("open global DB: %v", err)
	}
	_, err = globalDB.Exec("CREATE TABLE cursorDiskKV (key TEXT PRIMARY KEY, value TEXT)")
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	now := time.Now().UnixMilli()
	composerID := "old-format-001"

	// Insert composer data (old format with headers only).
	cd := map[string]any{
		"composerId":    composerID,
		"createdAt":     now - 5000,
		"lastUpdatedAt": now,
		"fullConversationHeadersOnly": []map[string]any{
			{"bubbleId": "b1", "type": 1},
			{"bubbleId": "b2", "type": 2},
		},
	}
	cdJSON, _ := json.Marshal(cd)
	_, err = globalDB.Exec("INSERT INTO cursorDiskKV (key, value) VALUES (?, ?)",
		"composerData:"+composerID, string(cdJSON))
	if err != nil {
		t.Fatalf("insert composer data: %v", err)
	}

	// Insert individual bubble data.
	b1 := map[string]any{
		"bubbleId": "b1",
		"type":     1,
		"text":     "What is this code doing?",
		"timingInfo": map[string]any{
			"clientRpcSendTime": now - 3000,
		},
	}
	b1JSON, _ := json.Marshal(b1)
	_, err = globalDB.Exec("INSERT INTO cursorDiskKV (key, value) VALUES (?, ?)",
		"bubbleId:"+composerID+":b1", string(b1JSON))
	if err != nil {
		t.Fatalf("insert bubble b1: %v", err)
	}

	b2 := map[string]any{
		"bubbleId": "b2",
		"type":     2,
		"text":     "This code processes data.",
		"timingInfo": map[string]any{
			"clientRpcSendTime": now - 2000,
		},
	}
	b2JSON, _ := json.Marshal(b2)
	_, err = globalDB.Exec("INSERT INTO cursorDiskKV (key, value) VALUES (?, ?)",
		"bubbleId:"+composerID+":b2", string(b2JSON))
	if err != nil {
		t.Fatalf("insert bubble b2: %v", err)
	}
	globalDB.Close()

	globalDB, err = openCursorDB(globalDBPath)
	if err != nil {
		t.Fatalf("open cursor DB: %v", err)
	}
	defer globalDB.Close()

	a := newAgent(database, tmpDir, tmpDir)

	parsed, err := readComposerData(globalDB, composerID)
	if err != nil {
		t.Fatalf("read composer data: %v", err)
	}

	ctx := context.Background()
	ok := a.processComposer(ctx, globalDB, parsed, "/proj/cursor")

	if !ok {
		t.Fatal("processComposer returned false, want true")
	}
	if n := countRows(t, database, "messages"); n != 2 {
		t.Errorf("messages: got %d, want 2", n)
	}
}

func TestBuildWorkspaceMap(t *testing.T) {
	tmpDir := t.TempDir()
	wsDir := filepath.Join(tmpDir, "workspaceStorage")

	// Create a workspace directory with workspace.json and state.vscdb.
	ws1Dir := filepath.Join(wsDir, "abc123")
	if err := os.MkdirAll(ws1Dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write workspace.json.
	wsJSON, _ := json.Marshal(map[string]string{"folder": "file:///Users/david/myproject"})
	if err := os.WriteFile(filepath.Join(ws1Dir, "workspace.json"), wsJSON, 0644); err != nil {
		t.Fatalf("write workspace.json: %v", err)
	}

	// Create workspace state.vscdb with composer data.
	wsDBPath := filepath.Join(ws1Dir, "state.vscdb")
	wsDB, err := sql.Open("sqlite3", wsDBPath)
	if err != nil {
		t.Fatalf("open ws DB: %v", err)
	}
	_, err = wsDB.Exec("CREATE TABLE ItemTable (key TEXT PRIMARY KEY, value TEXT)")
	if err != nil {
		t.Fatalf("create ItemTable: %v", err)
	}
	composerData, _ := json.Marshal(map[string]any{
		"allComposers": []map[string]any{
			{"composerId": "comp-1", "createdAt": 1700000000000},
			{"composerId": "comp-2", "createdAt": 1700000001000},
		},
	})
	_, err = wsDB.Exec("INSERT INTO ItemTable (key, value) VALUES ('composer.composerData', ?)", string(composerData))
	if err != nil {
		t.Fatalf("insert composer data: %v", err)
	}
	wsDB.Close()

	a := newAgent(nil, tmpDir, tmpDir)
	a.workspaceDir = wsDir
	wsMap := a.buildWorkspaceMap()

	if len(wsMap) != 2 {
		t.Fatalf("workspace map length = %d, want 2", len(wsMap))
	}
	if wsMap["comp-1"] != "/Users/david/myproject" {
		t.Errorf("comp-1 path = %q, want %q", wsMap["comp-1"], "/Users/david/myproject")
	}
	if wsMap["comp-2"] != "/Users/david/myproject" {
		t.Errorf("comp-2 path = %q, want %q", wsMap["comp-2"], "/Users/david/myproject")
	}
}

func TestBuildEnrichedRawJSON_SearchReplace(t *testing.T) {
	rawArgs, _ := json.Marshal(map[string]any{
		"file_path":  "/Users/david/project/main.go",
		"old_string": "func old() {}",
		"new_string": "func new() {}",
	})
	bubble := map[string]any{
		"bubbleId": "b1",
		"type":     2,
		"text":     "",
		"toolFormerData": map[string]any{
			"name":    "search_replace",
			"rawArgs": string(rawArgs),
		},
	}
	raw, _ := json.Marshal(bubble)
	enriched := buildEnrichedRawJSON(string(raw))

	var result map[string]any
	if err := json.Unmarshal([]byte(enriched), &result); err != nil {
		t.Fatalf("unmarshal enriched: %v", err)
	}

	if result["file_path"] != "/Users/david/project/main.go" {
		t.Errorf("file_path = %v, want /Users/david/project/main.go", result["file_path"])
	}
	if result["old_string"] != "func old() {}" {
		t.Errorf("old_string = %v, want func old() {}", result["old_string"])
	}
	if result["new_string"] != "func new() {}" {
		t.Errorf("new_string = %v, want func new() {}", result["new_string"])
	}
}

func TestBuildEnrichedRawJSON_EditFile(t *testing.T) {
	rawArgs, _ := json.Marshal(map[string]any{
		"target_file": "/Users/david/project/main.go",
		"code_edit":   "package main\n\nfunc main() {}\n",
	})
	bubble := map[string]any{
		"bubbleId": "b1",
		"type":     2,
		"text":     "",
		"toolFormerData": map[string]any{
			"name":    "edit_file",
			"rawArgs": string(rawArgs),
		},
	}
	raw, _ := json.Marshal(bubble)
	enriched := buildEnrichedRawJSON(string(raw))

	var result map[string]any
	if err := json.Unmarshal([]byte(enriched), &result); err != nil {
		t.Fatalf("unmarshal enriched: %v", err)
	}

	if result["filePath"] != "/Users/david/project/main.go" {
		t.Errorf("filePath = %v, want /Users/david/project/main.go", result["filePath"])
	}
	if result["content"] != "package main\n\nfunc main() {}\n" {
		t.Errorf("content = %v, want package main...", result["content"])
	}
}

func TestBuildEnrichedRawJSON_NoToolData(t *testing.T) {
	bubble := map[string]any{
		"bubbleId": "b1",
		"type":     1,
		"text":     "hello",
	}
	raw, _ := json.Marshal(bubble)
	enriched := buildEnrichedRawJSON(string(raw))

	// Should be unchanged (no toolFormerData to enrich).
	var orig, result map[string]any
	json.Unmarshal(raw, &orig)
	json.Unmarshal([]byte(enriched), &result)

	if result["file_path"] != nil {
		t.Errorf("unexpected file_path in enriched JSON")
	}
	if result["filePath"] != nil {
		t.Errorf("unexpected filePath in enriched JSON")
	}
}

func TestBuildEnrichedRawJSON_ArrayToolFormerData(t *testing.T) {
	rawArgs, _ := json.Marshal(map[string]any{
		"file_path":  "src/main.go",
		"old_string": "old",
		"new_string": "new",
	})
	bubble := map[string]any{
		"bubbleId": "b1",
		"type":     2,
		"toolFormerData": []map[string]any{
			{
				"name":    "search_replace",
				"rawArgs": string(rawArgs),
			},
		},
	}
	raw, _ := json.Marshal(bubble)
	enriched := buildEnrichedRawJSON(string(raw))

	var result map[string]any
	if err := json.Unmarshal([]byte(enriched), &result); err != nil {
		t.Fatalf("unmarshal enriched: %v", err)
	}

	if result["file_path"] != "src/main.go" {
		t.Errorf("file_path = %v, want src/main.go", result["file_path"])
	}
}

func TestExtractBubbleText_SingleObject(t *testing.T) {
	toolData, _ := json.Marshal(map[string]any{
		"name":    "search_replace",
		"rawArgs": `{"file_path":"main.go"}`,
	})
	got := extractBubbleText("", toolData)
	if got != "[tool: search_replace]" {
		t.Errorf("extractBubbleText single object = %q, want %q", got, "[tool: search_replace]")
	}
}

func TestProcessComposerNewFormatWithDiffs(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()

	globalDBPath := filepath.Join(tmpDir, "state.vscdb")
	globalDB, err := sql.Open("sqlite3", globalDBPath)
	if err != nil {
		t.Fatalf("open global DB: %v", err)
	}
	_, err = globalDB.Exec("CREATE TABLE cursorDiskKV (key TEXT PRIMARY KEY, value TEXT)")
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	now := time.Now().UnixMilli()
	composerID := "test-composer-diffs"

	rawArgs, _ := json.Marshal(map[string]any{
		"file_path":  "src/main.go",
		"old_string": "func hello() {}",
		"new_string": "func hello() {\n\tfmt.Println(\"hello\")\n}",
	})

	cd := map[string]any{
		"composerId":    composerID,
		"name":          "Add hello body",
		"createdAt":     now - 5000,
		"lastUpdatedAt": now,
		"conversation": []map[string]any{
			{
				"bubbleId": "b1",
				"type":     1,
				"text":     "Add a body to hello()",
				"timingInfo": map[string]any{
					"clientRpcSendTime": now - 3000,
				},
			},
			{
				"bubbleId": "b2",
				"type":     2,
				"text":     "",
				"toolFormerData": map[string]any{
					"name":    "search_replace",
					"rawArgs": string(rawArgs),
				},
				"timingInfo": map[string]any{
					"clientRpcSendTime": now - 2000,
				},
			},
		},
	}
	cdJSON, _ := json.Marshal(cd)
	_, err = globalDB.Exec("INSERT INTO cursorDiskKV (key, value) VALUES (?, ?)",
		"composerData:"+composerID, string(cdJSON))
	if err != nil {
		t.Fatalf("insert composer data: %v", err)
	}
	globalDB.Close()

	globalDB, err = openCursorDB(globalDBPath)
	if err != nil {
		t.Fatalf("open cursor DB: %v", err)
	}
	defer globalDB.Close()

	a := newAgent(database, tmpDir, tmpDir)

	parsed, err := readComposerData(globalDB, composerID)
	if err != nil {
		t.Fatalf("read composer data: %v", err)
	}

	ctx := context.Background()
	ok := a.processComposer(ctx, globalDB, parsed, "/proj/cursor")
	if !ok {
		t.Fatal("processComposer returned false, want true")
	}

	// Count diff messages.
	var diffCount int
	err = database.QueryRow(
		"SELECT COUNT(*) FROM messages WHERE conversation_id = ? AND message_type = 'diff'",
		composerID,
	).Scan(&diffCount)
	if err != nil {
		t.Fatalf("count diff messages: %v", err)
	}
	if diffCount == 0 {
		t.Errorf("expected diff messages, got 0")
	}
}

func TestInterpolateTimestamps(t *testing.T) {
	t.Run("all have timing", func(t *testing.T) {
		timings := []*bubbleTimingInfo{
			{ClientRpcSendTime: 1000},
			{ClientRpcSendTime: 2000},
			{ClientRpcSendTime: 3000},
		}
		got := interpolateTimestamps(timings, 500, 4000)
		want := []int64{1000, 2000, 3000}
		for i, v := range got {
			if v != want[i] {
				t.Errorf("ts[%d] = %d, want %d", i, v, want[i])
			}
		}
	})

	t.Run("none have timing", func(t *testing.T) {
		timings := []*bubbleTimingInfo{nil, nil, nil, nil}
		got := interpolateTimestamps(timings, 1000, 5000)
		// Should be evenly spaced between 1000 and 5000.
		// Anchors: (-1, 1000) and (4, 5000), span=5
		// j=0: 1000 + 4000*1/5 = 1800
		// j=1: 1000 + 4000*2/5 = 2600
		// j=2: 1000 + 4000*3/5 = 3400
		// j=3: 1000 + 4000*4/5 = 4200
		for i := 1; i < len(got); i++ {
			if got[i] <= got[i-1] {
				t.Errorf("ts[%d]=%d not > ts[%d]=%d", i, got[i], i-1, got[i-1])
			}
		}
		if got[0] <= 1000 || got[0] >= 5000 {
			t.Errorf("ts[0]=%d should be between 1000 and 5000", got[0])
		}
		if got[3] <= 1000 || got[3] >= 5000 {
			t.Errorf("ts[3]=%d should be between 1000 and 5000", got[3])
		}
	})

	t.Run("mixed known and unknown", func(t *testing.T) {
		timings := []*bubbleTimingInfo{
			nil,
			{ClientRpcSendTime: 3000},
			nil,
			nil,
			{ClientRpcSendTime: 7000},
			nil,
		}
		got := interpolateTimestamps(timings, 1000, 9000)
		// Known anchors should be preserved.
		if got[1] != 3000 {
			t.Errorf("ts[1] = %d, want 3000 (known anchor)", got[1])
		}
		if got[4] != 7000 {
			t.Errorf("ts[4] = %d, want 7000 (known anchor)", got[4])
		}
		// Unknowns should be interpolated between anchors.
		if got[0] <= 1000 || got[0] >= 3000 {
			t.Errorf("ts[0]=%d should be between 1000 and 3000", got[0])
		}
		if got[2] <= 3000 || got[2] >= 7000 {
			t.Errorf("ts[2]=%d should be between 3000 and 7000", got[2])
		}
		if got[5] <= 7000 || got[5] >= 9000 {
			t.Errorf("ts[5]=%d should be between 7000 and 9000", got[5])
		}
		// All strictly increasing.
		for i := 1; i < len(got); i++ {
			if got[i] <= got[i-1] {
				t.Errorf("ts[%d]=%d not > ts[%d]=%d", i, got[i], i-1, got[i-1])
			}
		}
	})

	t.Run("single bubble", func(t *testing.T) {
		timings := []*bubbleTimingInfo{nil}
		got := interpolateTimestamps(timings, 1000, 5000)
		if len(got) != 1 {
			t.Fatalf("len = %d, want 1", len(got))
		}
		if got[0] <= 1000 || got[0] >= 5000 {
			t.Errorf("ts[0]=%d should be between 1000 and 5000", got[0])
		}
	})

	t.Run("degenerate lastUpdatedAt <= createdAt", func(t *testing.T) {
		timings := []*bubbleTimingInfo{nil, nil, nil}
		got := interpolateTimestamps(timings, 5000, 5000)
		// Should fall back to createdAt + index.
		want := []int64{5000, 5001, 5002}
		for i, v := range got {
			if v != want[i] {
				t.Errorf("ts[%d] = %d, want %d", i, v, want[i])
			}
		}
	})

	t.Run("degenerate lastUpdatedAt zero", func(t *testing.T) {
		timings := []*bubbleTimingInfo{nil, nil}
		got := interpolateTimestamps(timings, 5000, 0)
		want := []int64{5000, 5001}
		for i, v := range got {
			if v != want[i] {
				t.Errorf("ts[%d] = %d, want %d", i, v, want[i])
			}
		}
	})

	t.Run("uniqueness enforcement", func(t *testing.T) {
		// Two anchors with the same timestamp — uniqueness pass should fix.
		timings := []*bubbleTimingInfo{
			{ClientRpcSendTime: 5000},
			{ClientRpcSendTime: 5000},
			{ClientRpcSendTime: 5000},
		}
		got := interpolateTimestamps(timings, 1000, 9000)
		for i := 1; i < len(got); i++ {
			if got[i] <= got[i-1] {
				t.Errorf("ts[%d]=%d not > ts[%d]=%d", i, got[i], i-1, got[i-1])
			}
		}
	})

	t.Run("empty", func(t *testing.T) {
		got := interpolateTimestamps(nil, 1000, 5000)
		if got != nil {
			t.Errorf("expected nil for empty timings, got %v", got)
		}
	})
}

func TestProcessComposerNewFormatMixedTimings(t *testing.T) {
	database := setupTestDB(t)
	tmpDir := t.TempDir()

	globalDBPath := filepath.Join(tmpDir, "state.vscdb")
	globalDB, err := sql.Open("sqlite3", globalDBPath)
	if err != nil {
		t.Fatalf("open global DB: %v", err)
	}
	_, err = globalDB.Exec("CREATE TABLE cursorDiskKV (key TEXT PRIMARY KEY, value TEXT)")
	if err != nil {
		t.Fatalf("create table: %v", err)
	}

	createdAt := int64(1700000000000)
	lastUpdatedAt := int64(1700000060000) // 60s conversation
	composerID := "test-mixed-timings"

	cd := map[string]any{
		"composerId":    composerID,
		"name":          "Mixed timings test",
		"createdAt":     createdAt,
		"lastUpdatedAt": lastUpdatedAt,
		"conversation": []map[string]any{
			{
				"bubbleId": "b1",
				"type":     1,
				"text":     "First user message",
				// No timingInfo — should be interpolated.
			},
			{
				"bubbleId": "b2",
				"type":     2,
				"text":     "First assistant response",
				"timingInfo": map[string]any{
					"clientRpcSendTime": createdAt + 15000,
				},
			},
			{
				"bubbleId": "b3",
				"type":     1,
				"text":     "Second user message",
				// No timingInfo — should be interpolated.
			},
			{
				"bubbleId": "b4",
				"type":     2,
				"text":     "Second assistant response",
				"timingInfo": map[string]any{
					"clientRpcSendTime": createdAt + 45000,
				},
			},
		},
	}
	cdJSON, _ := json.Marshal(cd)
	_, err = globalDB.Exec("INSERT INTO cursorDiskKV (key, value) VALUES (?, ?)",
		"composerData:"+composerID, string(cdJSON))
	if err != nil {
		t.Fatalf("insert composer data: %v", err)
	}
	globalDB.Close()

	globalDB, err = openCursorDB(globalDBPath)
	if err != nil {
		t.Fatalf("open cursor DB: %v", err)
	}
	defer globalDB.Close()

	a := newAgent(database, tmpDir, tmpDir)

	parsed, err := readComposerData(globalDB, composerID)
	if err != nil {
		t.Fatalf("read composer data: %v", err)
	}

	ctx := context.Background()
	ok := a.processComposer(ctx, globalDB, parsed, "/proj/cursor")
	if !ok {
		t.Fatal("processComposer returned false, want true")
	}

	// Query message timestamps (exclude diff messages).
	rows, err := database.Query(
		"SELECT timestamp FROM messages WHERE conversation_id = ? AND message_type != 'diff' ORDER BY timestamp",
		composerID,
	)
	if err != nil {
		t.Fatalf("query messages: %v", err)
	}
	defer rows.Close()

	var timestamps []int64
	for rows.Next() {
		var ts int64
		if err := rows.Scan(&ts); err != nil {
			t.Fatalf("scan: %v", err)
		}
		timestamps = append(timestamps, ts)
	}

	if len(timestamps) != 4 {
		t.Fatalf("got %d messages, want 4", len(timestamps))
	}

	// Timestamps should be spread across the conversation, not 1ms apart.
	spread := timestamps[len(timestamps)-1] - timestamps[0]
	if spread < 1000 {
		t.Errorf("timestamp spread = %dms, want >1000ms (got timestamps %v)", spread, timestamps)
	}

	// All strictly increasing.
	for i := 1; i < len(timestamps); i++ {
		if timestamps[i] <= timestamps[i-1] {
			t.Errorf("ts[%d]=%d not > ts[%d]=%d", i, timestamps[i], i-1, timestamps[i-1])
		}
	}

	// Known anchors should be preserved.
	if timestamps[1] != createdAt+15000 {
		t.Errorf("ts[1] = %d, want %d (known anchor)", timestamps[1], createdAt+15000)
	}
	if timestamps[3] != createdAt+45000 {
		t.Errorf("ts[3] = %d, want %d (known anchor)", timestamps[3], createdAt+45000)
	}
}

func TestCommonPathPrefix(t *testing.T) {
	tests := []struct {
		a, b, want string
	}{
		{"/Users/david/project/src", "/Users/david/project/lib", "/Users/david/project"},
		{"/a/b/c", "/a/b/d", "/a/b"},
		{"/a/b", "/c/d", ""},
		{"/same/path", "/same/path", "/same/path"},
	}
	for _, tt := range tests {
		got := commonPathPrefix(tt.a, tt.b)
		if got != tt.want {
			t.Errorf("commonPathPrefix(%q, %q) = %q, want %q", tt.a, tt.b, got, tt.want)
		}
	}
}
