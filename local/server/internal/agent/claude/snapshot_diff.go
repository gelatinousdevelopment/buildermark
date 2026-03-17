package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent"
	"github.com/gelatinousdevelopment/buildermark/local/server/internal/db"
	"github.com/pmezard/go-difflib/difflib"
)

type claudeSnapshotState struct {
	// repo-relative file path -> backup file name in ~/.claude/file-history/<session>/
	backups map[string]string
	// repo-relative file path -> most recent full file snapshot seen in the conversation.
	lastContents map[string]string
}

func newClaudeSnapshotState() *claudeSnapshotState {
	return &claudeSnapshotState{
		backups:      map[string]string{},
		lastContents: map[string]string{},
	}
}

func (s *claudeSnapshotState) ingestRawJSON(raw string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return
	}

	var entry struct {
		Type     string `json:"type"`
		Snapshot struct {
			TrackedFileBackups map[string]struct {
				BackupFileName *string `json:"backupFileName"`
			} `json:"trackedFileBackups"`
		} `json:"snapshot"`
	}
	if err := json.Unmarshal([]byte(raw), &entry); err != nil {
		return
	}
	if strings.TrimSpace(entry.Type) != "file-history-snapshot" {
		return
	}
	for path, backup := range entry.Snapshot.TrackedFileBackups {
		if backup.BackupFileName == nil {
			continue
		}
		name := strings.TrimSpace(*backup.BackupFileName)
		if name == "" {
			continue
		}
		norm := normalizeClaudeSnapshotPath(path)
		if norm == "" {
			continue
		}
		s.backups[norm] = name
	}
}

func (s *claudeSnapshotState) observeMessage(m db.Message) {
	if s == nil {
		return
	}
	snap, ok := extractClaudeWriteSnapshot(m)
	if !ok {
		return
	}
	for _, relPath := range snap.paths {
		s.lastContents[relPath] = snap.newContent
	}
}

type claudeWriteSnapshot struct {
	sessionID       string
	paths           []string
	newContent      string
	originalContent string
	isCreate        bool
}

func deriveClaudeSnapshotDiff(m db.Message, state *claudeSnapshotState) (string, bool) {
	if state == nil {
		return "", false
	}

	snap, ok := extractClaudeWriteSnapshot(m)
	if !ok {
		return "", false
	}
	if snap.originalContent != "" {
		if diff, ok := buildUnifiedDiff(snap.originalContent, snap.newContent, snap.paths[0]); ok {
			return diff, true
		}
	}
	for _, relPath := range snap.paths {
		if oldContent, ok := state.lastContents[relPath]; ok {
			if diff, ok := buildUnifiedDiff(oldContent, snap.newContent, relPath); ok {
				return diff, true
			}
		}
	}

	for _, relPath := range snap.paths {
		backupName, ok := state.backups[relPath]
		if !ok {
			continue
		}
		oldContent, ok := readClaudeBackupContent(snap.sessionID, backupName)
		if !ok {
			continue
		}
		oldContent = strings.ReplaceAll(oldContent, "\r\n", "\n")
		if oldContent == snap.newContent {
			return "", false
		}
		diff, ok := buildUnifiedDiff(oldContent, snap.newContent, relPath)
		if !ok {
			continue
		}
		return diff, true
	}

	if snap.isCreate {
		if diff, ok := buildClaudeAddFileDiff(snap.newContent, snap.paths[0]); ok {
			return diff, true
		}
	}

	return "", false
}

func extractClaudeWriteSnapshot(m db.Message) (claudeWriteSnapshot, bool) {
	raw := strings.TrimSpace(m.RawJSON)
	if raw == "" {
		return claudeWriteSnapshot{}, false
	}

	var payload struct {
		Cwd       string `json:"cwd"`
		SessionID string `json:"sessionId"`
		Message   struct {
			Content []struct {
				Type  string `json:"type"`
				Name  string `json:"name"`
				Input struct {
					FilePathSnake string `json:"file_path"`
					FilePathCamel string `json:"filePath"`
					Content       string `json:"content"`
				} `json:"input"`
			} `json:"content"`
		} `json:"message"`
		ToolUseResult struct {
			Type         string  `json:"type"`
			FilePath     string  `json:"filePath"`
			Content      string  `json:"content"`
			OriginalFile *string `json:"originalFile"`
			File         struct {
				FilePath     string  `json:"filePath"`
				Content      string  `json:"content"`
				OriginalFile *string `json:"originalFile"`
			} `json:"file"`
		} `json:"toolUseResult"`
	}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return claudeWriteSnapshot{}, false
	}

	filePath := strings.TrimSpace(payload.ToolUseResult.FilePath)
	newContent := strings.ReplaceAll(payload.ToolUseResult.Content, "\r\n", "\n")
	originalContent := ""
	if payload.ToolUseResult.OriginalFile != nil {
		originalContent = strings.ReplaceAll(*payload.ToolUseResult.OriginalFile, "\r\n", "\n")
	}
	if filePath == "" {
		filePath = strings.TrimSpace(payload.ToolUseResult.File.FilePath)
	}
	if newContent == "" {
		newContent = strings.ReplaceAll(payload.ToolUseResult.File.Content, "\r\n", "\n")
	}
	if originalContent == "" && payload.ToolUseResult.File.OriginalFile != nil {
		originalContent = strings.ReplaceAll(*payload.ToolUseResult.File.OriginalFile, "\r\n", "\n")
	}
	isCreate := strings.EqualFold(strings.TrimSpace(payload.ToolUseResult.Type), "create")

	if filePath == "" || strings.TrimSpace(newContent) == "" {
		for _, block := range payload.Message.Content {
			if !strings.EqualFold(strings.TrimSpace(block.Type), "tool_use") {
				continue
			}
			if !strings.EqualFold(strings.TrimSpace(block.Name), "Write") {
				continue
			}
			candidatePath := strings.TrimSpace(block.Input.FilePathSnake)
			if candidatePath == "" {
				candidatePath = strings.TrimSpace(block.Input.FilePathCamel)
			}
			candidateContent := strings.ReplaceAll(block.Input.Content, "\r\n", "\n")
			if candidatePath == "" || strings.TrimSpace(candidateContent) == "" {
				continue
			}
			filePath = candidatePath
			newContent = candidateContent
			break
		}
	}

	if filePath == "" || strings.TrimSpace(newContent) == "" {
		return claudeWriteSnapshot{}, false
	}

	sessionID := strings.TrimSpace(payload.SessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(m.ConversationID)
	}
	if sessionID == "" {
		return claudeWriteSnapshot{}, false
	}

	paths := buildClaudePathCandidates(filePath, payload.Cwd)
	if len(paths) == 0 {
		return claudeWriteSnapshot{}, false
	}

	return claudeWriteSnapshot{
		sessionID:       sessionID,
		paths:           paths,
		newContent:      newContent,
		originalContent: originalContent,
		isCreate:        isCreate,
	}, true
}

