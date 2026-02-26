//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/lxn/walk"
)

const (
	defaultServerExecutable = "buildermark-server.exe"
	localURL                = "http://localhost:7022"
)

type app struct {
	mw           *walk.MainWindow
	statusAction *walk.Action

	mu        sync.Mutex
	serverCmd *exec.Cmd
}

func main() {
	if err := run(); err != nil {
		walk.MsgBox(nil, "Buildermark Local", fmt.Sprintf("Failed to start: %v", err), walk.MsgBoxIconError)
		os.Exit(1)
	}
}

func run() error {
	mw, err := walk.NewMainWindow()
	if err != nil {
		return err
	}
	defer mw.Dispose()

	mw.SetVisible(false)
	mw.SetTitle("Buildermark Local")

	icon, err := walk.NewIconFromSysDLLWithSize("imageres", 2, 16)
	if err != nil {
		return err
	}

	ni, err := walk.NewNotifyIcon(mw)
	if err != nil {
		return err
	}
	defer ni.Dispose()

	ni.SetIcon(icon)
	ni.SetToolTip("Buildermark Local")

	statusAction := walk.NewAction()
	statusAction.SetText("Status: Stopped")
	statusAction.SetEnabled(false)
	if err := ni.ContextMenu().Actions().Add(statusAction); err != nil {
		return err
	}

	openAction := walk.NewAction()
	openAction.SetText("Open Buildermark Local")
	openAction.Triggered().Attach(func() {
		_ = openURL(localURL)
	})
	if err := ni.ContextMenu().Actions().Add(openAction); err != nil {
		return err
	}

	if err := ni.ContextMenu().Actions().Add(walk.NewSeparatorAction()); err != nil {
		return err
	}

	settingsAction := walk.NewAction()
	settingsAction.SetText("Settings")
	settingsAction.Triggered().Attach(func() {
		_ = showSettingsWindow(mw)
	})
	if err := ni.ContextMenu().Actions().Add(settingsAction); err != nil {
		return err
	}

	a := &app{mw: mw, statusAction: statusAction}

	quitAction := walk.NewAction()
	quitAction.SetText("Quit")
	quitAction.Triggered().Attach(func() {
		a.stopServer()
		walk.App().Exit(0)
	})
	if err := ni.ContextMenu().Actions().Add(quitAction); err != nil {
		return err
	}

	if err := ni.SetVisible(true); err != nil {
		return err
	}

	if err := a.startServer(); err != nil {
		a.setStatus(fmt.Sprintf("Status: Error (%v)", err))
	}

	mw.Run()
	return nil
}

func showSettingsWindow(owner walk.Form) error {
	dlg, err := walk.NewDialogWithFixedSize(owner)
	if err != nil {
		return err
	}
	defer dlg.Dispose()

	dlg.SetTitle("Buildermark Local Settings")
	dlg.SetSize(walk.Size{Width: 360, Height: 120})

	layout := walk.NewVBoxLayout()
	layout.SetMargins(walk.Margins{HNear: 16, VNear: 16, HFar: 16, VFar: 16})
	layout.SetSpacing(10)
	if err := dlg.SetLayout(layout); err != nil {
		return err
	}

	label, err := walk.NewTextLabel(dlg)
	if err != nil {
		return err
	}
	label.SetText("Buildermark website:")

	link, err := walk.NewLinkLabel(dlg)
	if err != nil {
		return err
	}
	link.SetText(`<a href="https://buildermark.dev">buildermark.dev</a>`)
	link.LinkActivated().Attach(func(link *walk.LinkLabelLink) {
		_ = openURL(link.URL())
	})

	closeButton, err := walk.NewPushButton(dlg)
	if err != nil {
		return err
	}
	closeButton.SetText("Close")
	closeButton.Clicked().Attach(func() {
		dlg.Accept()
	})

	dlg.Run()
	return nil
}

func (a *app) startServer() error {
	serverPath, err := resolveServerPath()
	if err != nil {
		return err
	}

	cmd := exec.Command(serverPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return err
	}

	a.mu.Lock()
	a.serverCmd = cmd
	a.mu.Unlock()

	a.setStatus("Status: Running")

	go func() {
		err := cmd.Wait()
		if err == nil {
			a.setStatus("Status: Stopped")
			return
		}

		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			a.setStatus(fmt.Sprintf("Status: Exited (%d)", exitErr.ExitCode()))
			return
		}

		a.setStatus(fmt.Sprintf("Status: Error (%v)", err))
	}()

	return nil
}

func (a *app) stopServer() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.serverCmd == nil || a.serverCmd.Process == nil {
		return
	}

	_ = a.serverCmd.Process.Kill()
}

func (a *app) setStatus(text string) {
	a.mw.Synchronize(func() {
		a.statusAction.SetText(text)
	})
}

func resolveServerPath() (string, error) {
	if customPath := os.Getenv("BUILDERMARK_SERVER_PATH"); customPath != "" {
		return customPath, nil
	}

	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}

	serverPath := filepath.Join(filepath.Dir(exePath), defaultServerExecutable)
	if _, err := os.Stat(serverPath); err != nil {
		return "", fmt.Errorf("server executable not found at %q", serverPath)
	}

	return serverPath, nil
}

func openURL(url string) error {
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}
