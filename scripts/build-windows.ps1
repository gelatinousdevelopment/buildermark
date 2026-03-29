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
$FrontendDir = Join-Path (Join-Path $RootDir "local") "frontend"
$ServerDir = Join-Path (Join-Path $RootDir "local") "server"
$WindowsDir = Join-Path (Join-Path $RootDir "apps") "windows"

$ServerBinary = "buildermark-server.exe"

# Map .NET runtime IDs to Go GOARCH values.
$GoArchMap = @{
    "win-x64"   = "amd64"
    "win-arm64" = "arm64"
}

# Map .NET runtime IDs to the C cross-compiler needed for CGO.
$CcMap = @{
    "win-x64"   = "x86_64-w64-mingw32-gcc"
    "win-arm64" = "aarch64-w64-mingw32-gcc"
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
Check-Tool "x86_64-w64-mingw32-gcc"   "Install LLVM MinGW: winget install -e --id MartinStorsjo.LLVM-MinGW.UCRT"
Check-Tool "aarch64-w64-mingw32-gcc" "Install LLVM MinGW: winget install -e --id MartinStorsjo.LLVM-MinGW.UCRT"
Check-Tool "dotnet" "Install .NET SDK: https://dotnet.microsoft.com/download"

# ---------------------------------------------------------------------------
# 1. Build Svelte frontend
# ---------------------------------------------------------------------------

Step "Building Svelte frontend"
Push-Location $FrontendDir
try {
    npm ci
    if ($LASTEXITCODE -ne 0) { throw "npm ci failed (exit code $LASTEXITCODE)" }
    npm run build
    if ($LASTEXITCODE -ne 0) { throw "npm run build failed (exit code $LASTEXITCODE)" }
} finally {
    Pop-Location
}

# Copy the full build output into the Go server's embed path so it gets
# compiled into the binary (//go:embed all:frontend in dashboard.go).
$EmbedDir = Join-Path (Join-Path (Join-Path $ServerDir "internal") "handler") "frontend"
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
        $env:CGO_ENABLED = "1"
        $env:CC = $CcMap[$rid]
        go build -o $ServerBinary ./cmd/buildermark
        if ($LASTEXITCODE -ne 0) { throw "go build failed for $rid (exit code $LASTEXITCODE)" }
    } finally {
        Pop-Location
    }

    # -- Build Windows app for this architecture --
    Step "Building Windows app ($rid)"
    & (Join-Path (Join-Path $WindowsDir "scripts") "build.ps1") -Runtime $rid
    if ($LASTEXITCODE -ne 0) { throw "Windows app build failed for $rid (exit code $LASTEXITCODE)" }

    # -- Copy server binary alongside the app --
    $AppDir = Join-Path (Join-Path $WindowsDir "build") $rid
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
    $AppDir = Join-Path (Join-Path $WindowsDir "build") $rid
    Write-Host "  $rid : $AppDir"
}
