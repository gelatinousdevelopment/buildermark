package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

func TestFileTypeCoverage(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	projectID := "test-project"
	if _, err := s.DB.Exec("INSERT INTO projects (id, path, label) VALUES (?, ?, ?)", projectID, "/tmp/test", "test"); err != nil {
		t.Fatalf("create project: %v", err)
	}

	// Insert commits with detail_files covering multiple extensions and overlapping paths.
	commit1Files := []commitFileCoverage{
		{
			Path:       "src/main.go",
			LinesTotal: 100,
			AgentSegments: []agentCoverageSegment{
				{Agent: "claude", LinesFromAgent: 60, LinePercent: 60},
			},
		},
		{
			Path:       "src/utils.go",
			LinesTotal: 50,
			AgentSegments: []agentCoverageSegment{
				{Agent: "claude", LinesFromAgent: 30, LinePercent: 60},
			},
		},
		{
			Path:       "README.md",
			LinesTotal: 20,
			AgentSegments: []agentCoverageSegment{
				{Agent: "copilot", LinesFromAgent: 10, LinePercent: 50},
			},
		},
		{
			Path:       "Makefile",
			LinesTotal: 15,
			AgentSegments: []agentCoverageSegment{
				{Agent: "claude", LinesFromAgent: 5, LinePercent: 33.33},
			},
		},
	}

	commit2Files := []commitFileCoverage{
		{
			Path:       "src/main.go", // overlapping path
			LinesTotal: 80,
			AgentSegments: []agentCoverageSegment{
				{Agent: "claude", LinesFromAgent: 40, LinePercent: 50},
				{Agent: "copilot", LinesFromAgent: 20, LinePercent: 25},
			},
		},
		{
			Path:       "src/handler.go",
			LinesTotal: 60,
			AgentSegments: []agentCoverageSegment{
				{Agent: "claude", LinesFromAgent: 45, LinePercent: 75},
			},
		},
		{
			Path:    "vendor/lib.go",
			Ignored: true, // should be excluded
		},
	}

	// Commit with empty detail_files (should be excluded by DB query).
	commit3Files := []commitFileCoverage{}

	files1, _ := json.Marshal(commit1Files)
	files2, _ := json.Marshal(commit2Files)
	files3, _ := json.Marshal(commit3Files)

	commits := []db.Commit{
		{
			ID:          "c1",
			ProjectID:   projectID,
			BranchName:  "main",
			CommitHash:  "aaa",
			AuthoredAt:  1000,
			LinesTotal:  185,
			DetailFiles: string(files1),
		},
		{
			ID:          "c2",
			ProjectID:   projectID,
			BranchName:  "main",
			CommitHash:  "bbb",
			AuthoredAt:  2000,
			LinesTotal:  140,
			DetailFiles: string(files2),
		},
		{
			ID:          "c3",
			ProjectID:   projectID,
			BranchName:  "main",
			CommitHash:  "ccc",
			AuthoredAt:  3000,
			LinesTotal:  0,
			DetailFiles: string(files3),
		},
	}

	for _, c := range commits {
		if err := db.UpsertCommit(context.Background(), s.DB, c); err != nil {
			t.Fatalf("upsert commit %s: %v", c.CommitHash, err)
		}
	}

	// Query the endpoint. Times are in ms, authored_at is in seconds.
	req := httptest.NewRequest("GET", "/api/v1/projects/"+projectID+"/file-type-coverage?start=500000&end=3500000", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}

	var env struct {
		OK   bool                  `json:"ok"`
		Data []fileTypeCoverageRow `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !env.OK {
		t.Fatalf("response not ok")
	}

	// .go: 3 unique files (main.go, utils.go, handler.go), totalLines = 100+50+80+60=290
	// .md: 1 file, totalLines = 20
	// Makefile: 1 file, totalLines = 15
	// Sorted by totalFiles desc: .go (3), .md (1), Makefile (1)
	if len(env.Data) != 3 {
		t.Fatalf("expected 3 extensions, got %d: %+v", len(env.Data), env.Data)
	}

	goRow := env.Data[0]
	if goRow.Extension != ".go" {
		t.Errorf("first row extension = %q, want .go", goRow.Extension)
	}
	if goRow.TotalFiles != 3 {
		t.Errorf(".go totalFiles = %d, want 3", goRow.TotalFiles)
	}
	if goRow.TotalLines != 290 {
		t.Errorf(".go totalLines = %d, want 290", goRow.TotalLines)
	}

	// Check agent segments for .go: claude = 60+30+40+45=175, copilot = 20
	agentMap := make(map[string]int)
	for _, seg := range goRow.AgentSegments {
		agentMap[seg.Agent] = seg.LinesFromAgent
	}
	if agentMap["claude"] != 175 {
		t.Errorf(".go claude lines = %d, want 175", agentMap["claude"])
	}
	if agentMap["copilot"] != 20 {
		t.Errorf(".go copilot lines = %d, want 20", agentMap["copilot"])
	}

	// Manual for .go: 290 - 175 - 20 = 95, manualPercent = 95/290*100
	expectedManualPct := float64(95) / float64(290) * 100
	if diff := goRow.ManualPercent - expectedManualPct; diff > 0.1 || diff < -0.1 {
		t.Errorf(".go manualPercent = %f, want ~%f", goRow.ManualPercent, expectedManualPct)
	}

	// Makefile (no extension) should use basename.
	var makefileRow *fileTypeCoverageRow
	for i := range env.Data {
		if env.Data[i].Extension == "Makefile" {
			makefileRow = &env.Data[i]
			break
		}
	}
	if makefileRow == nil {
		t.Fatal("missing Makefile row")
	}
	if makefileRow.TotalFiles != 1 {
		t.Errorf("Makefile totalFiles = %d, want 1", makefileRow.TotalFiles)
	}
	if makefileRow.TotalLines != 15 {
		t.Errorf("Makefile totalLines = %d, want 15", makefileRow.TotalLines)
	}
}

func TestFileTypeCoverageAttributableLines(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	projectID := "attr-project"
	if _, err := s.DB.Exec("INSERT INTO projects (id, path, label) VALUES (?, ?, ?)", projectID, "/tmp/attr", "attr"); err != nil {
		t.Fatalf("create project: %v", err)
	}

	// Files where AttributableLines < LinesTotal (non-attributable lines like {, }, etc.)
	files := []commitFileCoverage{
		{
			Path:              "src/main.go",
			LinesTotal:        200, // raw added+removed
			AttributableLines: 150, // only lines with letters/digits
			AgentSegments: []agentCoverageSegment{
				{Agent: "claude", LinesFromAgent: 120, LinePercent: 80},
			},
		},
		{
			Path:              "src/app.ts",
			LinesTotal:        100,
			AttributableLines: 80,
			AgentSegments: []agentCoverageSegment{
				{Agent: "claude", LinesFromAgent: 60, LinePercent: 75},
			},
		},
	}
	filesJSON, _ := json.Marshal(files)

	c := db.Commit{
		ID:          "attr-c1",
		ProjectID:   projectID,
		BranchName:  "main",
		CommitHash:  "attr-aaa",
		AuthoredAt:  1000,
		LinesTotal:  230, // = 150 + 80 attributable
		DetailFiles: string(filesJSON),
	}
	if err := db.UpsertCommit(context.Background(), s.DB, c); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/projects/"+projectID+"/file-type-coverage?start=500000&end=1500000", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", rec.Code, rec.Body.String())
	}

	var env struct {
		OK   bool                  `json:"ok"`
		Data []fileTypeCoverageRow `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(env.Data) != 2 {
		t.Fatalf("expected 2 extensions, got %d", len(env.Data))
	}

	for _, row := range env.Data {
		switch row.Extension {
		case ".go":
			// Should use AttributableLines (150) not LinesTotal (200)
			if row.TotalLines != 150 {
				t.Errorf(".go totalLines = %d, want 150 (attributable)", row.TotalLines)
			}
			// claude: 120/150 = 80%
			if len(row.AgentSegments) != 1 || row.AgentSegments[0].LinesFromAgent != 120 {
				t.Errorf(".go claude lines unexpected: %+v", row.AgentSegments)
			}
			expectedManual := (1 - 120.0/150.0) * 100
			if diff := row.ManualPercent - expectedManual; diff > 0.1 || diff < -0.1 {
				t.Errorf(".go manualPercent = %f, want ~%f", row.ManualPercent, expectedManual)
			}
		case ".ts":
			// Should use AttributableLines (80) not LinesTotal (100)
			if row.TotalLines != 80 {
				t.Errorf(".ts totalLines = %d, want 80 (attributable)", row.TotalLines)
			}
		}
	}
}

func TestFileTypeCoverageEmpty(t *testing.T) {
	s := setupTestServer(t)
	handler := s.Routes()

	projectID := "empty-project"
	if _, err := s.DB.Exec("INSERT INTO projects (id, path, label) VALUES (?, ?, ?)", projectID, "/tmp/empty", "empty"); err != nil {
		t.Fatalf("create project: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/projects/"+projectID+"/file-type-coverage?start=1000&end=2000", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var env struct {
		OK   bool                  `json:"ok"`
		Data []fileTypeCoverageRow `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(env.Data) != 0 {
		t.Errorf("expected empty data, got %d rows", len(env.Data))
	}
}
