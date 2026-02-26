import Foundation

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
            proc.executableURL = bundled
        } else if let inMacOS = Bundle.main.executableURL?.deletingLastPathComponent()
                    .appendingPathComponent("buildermark-server"),
                  FileManager.default.isExecutableFile(atPath: inMacOS.path) {
            proc.executableURL = inMacOS
        } else if let found = findInPath("buildermark-server") {
            proc.executableURL = URL(fileURLWithPath: found)
        } else {
            status = .error("Server binary not found")
            return
        }

        proc.arguments = ["-addr", ":7022"]
        proc.standardOutput = FileHandle.nullDevice
        proc.standardError = FileHandle.nullDevice

        proc.terminationHandler = { [weak self] terminated in
            Task { @MainActor in
                self?.healthCheckTimer?.invalidate()
                self?.healthCheckTimer = nil
                self?.process = nil
                if terminated.terminationStatus != 0 {
                    self?.status = .error("Exited (\(terminated.terminationStatus))")
                } else {
                    self?.status = .stopped
                }
            }
        }

        do {
            try proc.run()
            process = proc
            startHealthCheck()
        } catch {
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
