package cli

import (
	"fmt"
	"io"

	"github.com/gelatinousdevelopment/buildermark/local/server/internal/updater"
)

// validUpdateModes lists the allowed values for Config.UpdateMode.
var validUpdateModes = map[string]bool{
	"auto":  true,
	"check": true,
	"off":   true,
}

// RunUpdateCheck checks for available updates and prints the result.
func RunUpdateCheck(w io.Writer, u updater.Updater) error {
	result, err := u.Check()
	if err != nil {
		return fmt.Errorf("checking for updates: %w", err)
	}

	if !result.HasUpdate {
		fmt.Fprintf(w, "Already up to date (version %s)\n", result.CurrentVersion)
		return nil
	}

	fmt.Fprintf(w, "Update available: %s -> %s\n", result.CurrentVersion, result.LatestVersion)
	fmt.Fprintf(w, "Run 'buildermark update apply' to install\n")
	return nil
}

// RunUpdateApply checks for and applies an available update.
func RunUpdateApply(w io.Writer, u updater.Updater) error {
	result, err := u.Check()
	if err != nil {
		return fmt.Errorf("checking for updates: %w", err)
	}
	if !result.HasUpdate {
		fmt.Fprintf(w, "Already up to date (version %s)\n", result.CurrentVersion)
		return nil
	}

	fmt.Fprintf(w, "Downloading %s...\n", result.LatestVersion)
	if err := u.Apply(result); err != nil {
		return fmt.Errorf("applying update: %w", err)
	}

	fmt.Fprintf(w, "Updated to %s\n", result.LatestVersion)
	return nil
}

// RunUpdateSetMode sets the update mode in the config.
func RunUpdateSetMode(configDir string, mode string) error {
	if !validUpdateModes[mode] {
		return fmt.Errorf("invalid update mode %q (valid: auto, check, off)", mode)
	}

	cfg, err := LoadConfig(configDir)
	if err != nil {
		return err
	}
	cfg.UpdateMode = mode
	return SaveConfig(configDir, cfg)
}
