# Buildermark — Windows App

A Windows system tray application that manages the `buildermark-server` process. Built with WPF (.NET 8). Supports Windows 10 (build 19041+) and newer.

## Prerequisites

- [.NET 8 SDK](https://dotnet.microsoft.com/download/dotnet/8.0) (includes the `dotnet` CLI)
- LLVM MinGW (provides x64 and ARM64 cross-compilers for CGO) — install via `winget install -e --id MartinStorsjo.LLVM-MinGW.UCRT` (restart your shell after installing)
- Windows 10 (version 2004 / build 19041) or newer

## Project Structure

```
apps/windows/
├── Buildermark.sln              # Visual Studio solution
├── Buildermark/
│   ├── Buildermark.csproj       # Project file (NuGet deps, build config)
│   ├── App.xaml / App.xaml.cs   # App entry point, tray icon setup
│   ├── SettingsWindow.xaml/cs   # Tabbed settings window (General, Updates, About)
│   ├── ServerManager.cs         # Manages the buildermark-server process
│   ├── UpdaterManager.cs        # Auto-update via NetSparkle (Sparkle-compatible)
│   ├── PreferencesManager.cs    # Persists settings in Windows Registry
│   └── Resources/
│       └── buildermark.ico      # App/tray icon (placeholder — replace with real icon)
├── scripts/
│   └── build.ps1                # Command-line build script
└── README.md
```

## Building

### Command Line (recommended for CI)

```powershell
cd apps\windows

# Build both x64 and ARM64 (default)
powershell -ExecutionPolicy Bypass -File scripts\build.ps1

# Build only x64
powershell -ExecutionPolicy Bypass -File scripts\build.ps1 -Runtime win-x64

# Build only ARM64
powershell -ExecutionPolicy Bypass -File scripts\build.ps1 -Runtime win-arm64
```

Build output is organized by architecture:

```
build/
├── win-x64/Buildermark.exe
└── win-arm64/Buildermark.exe
```

### Visual Studio

1. Open `Buildermark.sln` in Visual Studio 2022+
2. Set the configuration to **Release**
3. Build > Publish, or just Build > Build Solution

### Build Parameters

| Variable | Default | Description |
|----------|---------|-------------|
| `CONFIGURATION` | `Release` | Build configuration |
| `RUNTIME` | `all` | `win-x64`, `win-arm64`, or `all` (builds both) |

## Running

1. Build `buildermark-server` (the Go server from `local/server/`) for Windows:
   ```powershell
   cd local\server
   # For x64
   $env:GOOS="windows"; $env:GOARCH="amd64"; go build -o buildermark-server.exe .
   # For ARM64
   $env:GOOS="windows"; $env:GOARCH="arm64"; go build -o buildermark-server.exe .
   ```
2. Place the matching `buildermark-server.exe` alongside each `Buildermark.exe` in the build output directory
3. Run `Buildermark.exe`

The app will:
- Start the server on port 55022
- Show a system tray icon
- Open the Settings window

## Server Binary Resolution

The app looks for `buildermark-server.exe` in this order:
1. Same directory as `Buildermark.exe`
2. System `PATH`

## Preferences

Preferences are stored in the Windows Registry under `HKCU\Software\Buildermark`:

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `hideMenuBarIcon` | DWORD | `0` | Hide the system tray icon |
| `notificationsEnabled` | DWORD | `1` | Show notifications for new commits and completed tasks |
| `startAtLogin` | DWORD | `1` | Launch at Windows startup |

Start-at-login is implemented via the standard `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` registry key.

## Auto-Updates

Uses [NetSparkle](https://github.com/NetSparkleUpdater/NetSparkle), the .NET equivalent of Sparkle/WinSparkle. Reads the same appcast feed (`https://buildermark.dev/appcast.xml`) with EdDSA signature verification.

## Replacing the Icon

The placeholder icon at `Buildermark/Resources/buildermark.ico` should be replaced with the real app icon. The `.ico` file should include at least 16x16, 32x32, 48x48, and 256x256 sizes.

## Distribution

For distribution, the `build.ps1` script produces a self-contained single-file `.exe`. For a more polished installer experience, consider packaging with:
- [Inno Setup](https://jrsoftware.org/isinfo.php) — free, widely used
- [WiX Toolset](https://wixtoolset.org/) — MSI-based, integrates with CI
- [MSIX](https://learn.microsoft.com/en-us/windows/msix/) — modern Windows packaging format
