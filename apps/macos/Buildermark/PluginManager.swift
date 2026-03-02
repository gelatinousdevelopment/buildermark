import Foundation
import os

private let logger = Logger(subsystem: "dev.buildermark.local", category: "PluginManager")

@MainActor
final class PluginManager: ObservableObject {

    struct PluginInfo: Identifiable {
        let id: String
        let name: String
        /// Path relative to home directory used to detect installation.
        let detectionPath: String
        /// Files to write on install: (path relative to ~, content, isExecutable).
        let files: [(path: String, content: String, executable: Bool)]
        /// Paths relative to ~ to remove on uninstall (processed in order).
        let cleanupPaths: [String]
    }

    @Published private(set) var statuses: [String: Bool] = [:]

    let plugins: [PluginInfo]

    private let fm = FileManager.default
    private var homeDir: String { fm.homeDirectoryForCurrentUser.path }

    init() {
        plugins = Self.allPlugins()
        refresh()
    }

    func refresh() {
        var s: [String: Bool] = [:]
        for p in plugins {
            let full = (homeDir as NSString).appendingPathComponent(p.detectionPath)
            s[p.id] = fm.fileExists(atPath: full)
        }
        statuses = s
    }

    func isInstalled(_ plugin: PluginInfo) -> Bool {
        statuses[plugin.id] ?? false
    }

    func install(_ plugin: PluginInfo) {
        do {
            for file in plugin.files {
                let fullPath = (homeDir as NSString).appendingPathComponent(file.path)
                let dir = (fullPath as NSString).deletingLastPathComponent
                try fm.createDirectory(atPath: dir, withIntermediateDirectories: true)
                try file.content.write(toFile: fullPath, atomically: true, encoding: .utf8)
                if file.executable {
                    try fm.setAttributes([.posixPermissions: 0o755], ofItemAtPath: fullPath)
                }
            }
            logger.info("Installed plugin: \(plugin.id, privacy: .public)")
        } catch {
            logger.error(
                "Failed to install \(plugin.id, privacy: .public): \(error.localizedDescription, privacy: .public)"
            )
        }
        refresh()
    }

    func uninstall(_ plugin: PluginInfo) {
        do {
            for path in plugin.cleanupPaths {
                let full = (homeDir as NSString).appendingPathComponent(path)
                if fm.fileExists(atPath: full) {
                    try fm.removeItem(atPath: full)
                }
            }
            logger.info("Uninstalled plugin: \(plugin.id, privacy: .public)")
        } catch {
            logger.error(
                "Failed to uninstall \(plugin.id, privacy: .public): \(error.localizedDescription, privacy: .public)"
            )
        }
        refresh()
    }
}

// MARK: - Plugin Definitions

extension PluginManager {

    static func allPlugins() -> [PluginInfo] {
        [claudeCode(), codex(), gemini()]
    }

    // MARK: Claude Code

    private static func claudeCode() -> PluginInfo {
        let base = ".claude/plugins/buildermark"
        return PluginInfo(
            id: "claudecode",
            name: "Claude Code",
            detectionPath: "\(base)/.claude-plugin/plugin.json",
            files: [
                (
                    path: "\(base)/.claude-plugin/plugin.json",
                    content: claudeCodePluginJSON,
                    executable: false
                ),
                (
                    path: "\(base)/skills/bbrate/SKILL.md",
                    content: claudeCodeSkillMD,
                    executable: false
                ),
                (
                    path: "\(base)/skills/bbrate/scripts/submit-rating.sh",
                    content: claudeCodeScript,
                    executable: true
                ),
            ],
            cleanupPaths: [base]
        )
    }

    // MARK: Codex

    private static func codex() -> PluginInfo {
        let base = ".codex/skills/bbrate"
        return PluginInfo(
            id: "codex",
            name: "Codex CLI",
            detectionPath: "\(base)/SKILL.md",
            files: [
                (
                    path: "\(base)/SKILL.md",
                    content: codexSkillMD,
                    executable: false
                ),
                (
                    path: "\(base)/scripts/submit-rating.sh",
                    content: codexScript,
                    executable: true
                ),
            ],
            cleanupPaths: [base]
        )
    }

    // MARK: Gemini

    private static func gemini() -> PluginInfo {
        return PluginInfo(
            id: "gemini",
            name: "Gemini CLI",
            detectionPath: ".gemini/commands/bbrate.toml",
            files: [
                (
                    path: ".gemini/commands/bbrate.toml",
                    content: geminiToml,
                    executable: false
                ),
                (
                    path: ".gemini/scripts/submit-rating.sh",
                    content: geminiScript,
                    executable: true
                ),
            ],
            cleanupPaths: [
                ".gemini/commands/bbrate.toml",
                ".gemini/scripts/submit-rating.sh",
            ]
        )
    }
}

