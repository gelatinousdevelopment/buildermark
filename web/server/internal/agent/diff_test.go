package agent

import (
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
