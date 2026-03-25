import Combine
import SwiftUI
import UserNotifications

/// Port constant accessible from any isolation context.
private let serverPort = 55022
private let showSettingsWindowSelector = Selector(("showSettingsWindow:"))

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

        SettingsController.shared.show()
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
                DispatchQueue.main.async {
                    SettingsController.shared.suppressForNotificationClick()
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

    func application(_ application: NSApplication, open urls: [URL]) {
        Task { @MainActor in
            for url in urls {
                SettingsController.shared.handle(url: url)
            }
        }
    }
}

extension Scene {
    func backport_disableRestoration() -> some Scene {
        if #available(macOS 15.0, *) {
            return self.restorationBehavior(.disabled)
        } else {
            return self
        }
    }
}

@main
struct BuildermarkApp: App {
    @NSApplicationDelegateAdaptor(AppDelegate.self) var appDelegate
    @StateObject private var serverManager = ServerManager()
    @StateObject private var updaterViewModel = UpdaterViewModel()
    @StateObject private var settingsController = SettingsController.shared

    @AppStorage("hideMenuBarIcon") private var hideMenuBarIcon = false
    @State private var updateCancellable: AnyCancellable?

    var body: some Scene {
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
                selectedTab: $settingsController.selectedTab
            )
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
        .backport_disableRestoration()
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

@MainActor
final class SettingsController: ObservableObject {
    static let shared = SettingsController()

    @Published var selectedTab = "general"

    private var showTask: Task<Void, Never>?

    private init() {}

    func show(selecting tab: String? = nil) {
        if let tab {
            selectedTab = tab
        }

        showTask?.cancel()
        showTask = Task { @MainActor in
            NSApp.setActivationPolicy(.regular)
            try? await Task.sleep(for: .milliseconds(200))
            guard !Task.isCancelled else { return }

            NSApp.sendAction(showSettingsWindowSelector, to: nil, from: nil)
            NSApp.activate(ignoringOtherApps: true)

            try? await Task.sleep(for: .milliseconds(300))
            guard !Task.isCancelled else { return }

            settingsWindow()?.orderFrontRegardless()

            try? await Task.sleep(for: .milliseconds(300))
            guard !Task.isCancelled else { return }

            NSApp.setActivationPolicy(.accessory)

            try? await Task.sleep(for: .milliseconds(100))
            guard !Task.isCancelled else { return }

            NSApp.activate(ignoringOtherApps: true)
            settingsWindow()?.orderFrontRegardless()
        }
    }

    func handle(url: URL) {
        if url.absoluteString.contains("settings/update") {
            show(selecting: "updates")
        }
    }

    func suppressForNotificationClick() {
        showTask?.cancel()
        hideSettingsWindows()
        NSApp.setActivationPolicy(.accessory)
        NSApp.hide(nil)

        Task { @MainActor in
            try? await Task.sleep(for: .milliseconds(150))
            hideSettingsWindows()
            NSApp.setActivationPolicy(.accessory)
        }
    }

    private func settingsWindow() -> NSWindow? {
        NSApp.windows.first(where: { $0.identifier?.rawValue.contains("Settings") == true })
    }

    private func hideSettingsWindows() {
        for window in NSApp.windows where window.identifier?.rawValue.contains("Settings") == true {
            window.orderOut(nil)
        }
    }
}
