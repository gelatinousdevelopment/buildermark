import Combine
import SwiftUI
import UserNotifications

/// Port constant accessible from any isolation context.
private let serverPort = 55022

@MainActor
class AppDelegate: NSObject, NSApplicationDelegate, UNUserNotificationCenterDelegate {
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        false
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        let center = UNUserNotificationCenter.current()
        center.delegate = self
        center.requestAuthorization(options: [.alert, .sound]) { _, _ in }

        // Detect post-update: compare current version with last known version.
        let currentVersion = Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? ""
        let lastKnownVersion = UserDefaults.standard.string(forKey: "lastKnownVersion") ?? ""
        if !lastKnownVersion.isEmpty && lastKnownVersion != currentVersion {
            UserDefaults.standard.set(lastKnownVersion, forKey: "previousVersion")
        }
        UserDefaults.standard.set(currentVersion, forKey: "lastKnownVersion")
    }

    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        didReceive response: UNNotificationResponse,
        withCompletionHandler completionHandler: @escaping () -> Void
    ) {
        if let urlPath = response.notification.request.content.userInfo["url"] as? String {
            let base = "http://localhost:\(serverPort)"
            if let url = URL(string: base + urlPath) {
                NSWorkspace.shared.open(url)
                // macOS activates the app when a notification is clicked, which
                // surfaces the Settings window. Revert to accessory policy and
                // hide the app so only the browser opens.
                DispatchQueue.main.async {
                    NSApp.setActivationPolicy(.accessory)
                    NSApp.hide(nil)
                }
            }
        }
        completionHandler()
    }

    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification,
        withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void
    ) {
        completionHandler([.banner, .sound])
    }
}

@main
struct BuildermarkApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) var appDelegate
    @StateObject private var serverManager = ServerManager()
    @StateObject private var updaterViewModel = UpdaterViewModel()

    @AppStorage("hideMenuBarIcon") private var hideMenuBarIcon = false
    @State private var selectedSettingsTab = "general"
    @State private var updateCancellable: AnyCancellable?

    var body: some Scene {
        // Invisible helper window — provides the SwiftUI environment
        // needed to call openSettings() on launch. Must be declared
        // before the Settings scene.
        Window("", id: "launcher") {
            SettingsLauncher()
        }
        .windowResizability(.contentSize)
        .defaultSize(width: 1, height: 1)
        .windowStyle(.hiddenTitleBar)

        MenuBarExtra(
            isInserted: Binding(
                get: { !hideMenuBarIcon },
                set: { hideMenuBarIcon = !$0 }
            )
        ) {
            MenuBarMenu(serverManager: serverManager, updaterViewModel: updaterViewModel)
        } label: {
            Label {
                Text("Buildermark")
            } icon: {
                Image("buildermark")
                    .renderingMode(.template)
                    .resizable()
                    .scaledToFit()
                    .frame(width: 18, height: 18)
            }
        }
        .menuBarExtraStyle(.menu)

        Settings {
            SettingsView(
                serverManager: serverManager,
                updaterViewModel: updaterViewModel,
                selectedTab: $selectedSettingsTab
            )
            .onOpenURL { url in
                handleURL(url)
            }
            .task {
                // When Sparkle finds an available update, notify the server via WS.
                updateCancellable = updaterViewModel.$availableVersion
                    .compactMap { $0 }
                    .sink { version in
                        Task { @MainActor in
                            serverManager.sendUpdateStatus(state: "available", version: version)
                        }
                    }
            }
        }
    }

    @MainActor
    private func handleURL(_ url: URL) {
        if url.absoluteString.contains("settings/update") {
            selectedSettingsTab = "updates"
            if let w = NSApp.windows.first(where: {
                $0.identifier?.rawValue.contains("Settings") == true
            }) {
                w.orderFrontRegardless()
            }
            NSApp.setActivationPolicy(.regular)
            NSApp.activate(ignoringOtherApps: true)
        }
    }
}

/// Opens the Settings window once on launch, then closes itself.
private struct SettingsLauncher: View {
    @Environment(\.openSettings) private var openSettings

    var body: some View {
        Color.clear
            .frame(width: 1, height: 1)
            .task {
                // Menu-bar apps use .accessory policy which blocks window activation.
                // Temporarily switch to .regular so the settings window can appear.
                NSApp.setActivationPolicy(.regular)
                try? await Task.sleep(for: .milliseconds(200))

                openSettings()
                NSApp.activate(ignoringOtherApps: true)

                // Bring the settings window to front
                try? await Task.sleep(for: .milliseconds(300))
                if let w = NSApp.windows.first(where: {
                    $0.identifier?.rawValue.contains("Settings") == true
                }) {
                    w.orderFrontRegardless()
                }

                // Revert to menu-bar-only and close this helper window
                try? await Task.sleep(for: .milliseconds(300))
                NSApp.setActivationPolicy(.accessory)

                // Switching to .accessory resigns active status. Re-activate
                // and bring the settings window forward again.
                try? await Task.sleep(for: .milliseconds(100))
                NSApp.activate(ignoringOtherApps: true)
                if let w = NSApp.windows.first(where: {
                    $0.identifier?.rawValue.contains("Settings") == true
                }) {
                    w.orderFrontRegardless()
                }

                for w in NSApp.windows where w.title.isEmpty && w.identifier?.rawValue == "launcher"
                {
                    w.close()
                }
            }
    }
}

struct MenuBarMenu: View {
    @ObservedObject var serverManager: ServerManager
    @ObservedObject var updaterViewModel: UpdaterViewModel
    @Environment(\.openSettings) private var openSettings

    var body: some View {
        Label(serverManager.statusText, systemImage: serverManager.statusIcon)

        Divider()

        Button("Open Buildermark") {
            if let url = URL(string: "http://localhost:\(ServerManager.port)") {
                NSWorkspace.shared.open(url)
            }
        }
        .keyboardShortcut("o")

        Divider()

        Button("Settings\u{2026}") {
            openSettings()
            NSApp.activate(ignoringOtherApps: true)
        }
        .keyboardShortcut(",")

        Button("Quit Buildermark") {
            serverManager.stop()
            NSApplication.shared.terminate(nil)
        }
        .keyboardShortcut("q")
    }
}
