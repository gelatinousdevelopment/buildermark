package agent

import (
	"strings"
	"testing"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
)

func TestAppendDiffDBMessagesWithOptionsUseAllJSONDiffsAndDeduplicate(t *testing.T) {
	raw := `{
		"one":"diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-old-a\n+new-a\n",
		"two":"diff --git a/b.txt b/b.txt\n--- a/b.txt\n+++ b/b.txt\n@@ -1 +1 @@\n-old-b\n+new-b\n",
		"dup":"diff --git a/a.txt b/a.txt\n--- a/a.txt\n+++ b/a.txt\n@@ -1 +1 @@\n-old-a\n+new-a\n"
	}`
	in := []db.Message{
		{
			Timestamp:      1000,
			ProjectID:      "p1",
			ConversationID: "c1",
			Role:           "agent",
			Content:        "[response_item]",
			RawJSON:        raw,
		},
	}

	out := AppendDiffDBMessagesWithOptions(in, DiffAppendOptions{
		Deduplicate:     true,
		UseAllJSONDiffs: true,
	})
	if got := len(out); got != 3 {
		t.Fatalf("len(out) = %d, want 3 (1 source + 2 unique derived)", got)
	}
	if out[1].RawJSON != DerivedDiffRawJSON || out[2].RawJSON != DerivedDiffRawJSON {
		t.Fatalf("expected derived diff raw json marker, got %q and %q", out[1].RawJSON, out[2].RawJSON)
	}
	if out[1].MessageType != db.MessageTypeDiff || out[2].MessageType != db.MessageTypeDiff {
		t.Fatalf("expected derived diff message_type %q, got %q and %q", db.MessageTypeDiff, out[1].MessageType, out[2].MessageType)
	}
	if !strings.Contains(out[1].Content+out[2].Content, "a/a.txt") {
		t.Fatalf("expected derived diff for a.txt in output: %q", out[1].Content+out[2].Content)
	}
	if !strings.Contains(out[1].Content+out[2].Content, "b/b.txt") {
		t.Fatalf("expected derived diff for b.txt in output: %q", out[1].Content+out[2].Content)
	}
}

func TestAppendDiffDBMessagesLegacyBehaviorUnchanged(t *testing.T) {
	contentDiff := "```diff\n--- a/content.txt\n+++ b/content.txt\n@@ -1 +1 @@\n-old\n+new\n```"
	raw := `{"payload":{"output":"diff --git a/json.txt b/json.txt\n--- a/json.txt\n+++ b/json.txt\n@@ -1 +1 @@\n-old\n+new\n"}}`
	in := []db.Message{
		{
			Timestamp:      2000,
			ProjectID:      "p1",
			ConversationID: "c1",
			Role:           "agent",
			Content:        contentDiff,
			RawJSON:        raw,
		},
	}

	out := AppendDiffDBMessages(in)
	if got := len(out); got != 2 {
		t.Fatalf("len(out) = %d, want 2 (legacy single derived diff)", got)
	}
	if !strings.Contains(out[1].Content, "content.txt") {
		t.Fatalf("legacy mode should prefer content diff, got: %q", out[1].Content)
	}
	if out[1].MessageType != db.MessageTypeDiff {
		t.Fatalf("legacy derived diff message_type = %q, want %q", out[1].MessageType, db.MessageTypeDiff)
	}
	if strings.Contains(out[1].Content, "json.txt") {
		t.Fatalf("legacy mode unexpectedly used all json diffs: %q", out[1].Content)
	}
}
