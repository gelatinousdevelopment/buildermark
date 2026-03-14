//go:build cli

package updater

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const (
	defaultUpdateURL = "https://buildermark.dev/api/v1/releases/latest"
)

// cliUpdater implements Updater for CLI builds.
type cliUpdater struct {
	version   string
	updateURL string
	client    *http.Client
}

func getUpdater(version string) Updater {
	return &cliUpdater{
		version:   version,
		updateURL: defaultUpdateURL,
		client:    &http.Client{Timeout: 15 * time.Second},
	}
}

type releaseResponse struct {
	Version     string `json:"version"`
	DownloadURL string `json:"downloadUrl"`
}

func (u *cliUpdater) Check() (*UpdateResult, error) {
	url := fmt.Sprintf("%s?os=%s&arch=%s&current=%s", u.updateURL, runtime.GOOS, runtime.GOARCH, u.version)
	resp, err := u.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("checking for updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update server returned status %d", resp.StatusCode)
	}

	var release releaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("parsing update response: %w", err)
	}

	result := &UpdateResult{
		CurrentVersion: u.version,
		LatestVersion:  release.Version,
		DownloadURL:    release.DownloadURL,
		HasUpdate:      release.Version != u.version && release.Version != "",
	}
	return result, nil
}

func (u *cliUpdater) Apply(result *UpdateResult) error {
	if result == nil || result.DownloadURL == "" {
		return fmt.Errorf("no download URL in update result")
	}

	// Write pre-update version marker so the next startup can detect the update.
	if home, err := os.UserHomeDir(); err == nil {
		markerDir := filepath.Join(home, ".buildermark")
		os.MkdirAll(markerDir, 0o755)
		os.WriteFile(filepath.Join(markerDir, "pre-update-version"), []byte(u.version), 0o644)
	}

	// Download to a temp file in the same directory as the current binary.
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding current executable: %w", err)
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		return fmt.Errorf("resolving executable path: %w", err)
	}

	dir := filepath.Dir(exe)
	tmpFile, err := os.CreateTemp(dir, "buildermark-update-*")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath) // clean up on failure

	resp, err := u.client.Get(result.DownloadURL)
	if err != nil {
		tmpFile.Close()
		return fmt.Errorf("downloading update: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		tmpFile.Close()
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		return fmt.Errorf("writing update: %w", err)
	}
	tmpFile.Close()

	// Make executable.
	if err := os.Chmod(tmpPath, 0o755); err != nil {
		return fmt.Errorf("setting permissions: %w", err)
	}

	// Atomic swap: rename temp file over the current binary.
	if err := os.Rename(tmpPath, exe); err != nil {
		return fmt.Errorf("replacing binary: %w", err)
	}

	return nil
}
