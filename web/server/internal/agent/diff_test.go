package agent

import "testing"

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
