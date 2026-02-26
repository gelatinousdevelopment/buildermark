import SwiftUI

@main
struct BuildermarkLocalApp: App {
    @StateObject private var serverManager = ServerManager()

    var body: some Scene {
        MenuBarExtra {
            MenuBarMenu(serverManager: serverManager)
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
