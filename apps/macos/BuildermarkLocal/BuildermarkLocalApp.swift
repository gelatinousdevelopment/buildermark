import SwiftUI

@main
struct BuildermarkLocalApp: App {
    @StateObject private var serverManager = ServerManager()
    @StateObject private var updaterViewModel = UpdaterViewModel()

    var body: some Scene {
        MenuBarExtra {
            MenuBarMenu(serverManager: serverManager, updaterViewModel: updaterViewModel)
        } label: {
            Label("Buildermark Local", systemImage: "hammer.fill")
        }
        .menuBarExtraStyle(.menu)

        Settings {
            SettingsView()
        }
    }
}

struct MenuBarMenu: View {
    @ObservedObject var serverManager: ServerManager
    @ObservedObject var updaterViewModel: UpdaterViewModel

    var body: some View {
        Label(serverManager.statusText, systemImage: serverManager.statusIcon)

        Divider()

        Button("Open Buildermark Local") {
            if let url = URL(string: "http://localhost:7022") {
                NSWorkspace.shared.open(url)
            }
        }
        .keyboardShortcut("o")

        Divider()

        Button("Check for Updates\u{2026}") {
            updaterViewModel.checkForUpdates()
        }
        .disabled(!updaterViewModel.canCheckForUpdates)

        SettingsLink {
            Text("Settings\u{2026}")
        }
        .keyboardShortcut(",")

        Button("Quit Buildermark Local") {
            serverManager.stop()
            NSApplication.shared.terminate(nil)
        }
        .keyboardShortcut("q")
    }
}
