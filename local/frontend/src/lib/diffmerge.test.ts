import { describe, it, expect } from 'vitest';
import { mergeSequentialDiffs } from './diffmerge';

/**
 * Helper: wraps a raw diff string in ```diff fences like the inputs will have.
 */
function fenced(raw: string): string {
	return '```diff\n' + raw + '\n```';
}

/**
 * Helper: parse a unified diff to extract added/removed lines per hunk.
 * Returns an array of {oldStart, oldCount, newStart, newCount, lines} objects.
 */
function parseHunks(diff: string) {
	const lines = diff.split('\n');
	const hunks: {
		oldStart: number;
		oldCount: number;
		newStart: number;
		newCount: number;
		lines: string[];
	}[] = [];
	let current: (typeof hunks)[number] | null = null;

	for (const line of lines) {
		const m = line.match(/^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/);
		if (m) {
			current = {
				oldStart: parseInt(m[1]),
				oldCount: parseInt(m[2] ?? '1'),
				newStart: parseInt(m[3]),
				newCount: parseInt(m[4] ?? '1'),
				lines: []
			};
			hunks.push(current);
		} else if (current && (line.startsWith('+') || line.startsWith('-') || line.startsWith(' '))) {
			current.lines.push(line);
		}
	}
	return hunks;
}

/**
 * Helper: extract only added lines (starting with +) from a diff string.
 */
function addedLines(diff: string): string[] {
	return diff
		.split('\n')
		.filter((l) => l.startsWith('+') && !l.startsWith('+++'))
		.map((l) => l.slice(1));
}

/**
 * Helper: extract only removed lines (starting with -) from a diff string.
 */
function removedLines(diff: string): string[] {
	return diff
		.split('\n')
		.filter((l) => l.startsWith('-') && !l.startsWith('---'))
		.map((l) => l.slice(1));
}

