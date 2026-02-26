# Buildermark Local — Windows App

A Windows system tray application that runs and manages Buildermark Local.
The app lives in the notification area (no taskbar window) and provides quick
access to the dashboard, server status, and settings.

## Tray Menu

Right-click the tray icon to see:

| Item                      | Description                                  |
|---------------------------|----------------------------------------------|
| **Server: Running/Stopped** | Live status of the backend server            |
| **Open Buildermark Local**  | Opens http://localhost:7022 in your browser  |
| ─                         | *(divider)*                                  |
| **Settings**              | Opens a settings dialog                      |
| **Quit**                  | Stops the server and exits                   |

## Prerequisites

1. **Go 1.21+** — https://go.dev/dl/
2. **GCC toolchain** (required for CGO, which Walk and go-sqlite3 need):
   - **Recommended:** [TDM-GCC](https://jmeubank.github.io/tdm-gcc/) — easiest
     single installer, adds itself to PATH.
   - **Alternative:** [MSYS2](https://www.msys2.org/) — install the
     `mingw-w64-x86_64-gcc` package, then add
     `C:\msys64\mingw64\bin` to your PATH.
   - Verify: `gcc --version` should print a version number.
3. **rsrc** — embeds the Windows manifest into the Go binary. Installed
   automatically by `build.bat`, or manually:
   ```
   go install github.com/akavel/rsrc@latest
   ```

## Building

### Quick Build

From this directory (`apps/windows`):

```bat
build.bat
```

This installs tools, generates the resource file, resolves dependencies, and
produces `buildermark-local.exe`.

### Manual Build

```bat
:: 1. Install the rsrc tool
go install github.com/akavel/rsrc@latest

:: 2. Generate the .syso resource from the manifest
rsrc -manifest buildermark.manifest -o rsrc.syso

:: 3. Resolve Go dependencies
go mod tidy

:: 4. Build (the -H windowsgui flag hides the console window)
go build -ldflags="-H windowsgui" -o buildermark-local.exe .
```

### Build the Server

The tray app launches `buildermark-server.exe` as a child process. Build it
from the server directory:

```bat
cd ..\..\local\server
go build -o buildermark-server.exe ./cmd/buildermark
```

### Building the Frontend (SPA)

If you need a fresh frontend build to embed in the server:

```bat
cd ..\..\local\frontend
pnpm install
pnpm build
```

The static output is used by the server's embedded dashboard.

## Running

1. Place both executables in the same directory:
   ```
   buildermark-local.exe    ← tray app (this project)
   buildermark-server.exe   ← backend server
   ```
2. Double-click `buildermark-local.exe`.
3. A tray icon appears in the notification area. Right-click it to open the
   menu.

The tray app automatically starts the server on launch and stops it on quit.

## Architecture

```
buildermark-local.exe (this app)
  │
  ├─ Creates system tray icon via Walk NotifyIcon
  ├─ Launches buildermark-server.exe as a hidden child process
  ├─ Polls http://localhost:7022/api/v1/settings every 2s for status
  └─ Opens browser / settings dialog on menu clicks
```

### Technology

- **[Walk](https://github.com/lxn/walk)** — Windows Application Library Kit
  for Go. Provides native Windows UI including the system tray icon
  (`NotifyIcon`) and the settings dialog.

### Files

| File                     | Purpose                                        |
|--------------------------|------------------------------------------------|
| `main.go`                | Tray icon, menu, server management, settings   |
| `go.mod`                 | Go module definition                           |
| `buildermark.manifest`   | Windows application manifest (common controls, DPI) |
| `build.bat`              | One-step build script                          |
| `rsrc.syso`              | *(generated)* Embedded manifest resource        |

## Troubleshooting

**"Server: Stopped" but it should be running**
- Check that `buildermark-server.exe` is in the same directory as
  `buildermark-local.exe`.
- Check that port 7022 is not already in use.

**Build fails with CGO errors**
- Make sure GCC is installed and on your PATH: `gcc --version`
- Make sure `CGO_ENABLED=1` (this is the default on Windows when GCC is
  found).

**Tray icon not visible**
- Windows may hide new tray icons. Click the `^` arrow in the taskbar to
  check the overflow area.
- To always show it: Settings → Personalization → Taskbar → Other system
  tray icons → toggle Buildermark Local on.

**Walk / manifest errors**
- The `rsrc.syso` file must be present in the build directory. Re-run:
  `rsrc -manifest buildermark.manifest -o rsrc.syso`