// MARK: - Embedded Plugin Content

extension PluginManager {

    // ---- Claude Code ----

    private static let claudeCodePluginJSON = #"""
    {
      "name": "Buildermark",
      "author": { "name": "Gelatinous Development Studio", "email": "root@geldev.com" },
      "version": "1.0.0",
      "description": "Rate Claude Code conversations on a 0-5 scale. Ratings are stored locally via the Buildermark Local server.",
      "skills": "./skills/"
    }
    """#

    private static let claudeCodeSkillMD = #"""
    ---
    name: bbrate
    description: Rate this Claude Code conversation (0-5 scale)
    argument-hint: "[0-5] [note]"
    allowed-tools: ["Bash"]
    ---

    The user wants to rate this conversation.

    Parse `$ARGUMENTS`:
    - If the first word is a rating (0–5), use it as the rating and treat everything after as an optional note.
    - If no rating is provided (including no args or note-only args), infer the rating (0–5) from conversation quality and treat all provided args as an optional note.

    Before submitting, review the conversation in light of the rating and optional note. Produce two sections:

    **Prompt Suggestions** — short bullet points (max 3) on how the user's prompt could have been clearer or more effective.

    **Model Failures** — short bullet points (max 3) on what the model did wrong or could have done better.

    Guidelines:
    - Weigh the rating (0–5) and optional note to calibrate your response
    - If no note is present, the rating alone implies user sentiment — infer what went wrong from the conversation context
    - If rating < 5 and no note: explain what the model should have done better
    - If rating = 5: likely no suggestions and no failures, unless you genuinely identify something worth noting
    - 0, 1, or 2 bullets per section is perfectly acceptable — do not force 3
    - A section with no bullets should say "None."
    - Keep the tone technical and dry, no personality, never snarky or arrogant

    Then run the submission script, passing your analysis text in the `ANALYSIS` environment variable:

    ```bash
    ANALYSIS="your analysis text here" bash "$HOME/.claude/plugins/buildermark/skills/bbrate/scripts/submit-rating.sh" <rating> [note...]
    ```

    If the output starts with "ok", confirm to the user: **Rated N/5** (include the note if one was given), print a clickable conversation link using the `conversation_url` value from script output, then show your analysis under `**Prompt Suggestions:**` and `**Model Failures:**` headings.

    If the output starts with "error", relay the message to the user. If it's a connection error, suggest starting the server with `cd web/server && go run .`
    """#

    private static let claudeCodeScript = #"""
    #!/usr/bin/env bash
    set -euo pipefail

    SERVER="${BUILDERMARK_LOCAL_SERVER:-http://localhost:7022}"
    DASHBOARD="${BUILDERMARK_LOCAL_DASHBOARD:-http://localhost:5173}"