describe('mergeSequentialDiffs', () => {
	describe('Test 1: Non-overlapping diffs', () => {
		// Two diffs that touch different parts of the file.
		// Diff A adds a function after line 10.
		// Diff B adds a function after line 50.
		// Combined diff should show both additions.

		const diffA = fenced(
			`diff --git a/example.go b/example.go
--- a/example.go
+++ b/example.go
@@ -8,6 +8,9 @@
   line8
   line9
   line10
+  func newFuncA() {
+    return "A"
+  }
   line11
   line12
   line13`
		);

		const diffB = fenced(
			`diff --git a/example.go b/example.go
--- a/example.go
+++ b/example.go
@@ -53,6 +53,9 @@
   line50
   line51
   line52
+  func newFuncB() {
+    return "B"
+  }
   line53
   line54
   line55`
		);

		it('should include additions from both diffs', () => {
			const result = mergeSequentialDiffs([diffA, diffB]);
			const added = addedLines(result);
			expect(added.some((l) => l.includes('newFuncA'))).toBe(true);
			expect(added.some((l) => l.includes('newFuncB'))).toBe(true);
		});

		it('should not remove any lines', () => {
			const result = mergeSequentialDiffs([diffA, diffB]);
			const removed = removedLines(result);
			expect(removed).toHaveLength(0);
		});

		it('should produce valid unified diff format', () => {
			const result = mergeSequentialDiffs([diffA, diffB]);
			expect(result).toContain('---');
			expect(result).toContain('+++');
			expect(result).toMatch(/@@ -\d+,?\d* \+\d+,?\d* @@/);
		});
	});

	describe('Test 2: Overlapping / same-region diffs (real data)', () => {
		const diff1 = fenced(
			`diff --git a/local/server/internal/agent/claude/claude_test.go b/local/server/internal/agent/claude/claude_test.go
--- a/local/server/internal/agent/claude/claude_test.go
+++ b/local/server/internal/agent/claude/claude_test.go
@@ -2099,6 +2099,38 @@
   }
 }

+func TestReadConversationLogEntriesSkillExpansionStoredAsAgent(t *testing.T) {
+  home := t.TempDir()
+  projectPath := "/proj/test"
+  sessionID := "sess-1"
+
+  convPath := conversationPath(home, projectPath, sessionID)
+  if err := os.MkdirAll(filepath.Dir(convPath), 0o755); err != nil {
+    t.Fatalf("mkdir: %v", err)
+  }
+
+  lines := []string{
+    // Skill expansion prompt injected by Claude Code
+    fmt.Sprintf(\`{"type":"user","timestamp":"2026-02-18T10:00:00.000Z","sessionId":%q,"cwd":%q,"message":{"role":"user","content":"Base directory for this skill: /home/user/project/plugins/claudecode/skills/brate\\n\\nThe user wants to rate this conversation."}}\`, sessionID, projectPath),
+    // Real user message
+    fmt.Sprintf(\`{"type":"user","timestamp":"2026-02-18T10:00:01.000Z","sessionId":%q,"cwd":%q,"message":{"role":"user","content":"real user prompt"}}\`, sessionID, projectPath),
+  }
+  if err := os.WriteFile(convPath, []byte(strings.Join(lines, "\\n")+"\\n"), 0o644); err != nil {
+    t.Fatalf("write: %v", err)
+  }
+
+  entries := readConversationLogEntries(home, projectPath, sessionID)
+  if len(entries) != 2 {
+    t.Fatalf("entries len = %d, want 2", len(entries))
+  }
+  if entries[0].Role != "agent" {
+    t.Errorf("skill expansion role = %q, want %q", entries[0].Role, "agent")
+  }
+  if entries[1].Role != "user" {
+    t.Errorf("real user role = %q, want %q", entries[1].Role, "user")
+  }
+}
+
 func TestReadConversationLogEntriesKeepsSummaryEntries(t *testing.T) {
   home := t.TempDir()
   projectPath := "/proj/test"`
		);

		const diff2 = fenced(
			`diff --git a/local/server/internal/agent/claude/claude_test.go b/local/server/internal/agent/claude/claude_test.go
--- a/local/server/internal/agent/claude/claude_test.go
+++ b/local/server/internal/agent/claude/claude_test.go
@@ -1,2 +1,28 @@
+func TestReadFirstPromptSkipsSkillExpansion(t *testing.T) {
+    tmpDir := t.TempDir()
+
+    projectPath := "/proj/d"
+    sessionID := "sess-skill-exp"
+    dirName := "-proj-d"
+    convDir := filepath.Join(tmpDir, ".claude", "projects", dirName)
+    if err := os.MkdirAll(convDir, 0755); err != nil {
+        t.Fatalf("mkdir: %v", err)
+    }
+
+    convPath := filepath.Join(convDir, sessionID+".jsonl")
+    lines := []string{
+        \`{"type":"user","timestamp":"2026-01-01T00:00:00.000Z","message":{"content":"Base directory for this skill: /home/user/project/plugins/claudecode/skills/brate\\n\\nThe user wants to rate this conversation."}}\`,
+        \`{"type":"user","timestamp":"2026-01-01T00:00:01.000Z","message":{"content":"real prompt"}}\`,
+    }
+    if err := os.WriteFile(convPath, []byte(fmt.Sprintf("%s\\n", joinLines(lines))), 0644); err != nil {
+        t.Fatalf("write conv file: %v", err)
+    }
+
+    text, _ := readFirstPrompt(tmpDir, projectPath, sessionID)
+    if text != "real prompt" {
+        t.Errorf("text = %q, want %q", text, "real prompt")
+    }
+}
+
 func TestProcessEntriesDoesNotDuplicateFirstPrompt(t *testing.T) {`
		);

		const diff3 = fenced(
			`diff --git a/local/server/internal/agent/claude/claude_test.go b/local/server/internal/agent/claude/claude_test.go
--- a/local/server/internal/agent/claude/claude_test.go
+++ b/local/server/internal/agent/claude/claude_test.go
@@ -1130,6 +1130,32 @@
   }
 }

+func TestReadFirstPromptSkipsSkillExpansion(t *testing.T) {
+  tmpDir := t.TempDir()
+
+  projectPath := "/proj/d"
+  sessionID := "sess-skill-exp"
+  dirName := "-proj-d"
+  convDir := filepath.Join(tmpDir, ".claude", "projects", dirName)
+  if err := os.MkdirAll(convDir, 0755); err != nil {
+    t.Fatalf("mkdir: %v", err)
+  }
+
+  convPath := filepath.Join(convDir, sessionID+".jsonl")
+  lines := []string{
+    \`{"type":"user","timestamp":"2026-01-01T00:00:00.000Z","message":{"content":"Base directory for this skill: /home/user/project/plugins/claudecode/skills/brate\\n\\nThe user wants to rate this conversation."}}\`,
+    \`{"type":"user","timestamp":"2026-01-01T00:00:01.000Z","message":{"content":"real prompt"}}\`,
+  }
+  if err := os.WriteFile(convPath, []byte(fmt.Sprintf("%s\\n", joinLines(lines))), 0644); err != nil {
+    t.Fatalf("write conv file: %v", err)
+  }
+
+  text, _ := readFirstPrompt(tmpDir, projectPath, sessionID)
+  if text != "real prompt" {
+    t.Errorf("text = %q, want %q", text, "real prompt")
+  }
+}
+
 func TestProcessEntriesDoesNotDuplicateFirstPrompt(t *testing.T) {
   database := setupTestDB(t)
   tmpDir := t.TempDir()`
		);

		it('should include TestReadConversationLogEntriesSkillExpansionStoredAsAgent from diff1', () => {
			const result = mergeSequentialDiffs([diff1, diff2, diff3]);
			const added = addedLines(result);
			expect(
				added.some((l) => l.includes('TestReadConversationLogEntriesSkillExpansionStoredAsAgent'))
			).toBe(true);
		});

		it('should include TestReadFirstPromptSkipsSkillExpansion from diff3 (final placement)', () => {
			const result = mergeSequentialDiffs([diff1, diff2, diff3]);
			const added = addedLines(result);
			expect(added.some((l) => l.includes('TestReadFirstPromptSkipsSkillExpansion'))).toBe(true);
		});

		it('should NOT have the diff2 version at the top of the file (wrong placement)', () => {
			// Diff2 added the function at line 1 (wrong). Diff3 corrected it to line 1130.
			// The combined diff should NOT add TestReadFirstPromptSkipsSkillExpansion near line 1.
			const result = mergeSequentialDiffs([diff1, diff2, diff3]);
			const hunks = parseHunks(result);

			// Find hunk(s) that contain TestReadFirstPromptSkipsSkillExpansion
			const relevantHunks = hunks.filter((h) =>
				h.lines.some((l) => l.includes('TestReadFirstPromptSkipsSkillExpansion'))
			);

			// All relevant hunks should be at high line numbers (around 1130+), not at line 1
			for (const h of relevantHunks) {
				expect(h.oldStart).toBeGreaterThan(100);
			}
		});

		it('should use the diff3 indentation (2 spaces, not 4)', () => {
			// Diff2 used 4-space indentation, diff3 corrected to 2-space.
			// The combined diff should have the corrected 2-space indentation.
			const result = mergeSequentialDiffs([diff1, diff2, diff3]);
			const added = addedLines(result);
			const tmpDirLine = added.find((l) => l.includes('tmpDir := t.TempDir()'));
			expect(tmpDirLine).toBeDefined();
			// Should be 2-space indent (from diff3), not 4-space (from diff2)
			expect(tmpDirLine!.startsWith('  tmpDir')).toBe(true);
			expect(tmpDirLine!.startsWith('    tmpDir')).toBe(false);
		});

		it('should produce a valid unified diff', () => {
			const result = mergeSequentialDiffs([diff1, diff2, diff3]);
			expect(result).toContain('---');
			expect(result).toContain('+++');
			expect(result).toMatch(/@@ -\d+,?\d* \+\d+,?\d* @@/);
		});
	});

	describe('Test 3: Adjacent diffs', () => {
		// Two diffs where one adds lines right after where the other ends.
		// They should merge into a single hunk (or adjacent hunks).

		const diffA = fenced(
			`diff --git a/example.go b/example.go
--- a/example.go
+++ b/example.go
@@ -10,6 +10,8 @@
   line10
   line11
   line12
+  added_line_A1
+  added_line_A2
   line13
   line14
   line15`
		);

		const diffB = fenced(
			`diff --git a/example.go b/example.go
--- a/example.go
+++ b/example.go
@@ -17,6 +17,8 @@
   line15
   line16
   line17
+  added_line_B1
+  added_line_B2
   line18
   line19
   line20`
		);

		it('should include additions from both diffs', () => {
			const result = mergeSequentialDiffs([diffA, diffB]);
			const added = addedLines(result);
			expect(added.some((l) => l.includes('added_line_A1'))).toBe(true);
			expect(added.some((l) => l.includes('added_line_A2'))).toBe(true);
			expect(added.some((l) => l.includes('added_line_B1'))).toBe(true);
			expect(added.some((l) => l.includes('added_line_B2'))).toBe(true);
		});

		it('should produce hunks that could be a single merged hunk or adjacent', () => {
			const result = mergeSequentialDiffs([diffA, diffB]);
			const hunks = parseHunks(result);
			// Either one merged hunk or two adjacent hunks — both are valid.
			// Just verify the total added lines count is correct.
			const totalAdded = hunks.reduce(
				(sum, h) => sum + h.lines.filter((l) => l.startsWith('+')).length,
				0
			);
			expect(totalAdded).toBe(4);
		});

		it('should not remove any lines', () => {
			const result = mergeSequentialDiffs([diffA, diffB]);
			const removed = removedLines(result);
			expect(removed).toHaveLength(0);
		});
	});

	describe('Edge cases', () => {
		it('should handle a single diff (passthrough)', () => {
			const diff = fenced(
				`diff --git a/file.txt b/file.txt
--- a/file.txt
+++ b/file.txt
@@ -1,3 +1,4 @@
 line1
+new_line
 line2
 line3`
			);
			const result = mergeSequentialDiffs([diff]);
			const added = addedLines(result);
			expect(added).toContain('new_line');
		});

		it('should handle diffs that delete lines', () => {
			const diff1 = fenced(
				`diff --git a/file.txt b/file.txt
--- a/file.txt
+++ b/file.txt
@@ -1,4 +1,3 @@
 line1
-line_to_delete
 line3
 line4`
			);
			const diff2 = fenced(
				`diff --git a/file.txt b/file.txt
--- a/file.txt
+++ b/file.txt
@@ -1,3 +1,4 @@
 line1
 line3
+added_after
 line4`
			);
			const result = mergeSequentialDiffs([diff1, diff2]);
			const removed = removedLines(result);
			const added = addedLines(result);
			expect(removed).toContain('line_to_delete');
			expect(added).toContain('added_after');
		});

		it('should handle diffs that replace lines', () => {
			const diff1 = fenced(
				`diff --git a/file.txt b/file.txt
--- a/file.txt
+++ b/file.txt
@@ -1,3 +1,3 @@
 line1
-old_line2
+new_line2
 line3`
			);
			const result = mergeSequentialDiffs([diff1]);
			const removed = removedLines(result);
			const added = addedLines(result);
			expect(removed).toContain('old_line2');
			expect(added).toContain('new_line2');
		});
	});
});
