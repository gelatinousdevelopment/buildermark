package gemini

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/agent"
)

// readSessionTitle returns a title for the given session by extracting the
// first user prompt from the session file.
func readSessionTitle(path string) string {
	conv, err := readConversation(path)
	if err != nil {
		return ""
	}
	for _, m := range conv.Messages {
		if m.Type != "user" {
			continue
		}
		if text := extractMessageText(m); text != "" {
			return agent.TitleFromPrompt(text)
		}
	}
	return ""
}

func readConversation(path string) (*geminiConversation, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var conv geminiConversation
	if err := json.Unmarshal(data, &conv); err != nil {
		return nil, err
	}
	if strings.TrimSpace(conv.Model) == "" {
		conv.Model = detectGeminiModelFromJSON(data)
	}
	return &conv, nil
}

func detectGeminiModelFromJSON(data []byte) string {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return ""
	}
	return findGeminiModel(v)
}

func findGeminiModel(v any) string {
	switch x := v.(type) {
	case map[string]any:
		for _, k := range []string{"model", "modelName", "model_name", "model_slug", "selectedModel"} {
			if s, ok := x[k].(string); ok {
				s = strings.TrimSpace(s)
				if strings.Contains(strings.ToLower(s), "gemini") {
					return s
				}
			}
		}
		for _, nested := range x {
			if m := findGeminiModel(nested); m != "" {
				return m
			}
		}
	case []any:
		for _, item := range x {
			if m := findGeminiModel(item); m != "" {
				return m
			}
		}
	}
	return ""
}

func inferProjectPath(conv *geminiConversation) string {
	for _, d := range conv.Directories {
		d = strings.TrimSpace(d)
		if filepath.IsAbs(d) {
			return d
		}
	}

	for _, m := range conv.Messages {
		for _, tc := range m.ToolCalls {
			for _, key := range []string{"absolute_path", "file_path", "path", "cwd", "dir_path"} {
				v, ok := tc.Args[key]
				if !ok {
					continue
				}
				s, ok := v.(string)
				if !ok {
					continue
				}
				s = strings.TrimSpace(s)
				if s == "" {
					continue
				}
				if !filepath.IsAbs(s) {
					continue
				}
				if key == "cwd" {
					return s
				}
				if key == "dir_path" {
					return s
				}
				return filepath.Dir(s)
			}
		}
	}

	return ""
}

func hashProjectPath(path string) string {
	sum := sha256.Sum256([]byte(path))
	return fmt.Sprintf("%x", sum)
}
