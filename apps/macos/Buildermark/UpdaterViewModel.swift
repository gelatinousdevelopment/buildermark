import Foundation
import Sparkle

final class UpdaterViewModel: NSObject, ObservableObject, SPUUpdaterDelegate {
    // Lazy so we can pass `self` as the delegate after NSObject.init().
    private lazy var updaterController: SPUStandardUpdaterController = {
        SPUStandardUpdaterController(
            startingUpdater: true,
            updaterDelegate: self,
            userDriverDelegate: nil
        )
    }()

    @Published var canCheckForUpdates = false
    @Published var availableVersion: String?

    override init() {
        super.init()

        // Accessing the lazy property triggers construction with self as delegate.
        updaterController.updater.publisher(for: \.canCheckForUpdates)
            .assign(to: &$canCheckForUpdates)
    }

    var automaticallyChecksForUpdates: Bool {
        get { updaterController.updater.automaticallyChecksForUpdates }
        set {
            objectWillChange.send()
            updaterController.updater.automaticallyChecksForUpdates = newValue
        }
    }

    var automaticallyDownloadsUpdates: Bool {
        get { updaterController.updater.automaticallyDownloadsUpdates }
        set {
            objectWillChange.send()
            updaterController.updater.automaticallyDownloadsUpdates = newValue
        }
    }

    func checkForUpdates() {
        updaterController.updater.checkForUpdates()
    }

    // MARK: - SPUUpdaterDelegate

    func updater(_ updater: SPUUpdater, didFindValidUpdate item: SUAppcastItem) {
        availableVersion = item.displayVersionString
    }
}