    json_escape() {
      local s="${1:-}"
      s=${s//\\/\\\\}
      s=${s//\"/\\\"}
      s=${s//$'\n'/\\n}
      s=${s//$'\r'/\\r}
      s=${s//$'\t'/\\t}
      s=${s//$'\f'/\\f}
      s=${s//$'\b'/\\b}
      printf '%s' "$s"
    }

    # --- parse args ---
    if [[ $# -lt 1 ]]; then
      echo "error: no rating provided"
      echo "usage: submit-rating.sh <0-5> [note...]"
      exit 1
    fi

    rating="$1"; shift
    note="${*:-}"

    # --- validate ---
    if ! [[ "$rating" =~ ^[0-5]$ ]]; then
      echo "error: rating must be 0-5 (got '$rating')"
      exit 1
    fi

    # --- conversation ids ---
    temp_cid=$(uuidgen | tr '[:upper:]' '[:lower:]')
    canonical_cid="${CLAUDE_SESSION_ID:-}"

    # --- build JSON payload ---
    analysis="${ANALYSIS:-}"
    note_esc=$(json_escape "$note")
    analysis_esc=$(json_escape "$analysis")
    payload="{\"tempConversationId\":\"${temp_cid}\",\"rating\":${rating},\"note\":\"${note_esc}\",\"analysis\":\"${analysis_esc}\""
    if [[ -n "$canonical_cid" ]]; then
      canonical_esc=$(json_escape "$canonical_cid")
      payload="${payload},\"conversationId\":\"${canonical_esc}\""
    fi
    payload="${payload}}"

    # --- submit ---
    response=$(curl -s -X POST "${SERVER}/api/v1/rating" \
      -H 'Content-Type: application/json' \
      -d "$payload" 2>/dev/null) || {
      echo "error: could not connect to Buildermark Local server at ${SERVER}"
      echo "hint: start the server with: cd web/server && go run ."
      exit 1
    }

    # --- check response ---
    if printf '%s' "$response" | grep -q '"ok":true'; then
      conversation_url="${DASHBOARD%/}/conv/${temp_cid}"
      printf 'ok\n'
      printf 'rating: %s/5\n' "$rating"
      [[ -n "$note" ]] && printf 'note: %s\n' "$note"
      printf 'conversation: %s\n' "$temp_cid"
      printf 'conversation_url: %s\n' "$conversation_url"
    else
      echo "error: server rejected the rating"
      printf '%s\n' "$response"
      exit 1
    fi
    """#

    // ---- Codex CLI ----

    private static let codexSkillMD = #"""
    ---
    name: bbrate
    description: Rate this Codex CLI conversation (0-5 scale)
    argument-hint: "[0-5] [note]"
    allowed-tools: ["Bash"]
    ---

    The user wants to rate this conversation.

    Parse `$ARGUMENTS`:
    - If the first word is a rating (0–5), use it as the rating and treat everything after as an optional note.
    - If no rating is provided (including no args or note-only args), infer the rating (0–5) from conversation quality and treat all provided args as an optional note.

    Before submitting, review the conversation in light of the rating and optional note. Produce two sections:

    **Prompt Suggestions** — short bullet points (max 3) on how the user's prompt could have been clearer or more effective.

    **Model Failures** — short bullet points (max 3) on what the model did wrong or could have done better.

    Guidelines:
    - Weigh the rating (0–5) and optional note to calibrate your response
    - If no note is present, the rating alone implies user sentiment — infer what went wrong from the conversation context
    - If rating < 5 and no note: explain what the model should have done better
    - If rating = 5: likely no suggestions and no failures, unless you genuinely identify something worth noting
    - 0, 1, or 2 bullets per section is perfectly acceptable — do not force 3
    - A section with no bullets should say "None."
    - Keep the tone technical and dry, no personality, never snarky or arrogant

    Then run the submission script, passing your analysis text in the `ANALYSIS` environment variable:

    ```bash
    ANALYSIS="your analysis text here" bash "$HOME/.codex/skills/bbrate/scripts/submit-rating.sh" <rating> [note...]
    ```

    If the output starts with "ok", confirm to the user: **Rated N/5** (include the note if one was given), print a clickable conversation link using the `conversation_url` value from script output, then show your analysis under `**Prompt Suggestions:**` and `**Model Failures:**` headings.

    If the output starts with "error", relay the message to the user. If it's a connection error, suggest starting the server with `cd web/server && go run .`
    """#

    private static let codexScript = #"""
    #!/usr/bin/env bash
    set -euo pipefail

    SERVER="${BUILDERMARK_LOCAL_SERVER:-http://localhost:7022}"
    DASHBOARD="${BUILDERMARK_LOCAL_DASHBOARD:-http://localhost:5173}"

    json_escape() {
      local s="${1:-}"
      s=${s//\\/\\\\}
      s=${s//\"/\\\"}
      s=${s//$'\n'/\\n}
      s=${s//$'\r'/\\r}
      s=${s//$'\t'/\\t}
      s=${s//$'\f'/\\f}
      s=${s//$'\b'/\\b}
      printf '%s' "$s"
    }

    # --- parse args ---
    if [[ $# -lt 1 ]]; then
      echo "error: no rating provided"
      echo "usage: submit-rating.sh <0-5> [note...]"
      exit 1
    fi

    rating="$1"; shift
    note="${*:-}"

    # --- validate ---
    if ! [[ "$rating" =~ ^[0-5]$ ]]; then
      echo "error: rating must be 0-5 (got '$rating')"
      exit 1
    fi

    # --- conversation ids ---
    temp_cid=$(uuidgen | tr '[:upper:]' '[:lower:]')
    canonical_cid="${CODEX_THREAD_ID:-}"

    # --- build JSON payload ---
    analysis="${ANALYSIS:-}"
    note_esc=$(json_escape "$note")
    analysis_esc=$(json_escape "$analysis")
    payload="{\"tempConversationId\":\"${temp_cid}\",\"rating\":${rating},\"note\":\"${note_esc}\",\"analysis\":\"${analysis_esc}\",\"agent\":\"codex\""
    if [[ -n "$canonical_cid" ]]; then
      canonical_esc=$(json_escape "$canonical_cid")
      payload="${payload},\"conversationId\":\"${canonical_esc}\""
    fi
    payload="${payload}}"

    # --- submit ---
    response=$(curl -s -X POST "${SERVER}/api/v1/rating" \
      -H 'Content-Type: application/json' \
      -d "$payload" 2>/dev/null) || {
      echo "error: could not connect to Buildermark Local server at ${SERVER}"
      echo "hint: start the server with: cd web/server && go run ."
      exit 1
    }

    # --- check response ---
    if printf '%s' "$response" | grep -q '"ok":true'; then
      conversation_url="${DASHBOARD%/}/conv/${temp_cid}"
      printf 'ok\n'
      printf 'rating: %s/5\n' "$rating"
      [[ -n "$note" ]] && printf 'note: %s\n' "$note"
      printf 'conversation: %s\n' "$temp_cid"
      printf 'conversation_url: %s\n' "$conversation_url"
    else
      echo "error: server rejected the rating"
      printf '%s\n' "$response"
      exit 1
    fi
    """#

    // ---- Gemini CLI ----

    private static let geminiToml = #"""
    description = "Rate this Gemini CLI conversation (0-5 scale)"
    prompt = """
    The user wants to rate this conversation.

    Parse {{args}}:
    - If the first word is a rating (0-5), use it as the rating and treat everything after as an optional note.
    - If no rating is provided (including no args or note-only args), infer the rating (0-5) from conversation quality and treat all provided args as an optional note.

    Before submitting, review the conversation in light of the rating and optional note. Produce two sections:

    **Prompt Suggestions** - short bullet points (max 3) on how the user's prompt could have been clearer or more effective.

    **Model Failures** - short bullet points (max 3) on what the model did wrong or could have done better.

    Guidelines:
    - Weigh the rating (0-5) and optional note to calibrate your response
    - If no note is present, the rating alone implies user sentiment - infer what went wrong from the conversation context
    - If rating < 5 and no note: explain what the model should have done better
    - If rating = 5: likely no suggestions and no failures, unless you genuinely identify something worth noting
    - 0, 1, or 2 bullets per section is perfectly acceptable - do not force 3
    - A section with no bullets should say \"None.\"
    - Keep the tone technical and dry, no personality, never snarky or arrogant

    Then run the submission script, passing your analysis text in the ANALYSIS environment variable:

    ANALYSIS=\"your analysis text here\" bash \"$HOME/.gemini/scripts/submit-rating.sh\" <rating> [note...]

    If the output starts with \"ok\", confirm to the user: **Rated N/5** (include the note if one was given), print a clickable conversation link using the `conversation_url` value from script output, then show your analysis under `**Prompt Suggestions:**` and `**Model Failures:**` headings.

    If the output starts with \"error\", relay the message to the user. If it's a connection error, suggest starting the server with `cd web/server && go run .`.
    """
    """#

    private static let geminiScript = #"""
    #!/usr/bin/env bash
    set -euo pipefail

    SERVER="${BUILDERMARK_LOCAL_SERVER:-http://localhost:7022}"
    DASHBOARD="${BUILDERMARK_LOCAL_DASHBOARD:-http://localhost:5173}"

    json_escape() {
      local s="${1:-}"
      s=${s//\\/\\\\}
      s=${s//\"/\\\"}
      s=${s//$'\n'/\\n}
      s=${s//$'\r'/\\r}
      s=${s//$'\t'/\\t}
      s=${s//$'\f'/\\f}
      s=${s//$'\b'/\\b}
      printf '%s' "$s"
    }

    if [[ $# -lt 1 ]]; then
      echo "error: no rating provided"
      echo "usage: submit-rating.sh <0-5> [note...]"
      exit 1
    fi

    rating="$1"; shift
    note="${*:-}"

    if ! [[ "$rating" =~ ^[0-5]$ ]]; then
      echo "error: rating must be 0-5 (got '$rating')"
      exit 1
    fi

    temp_cid=$(uuidgen | tr '[:upper:]' '[:lower:]')
    canonical_cid="${GEMINI_SESSION_ID:-}"

    analysis="${ANALYSIS:-}"
    note_esc=$(json_escape "$note")
    analysis_esc=$(json_escape "$analysis")
    payload="{\"tempConversationId\":\"${temp_cid}\",\"rating\":${rating},\"note\":\"${note_esc}\",\"analysis\":\"${analysis_esc}\",\"agent\":\"gemini\""
    if [[ -n "$canonical_cid" ]]; then
      canonical_esc=$(json_escape "$canonical_cid")
      payload="${payload},\"conversationId\":\"${canonical_esc}\""
    fi
    payload="${payload}}"

    response=$(curl -s -X POST "${SERVER}/api/v1/rating" \
      -H 'Content-Type: application/json' \
      -d "$payload" 2>/dev/null) || {
      echo "error: could not connect to Buildermark Local server at ${SERVER}"
      echo "hint: start the server with: cd web/server && go run ."
      exit 1
    }

    if printf '%s' "$response" | grep -q '"ok":true'; then
      conversation_url="${DASHBOARD%/}/conv/${temp_cid}"
      printf 'ok\n'
      printf 'rating: %s/5\n' "$rating"
      [[ -n "$note" ]] && printf 'note: %s\n' "$note"
      printf 'conversation: %s\n' "$temp_cid"
      printf 'conversation_url: %s\n' "$conversation_url"
    else
      echo "error: server rejected the rating"
      printf '%s\n' "$response"
      exit 1
    fi
    """#
}
