//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

// version is set at build time via:
//
//	go build -ldflags="-X main.version=1.0.0"
var version = "dev"

// updateURL points to a JSON endpoint that returns the latest release info.
// Expected format: {"version":"1.2.0","url":"https://...exe","notes":"..."}
const updateURL = "https://buildermark.dev/api/releases/windows/latest"

type releaseInfo struct {
	Version string `json:"version"`
	URL     string `json:"url"`
	Notes   string `json:"notes"`
}

// startUpdateChecker cleans up after a previous update and begins periodic
// background checks.
func (a *app) startUpdateChecker() {
	cleanOldBinary()

	// Check shortly after startup (let the UI settle first).
	go func() {
		time.Sleep(10 * time.Second)
		a.checkForUpdate(false)
	}()

	// Then check every 6 hours.
	go func() {
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			a.checkForUpdate(false)
		}
	}()
}

// checkForUpdate fetches the latest release info and prompts the user if a
// newer version is available. When manual is true (triggered by the menu),
// it always shows feedback even if already up-to-date.
func (a *app) checkForUpdate(manual bool) {
	release, err := fetchLatestRelease()
	if err != nil {
		if manual {
			a.mw.Synchronize(func() {
				walk.MsgBox(a.mw, "Check for Updates",
					"Could not check for updates:\n"+err.Error(),
					walk.MsgBoxIconError)
			})
		}
		return
	}

	if !isNewer(release.Version, version) {
		if manual {
			a.mw.Synchronize(func() {
				walk.MsgBox(a.mw, "Check for Updates",
					fmt.Sprintf("You're up to date! (v%s)", version),
					walk.MsgBoxIconInformation)
			})
		}
		return
	}

	a.mw.Synchronize(func() {
		a.promptUpdate(release)
	})
}

// promptUpdate shows a dialog with release notes and Update / Skip buttons.
func (a *app) promptUpdate(release *releaseInfo) {
	var dlg *walk.Dialog
	var accepted bool

	notes := release.Notes
	if notes == "" {
		notes = "A new version is available."
	}

	Dialog{
		AssignTo: &dlg,
		Title:    "Update Available \u2014 Buildermark Local",
		MinSize:  Size{Width: 440, Height: 300},
		Layout:   VBox{Margins: Margins{Left: 20, Top: 20, Right: 20, Bottom: 20}},
		Children: []Widget{
			Label{
				Text: fmt.Sprintf("Buildermark Local v%s is available!", release.Version),
				Font: Font{PointSize: 12, Bold: true},
			},
			VSpacer{Size: 5},
			Label{
				Text: fmt.Sprintf("You are currently running v%s.", version),
			},
			VSpacer{Size: 10},
			Label{Text: "Release notes:"},
			TextEdit{
				Text:     notes,
				ReadOnly: true,
				VScroll:  true,
			},
			VSpacer{Size: 10},
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					HSpacer{},
					PushButton{
						Text:      "Skip",
						OnClicked: func() { dlg.Accept() },
					},
					PushButton{
						Text: "Update and Restart",
						OnClicked: func() {
							accepted = true
							dlg.Accept()
						},
					},
				},
			},
		},
	}.Run(a.mw)

	if accepted {
		a.applyUpdate(release)
	}
}

// applyUpdate downloads the new binary, swaps it with the running one using
// the Windows rename-in-place trick, and restarts the application.
//
// Windows allows renaming an in-use .exe but not overwriting it, so the
// sequence is: rename current → .old, rename downloaded → current, relaunch.
func (a *app) applyUpdate(release *releaseInfo) {
	exePath, err := os.Executable()
	if err != nil {
		walk.MsgBox(a.mw, "Update Failed",
			"Cannot determine executable path:\n"+err.Error(),
			walk.MsgBoxIconError)
		return
	}

	dir := filepath.Dir(exePath)
	tmpPath := filepath.Join(dir, "buildermark-local.exe.new")

	if err := downloadFile(release.URL, tmpPath); err != nil {
		os.Remove(tmpPath)
		walk.MsgBox(a.mw, "Update Failed",
			"Download failed:\n"+err.Error(),
			walk.MsgBoxIconError)
		return
	}

	// Swap: current → .old, new → current.
	oldPath := exePath + ".old"
	os.Remove(oldPath) // remove stale .old from a prior update

	if err := os.Rename(exePath, oldPath); err != nil {
		os.Remove(tmpPath)
		walk.MsgBox(a.mw, "Update Failed",
			"Cannot rename current binary:\n"+err.Error(),
			walk.MsgBoxIconError)
		return
	}

	if err := os.Rename(tmpPath, exePath); err != nil {
		// Attempt to restore the original.
		os.Rename(oldPath, exePath)
		walk.MsgBox(a.mw, "Update Failed",
			"Cannot install new binary:\n"+err.Error(),
			walk.MsgBoxIconError)
		return
	}

	// Restart: launch the new binary and exit.
	a.stopServer()

	cmd := exec.Command(exePath)
	cmd.Dir = dir
	cmd.Start()

	walk.App().Exit(0)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func fetchLatestRelease() (*releaseInfo, error) {
	client := http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(updateURL)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var release releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("invalid response: %w", err)
	}
	return &release, nil
}

func downloadFile(url, dest string) error {
	client := http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}

// isNewer returns true if remote is a higher semver than local.
func isNewer(remote, local string) bool {
	if local == "dev" {
		return false // never update a dev build
	}
	rp := parseVersion(remote)
	lp := parseVersion(local)
	for i := 0; i < 3; i++ {
		if rp[i] > lp[i] {
			return true
		}
		if rp[i] < lp[i] {
			return false
		}
	}
	return false
}

func parseVersion(v string) [3]int {
	v = strings.TrimPrefix(v, "v")
	parts := strings.SplitN(v, ".", 3)
	var out [3]int
	for i, p := range parts {
		if i >= 3 {
			break
		}
		fmt.Sscanf(p, "%d", &out[i])
	}
	return out
}

// cleanOldBinary removes a leftover .old file from a previous update.
func cleanOldBinary() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	os.Remove(exe + ".old")
}
