import Foundation
import os

private let logger = Logger(subsystem: "dev.buildermark.local", category: "ServerManager")

@MainActor
final class ServerManager: ObservableObject {
    enum Status: Equatable {
        case stopped
        case starting
        case running
        case error(String)
    }

    @Published private(set) var status: Status = .stopped

    var statusText: String {
        switch status {
        case .stopped:          return "Server Stopped"
        case .starting:         return "Server Starting\u{2026}"
        case .running:          return "Server Running"
        case .error(let msg):   return "Error: \(msg)"
        }
    }

    var statusIcon: String {
        switch status {
        case .stopped:  return "circle"
        case .starting: return "circle.dotted"
        case .running:  return "circle.fill"
        case .error:    return "exclamationmark.circle"
        }
    }

    private var process: Process?
    private var healthCheckTimer: Timer?

    init() {
        start()
    }

    func start() {
        guard process == nil else { return }

        status = .starting

        let proc = Process()

        // Look for the server binary in the app bundle first, then fall back to PATH.
        if let bundled = Bundle.main.url(forResource: "buildermark-server", withExtension: nil) {
            logger.info("Found server binary in bundle resources: \(bundled.path, privacy: .public)")
            proc.executableURL = bundled
        } else if let inMacOS = Bundle.main.executableURL?.deletingLastPathComponent()
                    .appendingPathComponent("buildermark-server"),
                  FileManager.default.isExecutableFile(atPath: inMacOS.path) {
            logger.info("Found server binary alongside executable: \(inMacOS.path, privacy: .public)")
            proc.executableURL = inMacOS
        } else if let found = findInPath("buildermark-server") {
            logger.info("Found server binary in PATH: \(found, privacy: .public)")
            proc.executableURL = URL(fileURLWithPath: found)
        } else {
            logger.error("Server binary not found in bundle, MacOS dir, or PATH")
            status = .error("Server binary not found")
            return
        }

        let dbPath = Self.resolveDBPath()
        logger.info("Using database path: \(dbPath, privacy: .public)")
        proc.arguments = ["-addr", ":7022", "-db", dbPath]

        let stdoutPipe = Pipe()
        let stderrPipe = Pipe()
        proc.standardOutput = stdoutPipe
        proc.standardError = stderrPipe

        // Log server output asynchronously.
        stdoutPipe.fileHandleForReading.readabilityHandler = { handle in
            let data = handle.availableData
            if !data.isEmpty, let line = String(data: data, encoding: .utf8) {
                logger.info("server stdout: \(line, privacy: .public)")
            }
        }
        stderrPipe.fileHandleForReading.readabilityHandler = { handle in
            let data = handle.availableData
            if !data.isEmpty, let line = String(data: data, encoding: .utf8) {
                logger.error("server stderr: \(line, privacy: .public)")
            }
        }

        proc.terminationHandler = { [weak self] terminated in
            // Clean up pipe handlers.
            stdoutPipe.fileHandleForReading.readabilityHandler = nil
            stderrPipe.fileHandleForReading.readabilityHandler = nil

            let code = terminated.terminationStatus
            logger.info("Server process exited with code \(code)")

            Task { @MainActor in
                self?.healthCheckTimer?.invalidate()
                self?.healthCheckTimer = nil
                self?.process = nil
                if code != 0 {
                    self?.status = .error("Exited (\(code))")
                } else {
                    self?.status = .stopped
                }
            }
        }

        logger.info("Launching server: \(proc.executableURL?.path ?? "nil", privacy: .public) \(proc.arguments ?? [], privacy: .public)")

        do {
            try proc.run()
            logger.info("Server process started (pid \(proc.processIdentifier))")
            process = proc
            startHealthCheck()
        } catch {
            logger.error("Failed to launch server: \(error.localizedDescription, privacy: .public)")
            status = .error(error.localizedDescription)
        }
    }

    func stop() {
        healthCheckTimer?.invalidate()
        healthCheckTimer = nil
        if let proc = process, proc.isRunning {
            proc.terminate()
        }
        process = nil
        status = .stopped
    }

    // MARK: - Health Check

    private func startHealthCheck() {
        healthCheckTimer?.invalidate()
        healthCheckTimer = Timer.scheduledTimer(withTimeInterval: 2.0, repeats: true) { [weak self] _ in
            Task { @MainActor in
                await self?.checkHealth()
            }
        }
        // Initial check after a brief startup delay.
        Task {
            try? await Task.sleep(nanoseconds: 500_000_000)
            await checkHealth()
        }
    }

    private func checkHealth() async {
        guard process != nil else { return }

        guard let url = URL(string: "http://localhost:7022/api/v1/settings") else { return }
        do {
            let (_, response) = try await URLSession.shared.data(from: url)
            if let http = response as? HTTPURLResponse, http.statusCode == 200 {
                status = .running
            }
        } catch {
            // Server might still be booting; only update if not already running.
            if status == .running {
                status = .starting
            }
        }
    }

    // MARK: - Helpers

    /// Returns the database path, honoring BUILDERMARK_LOCAL_DB_PATH if set,
    /// otherwise defaulting to ~/Library/Application Support/BuildermarkLocal/local.db.
    private static func resolveDBPath() -> String {
        if let env = ProcessInfo.processInfo.environment["BUILDERMARK_LOCAL_DB_PATH"], !env.isEmpty {
            return env
        }
        let appSupport = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first!
        return appSupport.appendingPathComponent("BuildermarkLocal/local.db").path
    }

    private func findInPath(_ binary: String) -> String? {
        let dirs = (ProcessInfo.processInfo.environment["PATH"] ?? "/usr/local/bin:/usr/bin:/bin")
            .split(separator: ":")
        for dir in dirs {
            let full = "\(dir)/\(binary)"
            if FileManager.default.isExecutableFile(atPath: full) {
                return full
            }
        }
        return nil
    }
}
