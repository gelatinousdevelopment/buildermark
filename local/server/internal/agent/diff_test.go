package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtractReliableDiffFromFencedBlock(t *testing.T) {
	input := "Done.\n```diff\n--- a/x.txt\n+++ b/x.txt\n@@ -1 +1 @@\n-old\n+new\n```\n"
	diff, ok := ExtractReliableDiff(input)
	if !ok {
		t.Fatal("expected fenced diff to be extracted")
	}
	if diff == "" {
		t.Fatal("expected non-empty diff")
	}
}

func TestExtractReliableDiffRejectsNonDiffFence(t *testing.T) {
	input := "```txt\n-old\n+new\n```"
	if _, ok := ExtractReliableDiff(input); ok {
		t.Fatal("expected non-diff fenced content to be rejected")
	}
}

func TestExtractReliableDiffRejectsAmbiguousText(t *testing.T) {
	input := "I can make changes:\n- remove this\n+ add this\nThanks."
	if _, ok := ExtractReliableDiff(input); ok {
		t.Fatal("expected ambiguous prose to be rejected")
	}
}

func TestExtractReliableDiffFromCreatedFileLogWithEmbeddedDiffExamples(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	filePath := filepath.Join(repo, "local", "frontend", "src", "lib", "diffmerge.test.ts")
	input := strings.Join([]string{
		"File created successfully at: " + filePath,
		`import { describe, it, expect } from "vitest";`,
		`function fenced(raw: string): string {`,
		"  return \"```diff\\n\" + raw + \"\\n```\";",
		"}",
		"`diff --git a/example.go b/example.go",
		"--- a/example.go",
		"+++ b/example.go",
		"@@ -1 +1 @@",
		"-old",
		"+new`",
	}, "\n")

	diff, ok := ExtractReliableDiff(input)
	if !ok || diff == "" {
		t.Fatal("expected created-file log to produce a diff")
	}
	if !strings.Contains(diff, "diff --git a/local/frontend/src/lib/diffmerge.test.ts b/local/frontend/src/lib/diffmerge.test.ts") {
		t.Fatalf("expected created file path in diff, got: %q", diff)
	}
	got := strings.Count(diff, "\ndiff --git ")
	if strings.HasPrefix(diff, "diff --git ") {
		got++
	}
	if got != 1 {
		t.Fatalf("expected exactly one real diff header, got %d in %q", got, diff)
	}
	if strings.Contains(diff, "\n+++ b/example.go\n") {
		t.Fatalf("expected embedded example diff to stay plain file content, got: %q", diff)
	}
}

func TestExtractReliableDiffRejectsSourceSnapshotWithEmbeddedDiffExamples(t *testing.T) {
	input := strings.Join([]string{
		`import { describe, it, expect } from "vitest";`,
		`function fenced(raw: string): string {`,
		"  return \"```diff\\n\" + raw + \"\\n```\";",
		"}",
		"`diff --git a/example.go b/example.go",
		"--- a/example.go",
		"+++ b/example.go",
		"@@ -1 +1 @@",
		"-old",
		"+new`",
	}, "\n")

	if _, ok := ExtractReliableDiff(input); ok {
		t.Fatal("expected source snapshot with embedded diff examples to be rejected")
	}
}

func TestExtractReliableDiffFromJSON(t *testing.T) {
	raw := `{"type":"response_item","payload":{"type":"function_call_output","output":"diff --git a/x.txt b/x.txt\n--- a/x.txt\n+++ b/x.txt\n@@ -1 +1 @@\n-old\n+new\n"}}`
	diff, ok := ExtractReliableDiffFromJSON(raw)
	if !ok {
		t.Fatal("expected diff from JSON string field")
	}
	if diff == "" {
		t.Fatal("expected non-empty diff from JSON")
	}
}

func TestExtractReliableDiffFromNestedJSONString(t *testing.T) {
	raw := `{"payload":{"output":"{\"resultDisplay\":\"diff --git a/x.txt b/x.txt\n--- a/x.txt\n+++ b/x.txt\n@@ -1 +1 @@\n-old\n+new\n\"}"}}`
	diff, ok := ExtractReliableDiffFromJSON(raw)
	if !ok || diff == "" {
		t.Fatal("expected diff from nested JSON string field")
	}
}