func readClaudeBackupContent(sessionID, backupName string) (string, bool) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	path := filepath.Join(home, ".claude", "file-history", sessionID, backupName)
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return string(b), true
}

func buildUnifiedDiff(oldContent, newContent, relPath string) (string, bool) {
	if strings.TrimSpace(relPath) == "" {
		return "", false
	}
	if !strings.HasSuffix(oldContent, "\n") {
		oldContent += "\n"
	}
	if !strings.HasSuffix(newContent, "\n") {
		newContent += "\n"
	}

	diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(oldContent),
		B:        difflib.SplitLines(newContent),
		FromFile: "a/" + relPath,
		ToFile:   "b/" + relPath,
		Context:  3,
	})
	if err != nil {
		return "", false
	}
	diff = strings.TrimSpace(diff)
	if diff == "" {
		return "", false
	}
	if _, ok := agent.ExtractReliableDiff(diff); !ok {
		return "", false
	}
	return diff, true
}

func buildClaudeAddFileDiff(newContent, relPath string) (string, bool) {
	relPath = strings.TrimSpace(relPath)
	if relPath == "" {
		return "", false
	}
	lines := strings.Split(strings.ReplaceAll(newContent, "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return "", false
	}

	var out strings.Builder
	out.WriteString("diff --git a/")
	out.WriteString(relPath)
	out.WriteString(" b/")
	out.WriteString(relPath)
	out.WriteString("\n--- /dev/null\n+++ b/")
	out.WriteString(relPath)
	out.WriteString("\n")
	out.WriteString("@@ -0,0 +1,")
	out.WriteString(strconv.Itoa(len(lines)))
	out.WriteString(" @@\n")
	for _, line := range lines {
		out.WriteString("+")
		out.WriteString(line)
		out.WriteString("\n")
	}

	diff := strings.TrimSpace(out.String())
	if diff == "" {
		return "", false
	}
	if _, ok := agent.ExtractReliableDiff(diff); !ok {
		return "", false
	}
	return diff, true
}

func buildClaudePathCandidates(filePath, cwd string) []string {
	path := strings.TrimSpace(filePath)
	if path == "" {
		return nil
	}

	candidates := make([]string, 0, 3)
	seen := map[string]struct{}{}
	add := func(p string) {
		norm := normalizeClaudeSnapshotPath(p)
		if norm == "" {
			return
		}
		if _, ok := seen[norm]; ok {
			return
		}
		seen[norm] = struct{}{}
		candidates = append(candidates, norm)
	}

	if !filepath.IsAbs(path) {
		add(path)
		if cwd = strings.TrimSpace(cwd); cwd != "" {
			add(filepath.Join(cwd, path))
			if root, ok := findClaudeGitRoot(cwd); ok {
				add(filepath.Join(root, path))
			}
		}
		return candidates
	}

	if cwd = strings.TrimSpace(cwd); cwd != "" {
		if rel, ok := claudeRelIfContained(cwd, path); ok {
			add(rel)
		}
		if root, ok := findClaudeGitRoot(cwd); ok {
			if rel, ok := claudeRelIfContained(root, path); ok {
				add(rel)
			}
		}
	}
	add(path)

	return candidates
}

func normalizeClaudeSnapshotPath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.ReplaceAll(path, "\\", "/")
	path = strings.TrimPrefix(path, "./")
	path = strings.TrimPrefix(path, "/")
	return strings.TrimSpace(path)
}

func claudeRelIfContained(base, target string) (string, bool) {
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(target))
	if err != nil {
		return "", false
	}
	if rel == "." || rel == "" {
		return "", false
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}

func findClaudeGitRoot(start string) (string, bool) {
	dir := filepath.Clean(start)
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}
