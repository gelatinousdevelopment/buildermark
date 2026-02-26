//go:build windows

package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

func main() {
	a := &app{}
	if err := a.run(); err != nil {
		log.Fatal(err)
	}
}

type app struct {
	mw         *walk.MainWindow
	ni         *walk.NotifyIcon
	statusItem *walk.Action
	server     serverProcess
}

type serverProcess struct {
	cmd     *exec.Cmd
	running bool
	mu      sync.Mutex
}

func (a *app) run() error {
	// Create an invisible main window. Walk requires a top-level form to own
	// the NotifyIcon; we never show this window so the app is tray-only.
	if err := (MainWindow{
		AssignTo: &a.mw,
		Visible:  false,
	}).Create(); err != nil {
		return fmt.Errorf("create main window: %w", err)
	}

	var err error
	a.ni, err = walk.NewNotifyIcon(a.mw)
	if err != nil {
		return fmt.Errorf("create notify icon: %w", err)
	}
	defer a.ni.Dispose()

	if err := a.ni.SetIcon(walk.IconApplication()); err != nil {
		return fmt.Errorf("set icon: %w", err)
	}
	if err := a.ni.SetToolTip("Buildermark Local"); err != nil {
		return fmt.Errorf("set tooltip: %w", err)
	}

	a.buildMenu()

	if err := a.ni.SetVisible(true); err != nil {
		return fmt.Errorf("show tray icon: %w", err)
	}

	a.startServer()
	defer a.stopServer()

	go a.monitorStatus()

	// Run the Windows message loop (blocks until exit).
	a.mw.Run()
	return nil
}

// buildMenu populates the tray icon's right-click context menu.
func (a *app) buildMenu() {
	menu := a.ni.ContextMenu()

	// Status indicator (disabled — display only).
	a.statusItem = walk.NewAction()
	a.statusItem.SetText("Server: Starting...")
	a.statusItem.SetEnabled(false)
	menu.Actions().Add(a.statusItem)

	// Open dashboard in browser.
	openAction := walk.NewAction()
	openAction.SetText("Open Buildermark Local")
	openAction.Triggered().Attach(func() {
		openURL("http://localhost:7022")
	})
	menu.Actions().Add(openAction)

	menu.Actions().Add(walk.NewSeparatorAction())

	// Settings dialog.
	settingsAction := walk.NewAction()
	settingsAction.SetText("Settings")
	settingsAction.Triggered().Attach(func() {
		a.showSettings()
	})
	menu.Actions().Add(settingsAction)

	// Quit — stop server and exit.
	quitAction := walk.NewAction()
	quitAction.SetText("Quit")
	quitAction.Triggered().Attach(func() {
		a.stopServer()
		walk.App().Exit(0)
	})
	menu.Actions().Add(quitAction)
}

// showSettings opens a modal dialog with a link to buildermark.dev.
func (a *app) showSettings() {
	var dlg *walk.Dialog

	Dialog{
		AssignTo: &dlg,
		Title:    "Settings \u2014 Buildermark Local",
		MinSize:  Size{Width: 380, Height: 220},
		Layout:   VBox{Margins: Margins{Left: 20, Top: 20, Right: 20, Bottom: 20}},
		Children: []Widget{
			Label{
				Text: "Buildermark Local",
				Font: Font{PointSize: 14, Bold: true},
			},
			VSpacer{Size: 10},
			Label{Text: "For documentation and updates, visit:"},
			VSpacer{Size: 5},
			PushButton{
				Text: "Open buildermark.dev",
				OnClicked: func() {
					openURL("https://buildermark.dev")
				},
			},
			VSpacer{},
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					HSpacer{},
					PushButton{
						Text:      "Close",
						OnClicked: func() { dlg.Accept() },
					},
				},
			},
		},
	}.Run(a.mw)
}

// ---------------------------------------------------------------------------
// Server lifecycle
// ---------------------------------------------------------------------------

// startServer launches buildermark-server.exe as a hidden child process.
func (a *app) startServer() {
	a.server.mu.Lock()
	defer a.server.mu.Unlock()

	exePath, err := os.Executable()
	if err != nil {
		log.Printf("cannot determine executable path: %v", err)
		return
	}
	dir := filepath.Dir(exePath)
	serverBin := filepath.Join(dir, "buildermark-server.exe")

	if _, err := os.Stat(serverBin); err != nil {
		log.Printf("server binary not found: %s", serverBin)
		return
	}

	a.server.cmd = exec.Command(serverBin)
	a.server.cmd.Dir = dir
	a.server.cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	if err := a.server.cmd.Start(); err != nil {
		log.Printf("failed to start server: %v", err)
		return
	}
	a.server.running = true

	go func() {
		a.server.cmd.Wait()
		a.server.mu.Lock()
		a.server.running = false
		a.server.mu.Unlock()
	}()
}

// stopServer terminates the child server process if it is still running.
func (a *app) stopServer() {
	a.server.mu.Lock()
	defer a.server.mu.Unlock()

	if a.server.cmd != nil && a.server.cmd.Process != nil && a.server.running {
		a.server.cmd.Process.Kill()
		a.server.running = false
	}
}

// monitorStatus polls the server health endpoint every 2 seconds and
// updates the tray menu status text.
func (a *app) monitorStatus() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		status := "Stopped"
		if checkHealth() {
			status = "Running"
		}
		a.mw.Synchronize(func() {
			a.statusItem.SetText("Server: " + status)
		})
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func checkHealth() bool {
	client := http.Client{Timeout: time.Second}
	resp, err := client.Get("http://localhost:7022/api/v1/settings")
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func openURL(url string) {
	exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}