func TestExtractReliableDiffFromApplyPatch(t *testing.T) {
	input := "*** Begin Patch\n*** Update File: x.txt\n@@\n-old\n+new\n*** End Patch\n"
	diff, ok := ExtractReliableDiff(input)
	if !ok {
		t.Fatal("expected apply_patch content to be extracted")
	}
	if diff == "" || !strings.Contains(diff, "--- a/x.txt") || !strings.Contains(diff, "+++ b/x.txt") {
		t.Fatalf("unexpected apply_patch diff output: %q", diff)
	}
}

func TestExtractReliableDiffFromCustomToolApplyPatchJSON(t *testing.T) {
	raw := `{"type":"response_item","payload":{"type":"custom_tool_call","name":"apply_patch","input":"*** Begin Patch\n*** Update File: x.txt\n@@\n-old\n+new\n*** End Patch\n"}}`
	diff, ok := ExtractReliableDiffFromJSON(raw)
	if !ok || diff == "" {
		t.Fatal("expected diff from custom_tool_call apply_patch JSON")
	}
	if !strings.Contains(diff, "diff --git a/x.txt b/x.txt") {
		t.Fatalf("expected normalized diff header, got: %q", diff)
	}
}

func TestExtractReliableDiffFromCustomToolApplyPatchJSONWithAbsolutePathUsesCWD(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	filePath := filepath.Join(repo, "local", "server", "internal", "db", "projects.go")
	raw := fmt.Sprintf(`{
		"cwd":%q,
		"type":"response_item",
		"payload":{
			"type":"custom_tool_call",
			"name":"apply_patch",
			"input":"*** Begin Patch\n*** Update File: %s\n@@\n-old\n+new\n*** End Patch\n"
		}
	}`, repo, strings.ReplaceAll(filePath, `\`, `\\`))
	diff, ok := ExtractReliableDiffFromJSON(raw)
	if !ok || diff == "" {
		t.Fatal("expected diff from custom_tool_call apply_patch JSON with absolute path")
	}
	if !strings.Contains(diff, "diff --git a/local/server/internal/db/projects.go b/local/server/internal/db/projects.go") {
		t.Fatalf("expected repo-relative diff header, got: %q", diff)
	}
	if strings.Contains(diff, "diff --git a//") {
		t.Fatalf("unexpected absolute-path diff header, got: %q", diff)
	}
}

func TestExtractReliableDiffFromApplyPatchMultipleSections(t *testing.T) {
	input := "*** Begin Patch\n*** Update File: a.txt\n@@\n-old-a\n+new-a\n*** Update File: b.txt\n@@\n-old-b\n+new-b\n*** End Patch\n"
	diff, ok := ExtractReliableDiff(input)
	if !ok || diff == "" {
		t.Fatal("expected multi-section apply_patch content to be extracted")
	}
	if !strings.Contains(diff, "diff --git a/a.txt b/a.txt") {
		t.Fatalf("expected first file header, got: %q", diff)
	}
	if !strings.Contains(diff, "diff --git a/b.txt b/b.txt") {
		t.Fatalf("expected second file header, got: %q", diff)
	}
}

func TestExtractReliableDiffFromApplyPatchMoveTo(t *testing.T) {
	input := "*** Begin Patch\n*** Update File: old.txt\n*** Move to: new.txt\n@@\n-old\n+new\n*** End Patch\n"
	diff, ok := ExtractReliableDiff(input)
	if !ok || diff == "" {
		t.Fatal("expected moved-file apply_patch content to be extracted")
	}
	if !strings.Contains(diff, "diff --git a/old.txt b/new.txt") {
		t.Fatalf("expected move diff header, got: %q", diff)
	}
	if !strings.Contains(diff, "--- a/old.txt") || !strings.Contains(diff, "+++ b/new.txt") {
		t.Fatalf("expected moved file paths, got: %q", diff)
	}
}

func TestExtractReliableDiffFromApplyPatchAddFile(t *testing.T) {
	input := "*** Begin Patch\n*** Add File: add.txt\n+hello\n*** End Patch\n"
	diff, ok := ExtractReliableDiff(input)
	if !ok || diff == "" {
		t.Fatal("expected add-file apply_patch content to be extracted")
	}
	if !strings.Contains(diff, "--- /dev/null") || !strings.Contains(diff, "+++ b/add.txt") {
		t.Fatalf("expected add-file markers, got: %q", diff)
	}
}

func TestExtractReliableDiffRejectsApplyPatchWithoutHunkLines(t *testing.T) {
	input := "*** Begin Patch\n*** Update File: x.txt\n*** End Patch\n"
	if _, ok := ExtractReliableDiff(input); ok {
		t.Fatal("expected apply_patch without changes to be rejected")
	}
}

func TestExtractReliableDiffFromShellHeredocWrite(t *testing.T) {
	input := "mkdir -p src && cat > 'src/a.txt' <<'EOF'\nline1\nline2\nEOF\n"
	diff, ok := ExtractReliableDiff(input)
	if !ok || diff == "" {
		t.Fatal("expected heredoc file-write command to be extracted")
	}
	if !strings.Contains(diff, "diff --git a/src/a.txt b/src/a.txt") {
		t.Fatalf("expected diff header for heredoc write, got: %q", diff)
	}
	if !strings.Contains(diff, "\n+line1\n+line2") {
		t.Fatalf("expected heredoc body lines in diff, got: %q", diff)
	}
}

func TestExtractReliableDiffFromJSONFunctionCallCmdHeredoc(t *testing.T) {
	raw := `{"type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{\"cmd\":\"cat > 'src/a.txt' <<'EOF'\\nalpha\\nbeta\\nEOF\\n\"}"}}`
	diff, ok := ExtractReliableDiffFromJSON(raw)
	if !ok || diff == "" {
		t.Fatal("expected heredoc diff from nested JSON command")
	}
	if !strings.Contains(diff, "diff --git a/src/a.txt b/src/a.txt") {
		t.Fatalf("expected diff header in heredoc JSON extraction, got: %q", diff)
	}
}

func TestExtractReliableDiffFromStructuredPatchJSON(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	cwd := filepath.Join(repo, "web", "frontend")
	filePath := filepath.Join(cwd, "src", "app.ts")
	raw := fmt.Sprintf(`{
		"cwd":%q,
		"toolUseResult":{
			"filePath":%q,
			"structuredPatch":[
				{"oldStart":10,"oldLines":3,"newStart":10,"newLines":3,"lines":[" line1","-old"," +badprefix","+new"]}
			]
		}
	}`, cwd, filePath)
	diff, ok := ExtractReliableDiffFromJSON(raw)
	if !ok || diff == "" {
		t.Fatal("expected diff from structuredPatch JSON")
	}
	if !strings.Contains(diff, "diff --git a/web/frontend/src/app.ts b/web/frontend/src/app.ts") {
		t.Fatalf("expected repo-relative path variant in diff, got: %q", diff)
	}
	if got := strings.Count(diff, "diff --git "); got != 1 {
		t.Fatalf("expected exactly one diff header, got %d in %q", got, diff)
	}
	if !strings.Contains(diff, "@@ -10,3 +10,3 @@") {
		t.Fatalf("expected hunk header from structuredPatch metadata, got: %q", diff)
	}
	if !strings.Contains(diff, "\n-old\n") || !strings.Contains(diff, "\n+new") {
		t.Fatalf("expected added and removed lines from structuredPatch, got: %q", diff)
	}
}

func TestExtractReliableDiffFromJSONFileSnapshotReadOnlyRejected(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	cwd := filepath.Join(repo, "web", "server")
	filePath := filepath.Join(repo, "web", "frontend", "src", "app.ts")
	raw := fmt.Sprintf(`{
		"cwd":%q,
		"toolUseResult":{
			"file":{
				"filePath":%q,
				"content":"line1\nline2\n",
				"numLines":2,
				"startLine":1,
				"totalLines":2
			}
		}
	}`, cwd, filePath)

	diff, ok := ExtractReliableDiffFromJSON(raw)
	if ok || diff != "" {
		t.Fatalf("expected read-style snapshot to be rejected, got ok=%v diff=%q", ok, diff)
	}
}

func TestExtractReliableDiffFromJSONCatNumberedSnapshot(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	cwd := filepath.Join(repo, "web", "server")
	filePath := filepath.Join(repo, "web", "frontend", "src", "app.ts")
	raw := fmt.Sprintf(`{
		"cwd":%q,
		"toolUseResult":{
			"filePath":%q,
			"content":"     1→alpha\n     2→beta\n     3→"
		}
	}`, cwd, filePath)

	diff, ok := ExtractReliableDiffFromJSON(raw)
	if !ok || diff == "" {
		t.Fatal("expected diff from cat-numbered JSON snapshot")
	}
	if strings.Contains(diff, "1→alpha") || strings.Contains(diff, "2→beta") {
		t.Fatalf("expected cat-number prefixes to be stripped, got: %q", diff)
	}
	if !strings.Contains(diff, "\n+alpha\n+beta") {
		t.Fatalf("expected stripped snapshot lines in diff, got: %q", diff)
	}
}

func TestExtractReliableDiffFromJSONPrefersRealDiffOverSnapshot(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	cwd := filepath.Join(repo, "web", "server")
	filePath := filepath.Join(repo, "web", "frontend", "src", "app.ts")
	raw := fmt.Sprintf(`{
		"cwd":%q,
		"payload":{"output":"diff --git a/x.txt b/x.txt\n--- a/x.txt\n+++ b/x.txt\n@@ -1 +1 @@\n-old\n+new\n"},
		"toolUseResult":{
			"file":{"filePath":%q,"content":"line1\nline2\nline3","numLines":3}
		}
	}`, cwd, filePath)

	diff, ok := ExtractReliableDiffFromJSON(raw)
	if !ok || diff == "" {
		t.Fatal("expected diff from JSON")
	}
	if !strings.Contains(diff, "diff --git a/x.txt b/x.txt") {
		t.Fatalf("expected real unified diff to be preferred, got: %q", diff)
	}
	if strings.Contains(diff, "+++ b/local/frontend/src/app.ts") {
		t.Fatalf("unexpected fallback snapshot diff selected: %q", diff)
	}
}

func TestExtractReliableDiffFromJSONSkipsToolUseBlock(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	cwd := repo
	filePath := filepath.Join(repo, "web", "frontend", "src", "lib", "messageUtils.ts")
	raw := fmt.Sprintf(`{
		"cwd":%q,
		"message":{
			"content":[
				{"type":"tool_use","name":"Edit","input":{
					"file_path":%q,
					"old_string":"export function renderMarkdown(content: string): string {\n\treturn marked.parse(content, { gfm: true, breaks: true }) as string;\n}",
					"new_string":"marked.use({\n\trenderer: {\n\t\thtml(token) {\n\t\t\treturn escapeHtml(token.text);\n\t\t}\n\t}\n});\n\nexport function renderMarkdown(content: string): string {\n\treturn marked.parse(content, { gfm: true, breaks: true }) as string;\n}"
				}}
			]
		}
	}`, cwd, filePath)

	diff, ok := ExtractReliableDiffFromJSON(raw)
	if ok || diff != "" {
		t.Fatalf("expected tool_use block to be skipped, got ok=%v diff=%q", ok, diff)
	}
}

func TestExtractReliableDiffFromJSONToolUseResultNotSkipped(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	cwd := repo
	filePath := filepath.Join(repo, "web", "frontend", "src", "lib", "messageUtils.ts")
	raw := fmt.Sprintf(`{
		"cwd":%q,
		"toolUseResult":{
			"filePath":%q,
			"oldString":"export function renderMarkdown(content: string): string {\n\treturn marked.parse(content, { gfm: true, breaks: true }) as string;\n}",
			"newString":"marked.use({\n\trenderer: {\n\t\thtml(token) {\n\t\t\treturn escapeHtml(token.text);\n\t\t}\n\t}\n});\n\nexport function renderMarkdown(content: string): string {\n\treturn marked.parse(content, { gfm: true, breaks: true }) as string;\n}"
		}
	}`, cwd, filePath)

	diff, ok := ExtractReliableDiffFromJSON(raw)
	if !ok || diff == "" {
		t.Fatal("expected diff from toolUseResult old/new string payload")
	}
	if !strings.Contains(diff, "diff --git a/web/frontend/src/lib/messageUtils.ts b/web/frontend/src/lib/messageUtils.ts") {
		t.Fatalf("missing expected file path in extracted diff: %q", diff)
	}
	if !strings.Contains(diff, "+marked.use({") {
		t.Fatalf("missing expected added content in extracted diff: %q", diff)
	}
}

func TestExtractReliableDiffFromJSONStructuredPatchPriorityOverOldNew(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	cwd := repo
	filePath := filepath.Join(repo, "web", "frontend", "src", "app.ts")
	// JSON with both structuredPatch and oldString/newString at the toolUseResult level.
	// structuredPatch has correct line offsets; oldNewEdit would produce @@ -1.
	raw := fmt.Sprintf(`{
		"cwd":%q,
		"toolUseResult":{
			"filePath":%q,
			"oldString":"old line\n",
			"newString":"new line\nand another\n",
			"structuredPatch":[
				{"oldStart":42,"oldLines":3,"newStart":42,"newLines":4,"lines":[" ctx","-old line","+new line","+and another"," more"]}
			]
		}
	}`, cwd, filePath)

	diff, ok := ExtractReliableDiffFromJSON(raw)
	if !ok || diff == "" {
		t.Fatal("expected diff when both structuredPatch and oldNewEdit are present")
	}
	if !strings.Contains(diff, "@@ -42,3 +42,4 @@") {
		t.Fatalf("expected structuredPatch line offsets to win, got: %q", diff)
	}
	if strings.Contains(diff, "@@ -1,") {
		t.Fatalf("oldNewEdit @@ -1 offset should not appear, got: %q", diff)
	}
}

func TestExtractReliableDiffsFromJSONReturnsMultipleDiffs(t *testing.T) {
	raw := `{
		"one":"diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-old-a\n+new-a\n",
		"two":"diff --git a/b.txt b/b.txt\n--- a/b.txt\n+++ b/b.txt\n@@ -1 +1 @@\n-old-b\n+new-b\n"
	}`
	diffs := ExtractReliableDiffsFromJSON(raw)
	if got := len(diffs); got != 2 {
		t.Fatalf("len(diffs) = %d, want 2", got)
	}
	joined := strings.Join(diffs, "\n")
	if !strings.Contains(joined, "a/a.txt") || !strings.Contains(joined, "b/b.txt") {
		t.Fatalf("expected both diffs in output, got: %q", joined)
	}
}

func TestExtractReliableDiffFromJSONAbsolutePathWithoutCWDUsesGitRoot(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	filePath := filepath.Join(repo, "src", "main.go")
	raw := fmt.Sprintf(`{
		"toolUseResult":{
			"filePath":%q,
			"structuredPatch":[
				{"oldStart":1,"oldLines":1,"newStart":1,"newLines":1,"lines":["-old","+new"]}
			]
		}
	}`, filePath)
	diff, ok := ExtractReliableDiffFromJSON(raw)
	if !ok || diff == "" {
		t.Fatal("expected diff from structuredPatch payload")
	}
	if !strings.Contains(diff, "diff --git a/src/main.go b/src/main.go") {
		t.Fatalf("expected git-root relative path, got: %q", diff)
	}
	if strings.Contains(diff, "diff --git a/"+strings.TrimPrefix(strings.ReplaceAll(filePath, "\\", "/"), "/")) {
		t.Fatalf("unexpected absolute-path fallback selected first: %q", diff)
	}
}
