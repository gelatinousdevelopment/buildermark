package updater

// UpdateResult holds information about an available update.
type UpdateResult struct {
	CurrentVersion string
	LatestVersion  string
	DownloadURL    string
	HasUpdate      bool
}

// Updater checks for and applies updates.
type Updater interface {
	// Check checks for available updates.
	Check() (*UpdateResult, error)
	// Apply downloads and installs the update described by result.
	Apply(result *UpdateResult) error
}

// GetUpdater returns an Updater implementation for the current build.
// The real implementation is in updater_cli.go (build tag: cli).
// The stub is in updater_stub.go (build tag: !cli).
func GetUpdater(version string) Updater {
	return getUpdater(version)
}
