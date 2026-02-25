package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/gelatinousdevelopment/buildermark/web/server/internal/agent"
	"github.com/gelatinousdevelopment/buildermark/web/server/internal/db"
	"github.com/pmezard/go-difflib/difflib"
)

type claudeSnapshotState struct {
	// repo-relative file path -> backup file name in ~/.claude/file-history/<session>/
	backups map[string]string
}

func newClaudeSnapshotState() *claudeSnapshotState {
	return &claudeSnapshotState{backups: map[string]string{}}
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

func deriveClaudeSnapshotDiff(m db.Message, state *claudeSnapshotState) (string, bool) {
	if state == nil || len(state.backups) == 0 {
		return "", false
	}

	var payload struct {
		Cwd           string `json:"cwd"`
		SessionID     string `json:"sessionId"`
		ToolUseResult struct {
			Type string `json:"type"`
			File struct {
				FilePath string `json:"filePath"`
				Content  string `json:"content"`
			} `json:"file"`
		} `json:"toolUseResult"`
	}
	if err := json.Unmarshal([]byte(m.RawJSON), &payload); err != nil {
		return "", false
	}

	filePath := strings.TrimSpace(payload.ToolUseResult.File.FilePath)
	newContent := strings.ReplaceAll(payload.ToolUseResult.File.Content, "\r\n", "\n")
	if filePath == "" || strings.TrimSpace(newContent) == "" {
		return "", false
	}

	sessionID := strings.TrimSpace(payload.SessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(m.ConversationID)
	}
	if sessionID == "" {
		return "", false
	}

	candidates := buildClaudePathCandidates(filePath, payload.Cwd)
	for _, relPath := range candidates {
		backupName, ok := state.backups[relPath]
		if !ok {
			continue
		}
		oldContent, ok := readClaudeBackupContent(sessionID, backupName)
		if !ok {
			continue
		}
		oldContent = strings.ReplaceAll(oldContent, "\r\n", "\n")
		if oldContent == newContent {
			return "", false
		}
		diff, ok := buildUnifiedDiff(oldContent, newContent, relPath)
		if !ok {
			continue
		}
		return diff, true
	}

	return "", false
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

	add(path)

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
