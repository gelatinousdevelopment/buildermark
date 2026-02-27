#
# Orchestrate building the full Buildermark stack for Windows:
#   1. Svelte frontend  (local/frontend)
#   2. Go server binary  (local/server)  — embeds the frontend build
#   3. Windows app        (apps/windows)  — packages the Go binary alongside
#
# Usage:
#   powershell -ExecutionPolicy Bypass -File scripts\build-windows.ps1
#   powershell -ExecutionPolicy Bypass -File scripts\build-windows.ps1 -Runtime win-x64
#   powershell -ExecutionPolicy Bypass -File scripts\build-windows.ps1 -Runtime win-arm64
#
# Parameters / environment variables:
#   RUNTIME  - "win-x64", "win-arm64", or "all" (default: "all")
#

param(
    [string]$Runtime = $env:RUNTIME
)

$ErrorActionPreference = "Stop"

if (-not $Runtime) { $Runtime = "all" }

$RootDir = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$FrontendDir = Join-Path $RootDir "local" "frontend"
$ServerDir = Join-Path $RootDir "local" "server"
$WindowsDir = Join-Path $RootDir "apps" "windows"

$ServerBinary = "buildermark-server.exe"

# Map .NET runtime IDs to Go GOARCH values.
$GoArchMap = @{
    "win-x64"   = "amd64"
    "win-arm64" = "arm64"
}

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

function Step($message) {
    Write-Host ""
    Write-Host "==> $message" -ForegroundColor Cyan
    Write-Host ""
}

function Check-Tool($name, $installHint) {
    if (-not (Get-Command $name -ErrorAction SilentlyContinue)) {
        Write-Host "Error: $name is not installed." -ForegroundColor Red
        Write-Host "  $installHint"
        exit 1
    }
}

# ---------------------------------------------------------------------------
# Preflight
# ---------------------------------------------------------------------------

Step "Checking prerequisites"
Check-Tool "node"   "Install Node.js: https://nodejs.org/"
Check-Tool "npm"    "Install Node.js: https://nodejs.org/"
Check-Tool "go"     "Install Go: https://go.dev/dl/"
Check-Tool "dotnet" "Install .NET 8 SDK: https://dotnet.microsoft.com/download/dotnet/8.0"

# ---------------------------------------------------------------------------
# 1. Build Svelte frontend
# ---------------------------------------------------------------------------

Step "Building Svelte frontend"
Push-Location $FrontendDir
try {
    npm ci
    npm run build
} finally {
    Pop-Location
}

# Copy the full build output into the Go server's embed path so it gets
# compiled into the binary (//go:embed all:frontend in dashboard.go).
$EmbedDir = Join-Path $ServerDir "internal" "handler" "frontend"
if (Test-Path $EmbedDir) {
    Remove-Item -Recurse -Force $EmbedDir
}
Copy-Item -Recurse (Join-Path $FrontendDir "build") $EmbedDir

# ---------------------------------------------------------------------------
# Resolve target runtimes
# ---------------------------------------------------------------------------

if ($Runtime -eq "all") {
    $Runtimes = @("win-x64", "win-arm64")
} else {
    $Runtimes = @($Runtime)
}

# ---------------------------------------------------------------------------
# 2 & 3. For each runtime: build Go server, build Windows app, combine
# ---------------------------------------------------------------------------

foreach ($rid in $Runtimes) {
    $goArch = $GoArchMap[$rid]

    # -- Build Go server for this architecture --
    Step "Building Go server ($rid / GOARCH=$goArch)"
    Push-Location $ServerDir
    try {
        $env:GOOS = "windows"
        $env:GOARCH = $goArch
        $env:CGO_ENABLED = "0"
        go build -o $ServerBinary ./cmd/buildermark
    } finally {
        Pop-Location
    }

    # -- Build Windows app for this architecture --
    Step "Building Windows app ($rid)"
    & (Join-Path $WindowsDir "scripts" "build.ps1") -Runtime $rid

    # -- Copy server binary alongside the app --
    $AppDir = Join-Path $WindowsDir "build" $rid
    Copy-Item (Join-Path $ServerDir $ServerBinary) (Join-Path $AppDir $ServerBinary)

    # Clean up the server binary from the source tree
    Remove-Item (Join-Path $ServerDir $ServerBinary)

    Write-Host "  OK: $AppDir" -ForegroundColor Green
}

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------

Step "Full build complete"
foreach ($rid in $Runtimes) {
    $AppDir = Join-Path $WindowsDir "build" $rid
    Write-Host "  $rid : $AppDir"
}
