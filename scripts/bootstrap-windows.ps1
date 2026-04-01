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

function Get-ExecutableCommandPath($commandInfo) {
    if (-not $commandInfo) {
        return $null
    }

    if ($commandInfo.Path) {
        return [string]$commandInfo.Path
    }

    if ($commandInfo.Definition) {
        return [string]$commandInfo.Definition
    }

    if ($commandInfo.Source) {
        return [string]$commandInfo.Source
    }

    return $null
}

function Get-ExistingFilePath($paths) {
    foreach ($path in $paths) {
        if (-not $path) {
            continue
        }

        $candidate = [string]$path
        if (Test-Path -LiteralPath $candidate -PathType Leaf) {
            return (Get-Item -LiteralPath $candidate).FullName
        }
    }

    return $null
}

function Resolve-ToolPath($commandName, $fallbackPaths) {
    $directPath = Get-ExistingFilePath $fallbackPaths
    if ($directPath) {
        return $directPath
    }

    $command = Get-Command $commandName -ErrorAction SilentlyContinue
    if ($command) {
        $commandPath = Get-ExecutableCommandPath $command
        $existingCommandPath = Get-ExistingFilePath @($commandPath)
        if ($existingCommandPath) {
            return $existingCommandPath
        }
    }

    return $null
}

# ---------------------------------------------------------------------------
# System prerequisites
# ---------------------------------------------------------------------------

Step "Checking system prerequisites"

Check-Tool "node"   "Install Node.js: https://nodejs.org/"
Check-Tool "npm"    "Included with Node.js: https://nodejs.org/"
Check-Tool "go"     "Install Go: https://go.dev/dl/"
Check-Tool "x86_64-w64-mingw32-gcc" "Install LLVM MinGW: winget install -e --id MartinStorsjo.LLVM-MinGW.UCRT"
Check-Tool "aarch64-w64-mingw32-gcc" "Install LLVM MinGW: winget install -e --id MartinStorsjo.LLVM-MinGW.UCRT"
Check-Tool "dotnet" "Install .NET 8 SDK: https://dotnet.microsoft.com/download/dotnet/8.0"

$IsccPath = Resolve-ToolPath "ISCC.exe" @(
    (Join-Path ${env:ProgramFiles(x86)} "Inno Setup 6\ISCC.exe"),
    (Join-Path $env:LOCALAPPDATA "Programs\Inno Setup 6\ISCC.exe")
)
if (-not $IsccPath) {
    Write-Host "Error: ISCC.exe is not installed." -ForegroundColor Red
    Write-Host "  Install Inno Setup 6: https://jrsoftware.org/isinfo.php"
    exit 1
}

$SignToolCandidates = @()
foreach ($kitRoot in @(
    (Join-Path ${env:ProgramFiles(x86)} "Windows Kits\10\bin"),
    (Join-Path $env:ProgramFiles "Windows Kits\10\bin")
)) {
    if (-not $kitRoot -or -not (Test-Path $kitRoot)) {
        continue
    }

    $SignToolCandidates += Get-ChildItem -Path $kitRoot -Directory -ErrorAction SilentlyContinue |
        Sort-Object Name -Descending |
        ForEach-Object { Join-Path $_.FullName "x64\signtool.exe" } |
        Where-Object { Test-Path $_ }
}

$SignToolPath = Resolve-ToolPath "signtool.exe" $SignToolCandidates
if (-not $SignToolPath) {
    Write-Host "Error: signtool.exe is not installed." -ForegroundColor Red
    Write-Host "  Install the Windows SDK / Signing Tools for Desktop Apps."
    exit 1
}

Write-Host "  Node.js: $(node --version)"
Write-Host "  npm:     $(npm --version)"
Write-Host "  Go:      $(go version)"
Write-Host "  x64 CC:  $(x86_64-w64-mingw32-gcc --version | Select-Object -First 1)"
Write-Host "  arm64 CC:$(aarch64-w64-mingw32-gcc --version | Select-Object -First 1)"
Write-Host "  .NET:    $(dotnet --version)"
Write-Host "  ISCC:    $IsccPath"
Write-Host "  SignTool:$SignToolPath"

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
Write-Host "  Set signing env vars before a release build:"
Write-Host "    WINDOWS_SIGN_CERT_FILE or WINDOWS_SIGN_CERT_THUMBPRINT"
Write-Host "    WINDOWS_SIGN_CERT_PASSWORD (only for PFX files)"
Write-Host "    WINDOWS_SIGN_TIMESTAMP_URL (optional; defaults to RFC3161 DigiCert)"
Write-Host ""
Write-Host "  To build everything:  powershell -File scripts\build-windows.ps1"
Write-Host "  To run the server:    cd local\server && go run ./cmd/buildermark"
Write-Host "  To run the frontend:  cd local\frontend && npm run dev"
