package claude

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent"
)

type pastedContent struct {
	ID          int    `json:"id"`
	Type        string `json:"type"`
	ContentHash string `json:"contentHash"`
}

type historyEntry struct {
	Display                 string                   `json:"display"`
	Timestamp               int64                    `json:"timestamp"`
	SessionID               string                   `json:"sessionId"`
	Project                 string                   `json:"project"`
	Type                    string                   `json:"type"`
	Model                   string                   `json:"model"`
	Summary                 string                   `json:"summary"`
	SourceToolAssistantUUID string                   `json:"sourceToolAssistantUUID"`
	IsSidechain             bool                     `json:"isSidechain"`
	UserType                string                   `json:"userType"`
	AgentID                 string                   `json:"agentId"`
	Message                 historyEntryMessage      `json:"message"`
	PastedContents          map[string]pastedContent `json:"pastedContents"`
	RawJSON                 string                   `json:"-"`
}

type historyEntryMessage struct {
	Model string `json:"model"`
}

// ResolveSession polls history.jsonl for up to 5 seconds looking for a
// /bb entry. When found it collects every entry with the same
// sessionId and returns them all.
// If no match is found the fallbackID is returned with no entries.
func (a *Agent) ResolveSession(rating int, note string, fallbackID string) *agent.SessionResult {
	const (
		pollInterval = 500 * time.Millisecond
		maxWait      = 5 * time.Second
		tailBytes    = int64(64 * 1024)
		maxAge       = 30 * time.Second
	)

	var sessionID string
	deadline := time.Now().Add(maxWait)
	for {
		if sid, ok := searchHistory(a.path, tailBytes, maxAge); ok {
			sessionID = sid
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(pollInterval)
	}

	if sessionID == "" {
		log.Printf("claude session: no match found for /bb command, using fallback")
		return &agent.SessionResult{SessionID: fallbackID}
	}

	log.Printf("claude session: matched entry with sessionId=%s", sessionID)

	entries := collectSessionEntries(a.Home, a.path, sessionID)

	project := ""
	if len(entries) > 0 {
		project = entries[0].Project
	}

	return &agent.SessionResult{
		SessionID: sessionID,
		Project:   project,
		Entries:   entries,
	}
}

// isRatingDisplay returns true if the display field represents a /bb
// command invocation. Matches "/bb", "/bb 4", "/bb:rate", etc.
func isRatingDisplay(display string) bool {
	d := strings.TrimSpace(display)
	return d == "/bb" || d == "/bb:rate" || d == "/brate" || d == "/rate-buildermark" ||
		strings.HasPrefix(d, "/bb ") || strings.HasPrefix(d, "/bb:rate ") || strings.HasPrefix(d, "/brate ") || strings.HasPrefix(d, "/rate-buildermark ")
}

// searchHistory reads the last tailBytes of the history file and searches
// lines in reverse for a /bb entry whose timestamp is within maxAge of now.
func searchHistory(path string, tailBytes int64, maxAge time.Duration) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return "", false
	}

	offset := int64(0)
	readSize := info.Size()
	if readSize > tailBytes {
		offset = readSize - tailBytes
		readSize = tailBytes
	}

	buf := make([]byte, readSize)
	if _, err := f.ReadAt(buf, offset); err != nil && err != io.EOF {
		return "", false
	}

	lines := strings.Split(string(buf), "\n")
	now := time.Now()

	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}

		var entry historyEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		entryTime := time.UnixMilli(entry.Timestamp)
		if now.Sub(entryTime) > maxAge {
			break
		}

		if isRatingDisplay(entry.Display) && entry.SessionID != "" {
			return entry.SessionID, true
		}
	}

	return "", false
}

