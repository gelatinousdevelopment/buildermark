package cursor

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// composerData is the top-level JSON structure stored in the global DB under
// "composerData:{id}" keys.
type composerData struct {
	ComposerID    string `json:"composerId"`
	Name          string `json:"name"`
	CreatedAt     int64  `json:"createdAt"`     // ms epoch
	LastUpdatedAt int64  `json:"lastUpdatedAt"` // ms epoch

	// New format: inline messages.
	Conversation []composerBubble `json:"conversation"`

	// Old format: headers only, content requires bubbleId lookups.
	FullConversationHeadersOnly []bubbleHeader `json:"fullConversationHeadersOnly"`

	// Context for project path inference.
	Context *composerContext `json:"context"`
}

// composerBubble is an inline message in the new format's conversation array.
type composerBubble struct {
	BubbleID       string            `json:"bubbleId"`
	Type           int               `json:"type"` // 1=user, 2=assistant
	Text           string            `json:"text"`
	RichText       string            `json:"richText"`
	TimingInfo     *bubbleTimingInfo `json:"timingInfo"`
	ToolFormerData json.RawMessage   `json:"toolFormerData"`
	IsThought      bool              `json:"isThought"`
}

// bubbleTimingInfo contains timing data for assistant bubbles.
type bubbleTimingInfo struct {
	ClientRpcSendTime int64 `json:"clientRpcSendTime"` // ms epoch
}

// bubbleHeader is the old format entry in fullConversationHeadersOnly.
type bubbleHeader struct {
	BubbleID string `json:"bubbleId"`
	Type     int    `json:"type"` // 1=user, 2=assistant
}

// bubbleData is the full bubble content from a "bubbleId:{convId}:{bubbleId}" lookup.
type bubbleData struct {
	BubbleID       string            `json:"bubbleId"`
	Type           int               `json:"type"`
	Text           string            `json:"text"`
	RichText       string            `json:"richText"`
	TimingInfo     *bubbleTimingInfo `json:"timingInfo"`
	ToolFormerData json.RawMessage   `json:"toolFormerData"`
	IsThought      bool              `json:"isThought"`
}

// composerContext holds context metadata attached to a composer.
type composerContext struct {
	FileSelections []fileSelection `json:"fileSelections"`
}

// fileSelection references a file the user added as context.
type fileSelection struct {
	URI struct {
		FsPath string `json:"fsPath"`
	} `json:"uri"`
}

// workspaceComposerData is the structure stored in per-workspace DBs under
// "composer.composerData".
type workspaceComposerData struct {
	AllComposers []composerListEntry `json:"allComposers"`
}

// composerListEntry is an item in the workspace's allComposers array.
type composerListEntry struct {
	ComposerID string `json:"composerId"`
	CreatedAt  int64  `json:"createdAt"`
}

// bubbleRole converts a Cursor bubble type to a role string.
func bubbleRole(typ int) string {
	switch typ {
	case 1:
		return "user"
	case 2:
		return "agent"
	default:
		return "agent"
	}
}

// bubbleTimestamp returns the timestamp for a bubble. It uses timingInfo if
// available, otherwise derives a synthetic timestamp from fallbackBase + index.
func bubbleTimestamp(timing *bubbleTimingInfo, fallbackBase int64, index int) int64 {
	if timing != nil && timing.ClientRpcSendTime > 0 {
		return timing.ClientRpcSendTime
	}
	return fallbackBase + int64(index)
}

// extractBubbleText returns the display text for a bubble. If text is empty
// but toolFormerData is present, it returns a formatted tool call summary.
func extractBubbleText(text string, toolFormerData json.RawMessage) string {
	text = strings.TrimSpace(text)
	if text != "" {
		return text
	}

	if len(toolFormerData) > 0 && string(toolFormerData) != "null" {
		parts := extractToolNames(toolFormerData)
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}

	return ""
}

// extractToolNames parses toolFormerData (array or single object) and returns
// formatted tool name strings.
func extractToolNames(toolFormerData json.RawMessage) []string {
	// Try array first.
	var tools []map[string]any
	if err := json.Unmarshal(toolFormerData, &tools); err == nil && len(tools) > 0 {
		parts := make([]string, 0, len(tools))
		for _, t := range tools {
			if name := toolName(t); name != "" {
				parts = append(parts, fmt.Sprintf("[tool: %s]", name))
			}
		}
		return parts
	}

	// Try single object.
	var single map[string]any
	if err := json.Unmarshal(toolFormerData, &single); err == nil {
		if name := toolName(single); name != "" {
			return []string{fmt.Sprintf("[tool: %s]", name)}
		}
	}

	return nil
}

// toolName extracts the tool name from a tool data map.
func toolName(t map[string]any) string {
	for _, key := range []string{"toolName", "name"} {
		if name, ok := t[key].(string); ok && name != "" {
			return name
		}
	}
	return ""
}

