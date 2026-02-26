# Buildermark Local Windows App (Codex)

This folder contains a Windows tray application that launches Buildermark Local's Go server binary and provides a tray menu.

## What it does

- Runs as a tray-only app (no taskbar app window).
- Starts `buildermark-server.exe` automatically.
- Tray menu items:
  - `Status: ...` (first item, read-only)
  - `Open Buildermark Local` (opens http://localhost:7022)
  - separator
  - `Settings` (opens a small settings window)
  - `Quit` (stops server process and exits)
- Settings window currently includes a link to <https://buildermark.dev>.

## Tech stack

- Language: Go
- Windows UI/tray library: [`github.com/lxn/walk`](https://github.com/lxn/walk)

## Prerequisites

On a Windows machine:

1. Install Go (same major/minor used by this repo; currently Go 1.24.x).
2. Ensure this repository is checked out locally.

## Build steps (Windows)

From repository root:

```powershell
# 1) Build the Buildermark server binary expected by the tray app
cd local/server
go build -o ..\..\apps\windows_codex\dist\buildermark-server.exe ./cmd/buildermark

# 2) Build the tray app
cd ..\..\apps\windows_codex
go build -o dist\buildermark-local.exe .
```

After building, `apps/windows_codex/dist` should contain:

- `buildermark-local.exe`
- `buildermark-server.exe`

Keep both files in the same folder.

## Run

```powershell
cd apps\windows_codex\dist
.\buildermark-local.exe
```

You should see a tray icon. Use `Open Buildermark Local` to launch the app in your browser.

## Optional: custom server path

By default, the app looks for `buildermark-server.exe` in the same folder as `buildermark-local.exe`.

You can override this by setting:

- `BUILDERMARK_SERVER_PATH`

Example:

```powershell
$env:BUILDERMARK_SERVER_PATH = "C:\path\to\buildermark-server.exe"
.\buildermark-local.exe
```

## Distribution checklist

For a simple zip distribution:

1. Build both `.exe` files into `dist`.
2. Include this README (or a trimmed copy for end users).
3. Zip `dist` and share.

For installer-based distribution, you can use common Windows installer tools (e.g. Inno Setup, WiX) and install both binaries together.
