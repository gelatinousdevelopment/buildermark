import Foundation
import os

private let logger = Logger(subsystem: "dev.buildermark.local", category: "PluginManager")

@MainActor
final class PluginManager: ObservableObject {

    struct PluginFile {
        /// Path inside the bundle's `plugins/` resource folder.
        let bundlePath: String
        /// Destination path relative to the home directory.
        let installPath: String
        let executable: Bool
        /// Text replacements applied when copying (source → installed path rewriting).
        let replacements: [(old: String, new: String)]
    }

    struct PluginInfo: Identifiable {
        let id: String
        let name: String
        /// Path relative to home directory used to detect installation.
        let detectionPath: String
        /// Files to copy from the bundle on install.
        let files: [PluginFile]
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
                guard let content = Self.bundleContent(at: file.bundlePath) else {
                    logger.error("Missing bundle resource: \(file.bundlePath, privacy: .public)")
                    continue
                }

                var text = content
                for r in file.replacements {
                    text = text.replacingOccurrences(of: r.old, with: r.new)
                }

                let fullPath = (homeDir as NSString).appendingPathComponent(file.installPath)
                let dir = (fullPath as NSString).deletingLastPathComponent
                try fm.createDirectory(atPath: dir, withIntermediateDirectories: true)
                try text.write(toFile: fullPath, atomically: true, encoding: .utf8)
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

    /// Read a file from the bundled `plugins` folder resource.
    private static func bundleContent(at relativePath: String) -> String? {
        guard
            let url = Bundle.main.resourceURL?
                .appendingPathComponent("plugins")
                .appendingPathComponent(relativePath)
        else { return nil }
        return try? String(contentsOf: url, encoding: .utf8)
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
                PluginFile(
                    bundlePath: "claudecode/.claude-plugin/plugin.json",
                    installPath: "\(base)/.claude-plugin/plugin.json",
                    executable: false,
                    replacements: []
                ),
                PluginFile(
                    bundlePath: "claudecode/skills/bbrate/SKILL.md",
                    installPath: "\(base)/skills/bbrate/SKILL.md",
                    executable: false,
                    replacements: [
                        (
                            #""$(git rev-parse --show-toplevel)/plugins/claudecode/skills/bbrate/scripts/submit-rating.sh""#,
                            #""$HOME/.claude/plugins/buildermark/skills/bbrate/scripts/submit-rating.sh""#
                        ),
                    ]
                ),
                PluginFile(
                    bundlePath: "claudecode/skills/bbrate/scripts/submit-rating.sh",
                    installPath: "\(base)/skills/bbrate/scripts/submit-rating.sh",
                    executable: true,
                    replacements: []
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
                PluginFile(
                    bundlePath: "codex/skills/bbrate/SKILL.md",
                    installPath: "\(base)/SKILL.md",
                    executable: false,
                    replacements: [
                        (
                            "bash plugins/codex/skills/bbrate/scripts/submit-rating.sh",
                            #"bash "$HOME/.codex/skills/bbrate/scripts/submit-rating.sh""#
                        ),
                    ]
                ),
                PluginFile(
                    bundlePath: "codex/skills/bbrate/scripts/submit-rating.sh",
                    installPath: "\(base)/scripts/submit-rating.sh",
                    executable: true,
                    replacements: []
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
                PluginFile(
                    bundlePath: "gemini/commands/bbrate.toml",
                    installPath: ".gemini/commands/bbrate.toml",
                    executable: false,
                    replacements: [
                        (
                            "bash plugins/gemini/scripts/submit-rating.sh",
                            #"bash \"$HOME/.gemini/scripts/submit-rating.sh\""#
                        ),
                    ]
                ),
                PluginFile(
                    bundlePath: "gemini/scripts/submit-rating.sh",
                    installPath: ".gemini/scripts/submit-rating.sh",
                    executable: true,
                    replacements: []
                ),
            ],
            cleanupPaths: [
                ".gemini/commands/bbrate.toml",
                ".gemini/scripts/submit-rating.sh",
            ]
        )
    }
}