// buildEnrichedRawJSON takes a raw bubble JSON string and enriches it by
// parsing toolFormerData.rawArgs (a JSON-in-string field) and hoisting
// edit-related fields to the top level so the diff extractor can find them.
func buildEnrichedRawJSON(rawJSON string) string {
	if rawJSON == "" {
		return rawJSON
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(rawJSON), &obj); err != nil {
		return rawJSON
	}

	// Get toolFormerData — can be a single object or an array.
	toolDataList := extractToolFormerDataList(obj)
	if len(toolDataList) == 0 {
		return rawJSON
	}

	modified := false
	for _, td := range toolDataList {
		rawArgsStr, ok := td["rawArgs"].(string)
		if !ok || rawArgsStr == "" {
			continue
		}

		var args map[string]any
		if err := json.Unmarshal([]byte(rawArgsStr), &args); err != nil {
			continue
		}

		// Hoist the parsed rawArgs back into the tool data object so the
		// JSON walker can traverse it.
		td["rawArgsParsed"] = args
		modified = true

		// For search_replace: hoist file_path, old_string, new_string to top level.
		if fp, ok := args["file_path"].(string); ok && fp != "" {
			if _, hasOld := args["old_string"]; hasOld {
				if _, hasNew := args["new_string"]; hasNew {
					obj["file_path"] = fp
					obj["old_string"] = args["old_string"]
					obj["new_string"] = args["new_string"]
				}
			}
		}

		// For edit_file: hoist target_file as filePath and code_edit as content.
		if tf, ok := args["target_file"].(string); ok && tf != "" {
			if ce, ok := args["code_edit"].(string); ok && ce != "" {
				obj["filePath"] = tf
				obj["content"] = ce
			}
		}
	}

	if !modified {
		return rawJSON
	}

	enriched, err := json.Marshal(obj)
	if err != nil {
		return rawJSON
	}
	return string(enriched)
}

// extractToolFormerDataList returns toolFormerData entries as a list of maps,
// handling both array and single-object forms.
func extractToolFormerDataList(obj map[string]any) []map[string]any {
	raw, ok := obj["toolFormerData"]
	if !ok || raw == nil {
		return nil
	}

	// Already parsed as array of maps (from json.Unmarshal into map[string]any).
	if arr, ok := raw.([]any); ok {
		out := make([]map[string]any, 0, len(arr))
		for _, item := range arr {
			if m, ok := item.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}

	// Single object.
	if m, ok := raw.(map[string]any); ok {
		return []map[string]any{m}
	}

	// It might be a json.RawMessage (string). Try to parse it.
	var str string
	switch v := raw.(type) {
	case string:
		str = v
	case json.RawMessage:
		str = string(v)
	default:
		return nil
	}

	// Try array.
	var arr []map[string]any
	if err := json.Unmarshal([]byte(str), &arr); err == nil {
		return arr
	}
	// Try single object.
	var single map[string]any
	if err := json.Unmarshal([]byte(str), &single); err == nil {
		return []map[string]any{single}
	}
	return nil
}

// workspaceFolderToPath converts a workspace folder URI to a filesystem path.
// It strips the "file://" prefix and handles .code-workspace files by using
// their parent directory.
func workspaceFolderToPath(uri string) string {
	uri = strings.TrimSpace(uri)
	uri = strings.TrimPrefix(uri, "file://")
	if uri == "" {
		return ""
	}
	if strings.HasSuffix(uri, ".code-workspace") {
		return filepath.Dir(uri)
	}
	return uri
}

// interpolateTimestamps produces a timestamp for every bubble by using known
// timingInfo values as anchors and linearly interpolating between them.
// createdAt and lastUpdatedAt (ms epoch) bracket the conversation.
func interpolateTimestamps(timings []*bubbleTimingInfo, createdAt, lastUpdatedAt int64) []int64 {
	n := len(timings)
	if n == 0 {
		return nil
	}

	// Degenerate case: no valid range to interpolate within.
	if lastUpdatedAt <= createdAt || lastUpdatedAt == 0 {
		out := make([]int64, n)
		for i := range out {
			out[i] = createdAt + int64(i)
		}
		return out
	}

	// Build anchor list: (index, timestamp) pairs with known values.
	// Synthetic anchors bracket the conversation.
	type anchor struct {
		index int
		ts    int64
	}
	anchors := []anchor{{index: -1, ts: createdAt}}
	for i, t := range timings {
		if t != nil && t.ClientRpcSendTime > 0 {
			anchors = append(anchors, anchor{index: i, ts: t.ClientRpcSendTime})
		}
	}
	anchors = append(anchors, anchor{index: n, ts: lastUpdatedAt})

	// Linearly interpolate between consecutive anchors.
	out := make([]int64, n)
	for a := 0; a < len(anchors)-1; a++ {
		prev := anchors[a]
		next := anchors[a+1]
		for j := prev.index + 1; j < next.index; j++ {
			span := next.index - prev.index
			out[j] = prev.ts + (next.ts-prev.ts)*int64(j-prev.index)/int64(span)
		}
		// Fill exact anchor values (skip synthetic ones).
		if next.index >= 0 && next.index < n {
			out[next.index] = next.ts
		}
	}

	// Uniqueness pass: ensure strictly increasing.
	for i := 1; i < n; i++ {
		if out[i] <= out[i-1] {
			out[i] = out[i-1] + 1
		}
	}

	return out
}

// parseComposerData parses raw JSON into a composerData struct.
func parseComposerData(data []byte) (*composerData, error) {
	var cd composerData
	if err := json.Unmarshal(data, &cd); err != nil {
		return nil, err
	}
	return &cd, nil
}
