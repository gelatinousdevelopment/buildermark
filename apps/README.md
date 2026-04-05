# Buildermark — App Specifications

Platform-agnostic specification for the Buildermark native app. Use this as a reference when building for any platform. Currently implemented for macOS.

## Overview

Buildermark is a system tray / menu bar app that manages a local `buildermark-server` process. It has no main window and no dock/taskbar presence. The app runs in the background and provides a tray icon for quick access to the server and settings.

## App Lifecycle

- On launch: start the server, show the settings window, and add a tray icon
- While running: the app stays alive in the background regardless of whether any windows are open
- On quit: send SIGTERM to the server process, then exit
- Closing the settings window must NOT quit the app

## Tray Icon Menu

The tray icon uses a hammer icon and opens a dropdown menu with:

1. **Server status** — read-only label with icon (e.g. "Server Running")
2. **Open Buildermark** — opens `http://localhost:55022` in the default browser (keyboard shortcut: Cmd+O / Ctrl+O)
3. **Settings** — opens the settings window (keyboard shortcut: Cmd+, / Ctrl+,)
4. **Quit Buildermark** — stops the server and exits (keyboard shortcut: Cmd+Q / Ctrl+Q)

## Server Management

The app manages a bundled `buildermark-server` Go binary.

### Binary Resolution Order
1. App bundle resources directory
2. Alongside the app executable
3. System PATH

### Server Process
- **Port**: 55022 (hardcoded — agent plugins depend on this)
- **Launch args**: `-addr :55022 -db <db_path>`
- **Database path**: `BUILDERMARK_LOCAL_DB_PATH` env var, or platform app data directory + `Buildermark/local.db`
- **Health check**: poll `GET http://localhost:55022/api/v1/settings` every 2 seconds
- **Logging**: stdout and stderr from the server process are captured and forwarded to the platform's logging system

### Server Status States
| State | Icon | Color | Description |
|-------|------|-------|-------------|
| Stopped | empty circle | gray | Server is not running |
| Starting | dotted circle | orange | Server process launched, waiting for health check |
| Running | filled circle | green | Health check returned 200 |
| Error | exclamation circle | red | Process exited with non-zero code or failed to launch |

### Restart
The restart flow must:
1. Send SIGTERM to the old process
2. Clean up pipe/stream handlers to avoid CPU spin on EOF
3. **Wait for the old process to fully exit** before starting a new one (otherwise the port is still in use)
4. Launch the new process

### Termination
The server must be killed when the app exits, regardless of how the exit happens (quit button, system shutdown, force quit). Register for the platform's app-will-terminate event to ensure cleanup.

## Settings Window

A tabbed settings window with three tabs. The window should resize its height to fit each tab's content.

### General Tab

| Element | Type | Details |
|---------|------|---------|
| Buildermark | Link | `http://localhost:55022` — opens in browser |
| Server status | Read-only | Status icon (colored per state table above) + status text |
| Restart Server | Button | Restarts the server process |
| _spacer_ | | Visual separation before options |
| Options: Start at login | Toggle | Persisted, default `true`. Registers/unregisters with the OS login items system. Must be synced on first launch (not just on change) so the default `true` takes effect immediately. |
| Options: Enable notifications | Toggle | Persisted, default `true`. Shows notifications for new commits and completed tasks. |
| Hide menu bar icon | Toggle | Persisted, default `false`. Help text: "Relaunch the app for this to take effect" |
| _helper text_ | Caption | "When hidden, launch app to show settings." |

### Updates Tab
| Element | Type | Details |
|---------|------|---------|
| Automatically check for updates | Toggle | Bound to the update framework's auto-check setting |
| Check for Updates | Button | Triggers a manual update check. Disabled when updater is not ready |

### About Tab
Centered vertically:
- App icon (64x64)
- App name: "Buildermark"
- Version string: "Version {short_version} ({build})"
- Copyright from app metadata
- Link to https://buildermark.dev

### Persisted Preferences
| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `hideMenuBarIcon` | bool | `false` | Whether the tray icon is hidden |
| `notificationsEnabled` | bool | `true` | Whether desktop notifications are shown |
| `startAtLogin` | bool | `true` | Whether the app launches at login. Must be synced to the OS on first appearance, not just on toggle change. |

## Auto-Updates

The app should support auto-updates using a platform-appropriate mechanism:
- **macOS**: Sparkle (EdDSA-signed appcast at `https://buildermark.dev/appcast.xml`)
- **Windows**: WinSparkle
- **Linux**: AppImage self-updating

Required configuration:
- Appcast/feed URL
- EdDSA public key for signature verification (per-developer, shared across apps)

## Platform Notes

### macOS (implemented)
- Built with SwiftUI
- Uses `MenuBarExtra` for the tray icon
- Uses `SMAppService.mainApp` for login items
- Settings window opened on launch via a hidden helper window + `@Environment(\.openSettings)` (workaround for menu-bar apps not supporting programmatic settings activation)
- `applicationShouldTerminateAfterLastWindowClosed` returns `false`
- Build: `cd apps/macos && ./scripts/build.sh`

### Windows (implemented)
- Built with WPF (.NET 8) targeting Windows 10 (build 19041+) and newer
- Uses `Hardcodet.NotifyIcon.Wpf` for the system tray icon
- Uses `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` registry key for start-at-login
- Uses NetSparkle (Sparkle-compatible) for auto-updates with EdDSA signature verification
- Preferences stored in Windows Registry under `HKCU\Software\Buildermark`
- Server termination uses `Process.Kill(entireProcessTree: true)` (no SIGTERM on Windows)
- Build: `cd apps/windows && powershell -ExecutionPolicy Bypass -File scripts\build.ps1`

### Linux (not yet implemented)
- Consider GTK with `AppIndicator` / `StatusNotifierItem` for system tray
- Use `.desktop` file in `~/.config/autostart/` for start-at-login
- Bundle the server binary in the app directory
