import ServiceManagement
import SwiftUI

struct SettingsView: View {
    @ObservedObject var serverManager: ServerManager
    @ObservedObject var updaterViewModel: UpdaterViewModel

    var body: some View {
        TabView {
            GeneralTab(serverManager: serverManager)
                .tabItem { Label("General", systemImage: "gear") }

            UpdatesTab(updaterViewModel: updaterViewModel)
                .tabItem { Label("Updates", systemImage: "arrow.triangle.2.circlepath") }

            AboutTab()
                .tabItem { Label("About", systemImage: "info.circle") }
        }
        .frame(width: 380)
        .fixedSize(horizontal: false, vertical: true)
    }
}

// MARK: - General Tab

private struct GeneralTab: View {
    @ObservedObject var serverManager: ServerManager
    @AppStorage("hideMenuBarIcon") private var hideMenuBarIcon = false
    @AppStorage("startAtLogin") private var startAtLogin = true

    private static let serverURL = "http://localhost:\(ServerManager.port)"

    var body: some View {
        Form {
            LabeledContent("Buildermark:") {
                Link(Self.serverURL, destination: URL(string: Self.serverURL)!)
            }

            HStack(spacing: 4) {
                Image(systemName: serverManager.statusIcon)
                    .foregroundStyle(serverManager.statusColor)

                Text(serverManager.statusText)
            }

            Button("Restart Server") {
                serverManager.restart()
            }

            Spacer()
                .frame(height: 20)

            LabeledContent("Options:") {
                Toggle("Start at login", isOn: $startAtLogin)
                    .onAppear { syncLoginItem(startAtLogin) }
                    .onChange(of: startAtLogin) { _, enabled in syncLoginItem(enabled) }
            }

            Toggle("Hide menu bar icon", isOn: $hideMenuBarIcon)
                .help("Relaunch the app for this to take effect")

            Text("When hidden, launch app to show settings.")
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .formStyle(.columns)
        .padding(.horizontal)
        .padding(.vertical, 24)
        .frame(width: 300)
    }

    private func syncLoginItem(_ enabled: Bool) {
        do {
            if enabled {
                try SMAppService.mainApp.register()
            } else {
                try SMAppService.mainApp.unregister()
            }
        } catch {
            // Silently handle — the toggle still reflects the user's intent
        }
    }
}

// MARK: - Updates Tab

private struct UpdatesTab: View {
    @ObservedObject var updaterViewModel: UpdaterViewModel

    var body: some View {
        Form {
            Toggle(
                "Automatically check for updates",
                isOn: Binding(
                    get: { updaterViewModel.automaticallyChecksForUpdates },
                    set: { updaterViewModel.automaticallyChecksForUpdates = $0 }
                )
            )

            Spacer()
                .frame(height: 10)

            Button("Check for Updates\u{2026}") {
                updaterViewModel.checkForUpdates()
            }
            .disabled(!updaterViewModel.canCheckForUpdates)
        }
        .formStyle(.columns)
        .padding(.horizontal)
        .padding(.vertical, 24)
    }
}

// MARK: - About Tab

private struct AboutTab: View {
    private var version: String {
        let v = Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "–"
        let b = Bundle.main.infoDictionary?["CFBundleVersion"] as? String ?? "–"
        return "\(v) (\(b))"
    }

    private var copyright: String {
        Bundle.main.infoDictionary?["NSHumanReadableCopyright"] as? String
            ?? "\u{00A9} 2026 Gelatinous Development Studio"
    }

    var body: some View {
        VStack(spacing: 12) {
            Image(nsImage: NSApplication.shared.applicationIconImage)
                .resizable()
                .frame(width: 64, height: 64)

            Text("Buildermark")
                .font(.title2.bold())

            Text(verbatim: "Version \(version)")
                .foregroundStyle(.secondary)

            Text(copyright)
                .font(.caption)
                .foregroundStyle(.secondary)

            Link("https://buildermark.dev", destination: URL(string: "https://buildermark.dev")!)
        }
        .frame(maxWidth: .infinity)
        .padding(.horizontal)
        .padding(.vertical, 24)
    }
}
