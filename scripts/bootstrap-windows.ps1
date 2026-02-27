#
# Bootstrap the Buildermark development environment on Windows.
# Installs project dependencies and verifies prerequisites.
#
# Usage:
#   powershell -ExecutionPolicy Bypass -File scripts\bootstrap-windows.ps1
#

$ErrorActionPreference = "Stop"

$RootDir = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$FrontendDir = Join-Path (Join-Path $RootDir "local") "frontend"
$ServerDir = Join-Path (Join-Path $RootDir "local") "server"

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
# System prerequisites
# ---------------------------------------------------------------------------

Step "Checking system prerequisites"

Check-Tool "node"   "Install Node.js: https://nodejs.org/"
Check-Tool "npm"    "Included with Node.js: https://nodejs.org/"
Check-Tool "go"     "Install Go: https://go.dev/dl/"
Check-Tool "gcc"    "Install a C compiler (e.g. MSYS2 mingw-w64-ucrt-x86_64-gcc): https://www.msys2.org/"
Check-Tool "dotnet" "Install .NET 8 SDK: https://dotnet.microsoft.com/download/dotnet/8.0"

Write-Host "  Node.js: $(node --version)"
Write-Host "  npm:     $(npm --version)"
Write-Host "  Go:      $(go version)"
Write-Host "  GCC:     $(gcc --version | Select-Object -First 1)"
Write-Host "  .NET:    $(dotnet --version)"

# ---------------------------------------------------------------------------
# Frontend dependencies
# ---------------------------------------------------------------------------

Step "Installing frontend dependencies"
Push-Location $FrontendDir
try {
    npm install
} finally {
    Pop-Location
}

# ---------------------------------------------------------------------------
# Go dependencies
# ---------------------------------------------------------------------------

Step "Downloading Go modules"
Push-Location $ServerDir
try {
    go mod download
} finally {
    Pop-Location
}

# ---------------------------------------------------------------------------
# .NET dependencies
# ---------------------------------------------------------------------------

Step "Restoring .NET packages"
$CsprojPath = Join-Path (Join-Path (Join-Path (Join-Path $RootDir "apps") "windows") "Buildermark") "Buildermark.csproj"
dotnet restore $CsprojPath

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------

Step "Bootstrap complete"
Write-Host "  Frontend deps: $FrontendDir\node_modules"
Write-Host "  Go modules:    $(go env GOMODCACHE)"
Write-Host "  .NET packages:  restored"
Write-Host ""
Write-Host "  To build everything:  powershell -File scripts\build-windows.ps1"
Write-Host "  To run the server:    cd local\server && go run ./cmd/buildermark"
Write-Host "  To run the frontend:  cd local\frontend && npm run dev"