// collectSessionEntries reads the full history file and returns every entry
// whose sessionId matches the given id, ordered chronologically.
func collectSessionEntries(home, path, sessionID string) []agent.Entry {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("claude session: error reading file for session collection: %v", err)
		return nil
	}

	lines := strings.Split(string(data), "\n")
	var entries []agent.Entry

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var entry historyEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		entry.RawJSON = line
		if entry.SessionID != sessionID {
			continue
		}

		role := "user"
		if isAssistantAuthoredHistoryEntry(entry) {
			role = "agent"
		}

		display := entry.Display
		if entry.Type == "summary" && strings.TrimSpace(entry.Summary) != "" {
			display = strings.TrimSpace(entry.Summary)
		}
		if len(entry.PastedContents) > 0 {
			display = resolvePastedContents(home, display, entry.PastedContents)
		}
		if strings.TrimSpace(display) == "" {
			display = "[" + strings.TrimSpace(entry.Type) + "]"
		}

		entries = append(entries, agent.Entry{
			Timestamp: entry.Timestamp,
			SessionID: entry.SessionID,
			Project:   entry.Project,
			Role:      role,
			Model:     historyEntryModel(entry),
			Display:   display,
			RawJSON:   line,
		})
	}

	return agent.AppendDiffEntries(entries)
}

func historyEntryModel(e historyEntry) string {
	if model := strings.TrimSpace(e.Model); model != "" {
		return model
	}
	if model := strings.TrimSpace(e.Message.Model); model != "" {
		return model
	}
	if model := extractModelFromJSONLine(e.RawJSON); model != "" {
		return model
	}
	return ""
}

func isAssistantAuthoredHistoryEntry(entry historyEntry) bool {
	if entry.Type == "assistant" || entry.Type == "summary" || strings.TrimSpace(entry.SourceToolAssistantUUID) != "" {
		return true
	}
	// Skill expansion prompts (e.g. the expanded SKILL.md injected by Claude
	// Code when the user runs /rate-buildermark) are system-generated, not user-authored.
	if entry.Type == "user" && IsSkillExpansion(entry.Display) {
		return true
	}
	// Claude can log assistant-generated subagent prompts as type=user entries.
	// Treat these as agent turns to avoid misattribution in the UI.
	return entry.Type == "user" &&
		entry.IsSidechain &&
		strings.EqualFold(strings.TrimSpace(entry.UserType), "external") &&
		strings.TrimSpace(entry.AgentID) != ""
}

// extractStopReasonFromJSON extracts message.stop_reason from a raw JSON line.
func extractStopReasonFromJSON(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}
	var obj struct {
		Message struct {
			StopReason string `json:"stop_reason"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		return ""
	}
	return strings.TrimSpace(obj.Message.StopReason)
}

func extractModelFromJSONLine(line string) string {
	line = strings.TrimSpace(line)
	if line == "" {
		return ""
	}

	var obj any
	if err := json.Unmarshal([]byte(line), &obj); err != nil {
		return ""
	}
	return findModelString(obj)
}

func findModelString(v any) string {
	switch x := v.(type) {
	case map[string]any:
		// Check direct model-like keys first at this level.
		for _, k := range []string{"model", "model_name", "modelName", "model_slug", "modelSlug"} {
			if s, ok := x[k].(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					return s
				}
			}
		}
		for _, nested := range x {
			if found := findModelString(nested); found != "" {
				return found
			}
		}
	case []any:
		for _, item := range x {
			if found := findModelString(item); found != "" {
				return found
			}
		}
	}
	return ""
}

// resolvePastedContents replaces [Pasted text #N] placeholders in display
// with the actual content from ~/.claude/paste-cache/{contentHash}.txt.
func resolvePastedContents(home, display string, pasted map[string]pastedContent) string {
	for _, pc := range pasted {
		if pc.Type != "text" || pc.ContentHash == "" {
			continue
		}

		placeholder := fmt.Sprintf("[Pasted text #%d]", pc.ID)
		cachePath := filepath.Join(home, ".claude", "paste-cache", pc.ContentHash+".txt")

		content, err := os.ReadFile(cachePath)
		if err != nil {
			log.Printf("claude session: failed to read paste cache %s: %v", cachePath, err)
			continue
		}

		display = strings.Replace(display, placeholder, string(content), 1)
	}
	return display
}
