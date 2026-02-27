#
# Build the Buildermark Windows app.
#
# Prerequisites:
#   - .NET 8 SDK (https://dotnet.microsoft.com/download/dotnet/8.0)
#
# Usage:
#   powershell -ExecutionPolicy Bypass -File scripts\build.ps1
#
# Environment variables (override defaults):
#   CONFIGURATION  - Build configuration (default: "Release")
#   RUNTIME        - Target runtime (default: "win-x64")
#

param(
    [string]$Configuration = $env:CONFIGURATION,
    [string]$Runtime = $env:RUNTIME
)

$ErrorActionPreference = "Stop"

if (-not $Configuration) { $Configuration = "Release" }
if (-not $Runtime) { $Runtime = "win-x64" }

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectDir = Split-Path -Parent $ScriptDir
$BuildDir = Join-Path $ProjectDir "build"
$PublishDir = Join-Path $BuildDir "publish"
$CsprojPath = Join-Path $ProjectDir "Buildermark" "Buildermark.csproj"

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
Check-Tool "dotnet" "Install .NET 8 SDK: https://dotnet.microsoft.com/download/dotnet/8.0"

$dotnetVersion = dotnet --version
Write-Host "  .NET SDK: $dotnetVersion"

# ---------------------------------------------------------------------------
# Clean
# ---------------------------------------------------------------------------

Step "Cleaning previous build"
if (Test-Path $BuildDir) {
    Remove-Item -Recurse -Force $BuildDir
}
New-Item -ItemType Directory -Path $BuildDir -Force | Out-Null

# ---------------------------------------------------------------------------
# Restore
# ---------------------------------------------------------------------------

Step "Restoring NuGet packages"
dotnet restore $CsprojPath --runtime $Runtime

# ---------------------------------------------------------------------------
# Publish
# ---------------------------------------------------------------------------

Step "Publishing $Configuration ($Runtime)"
dotnet publish $CsprojPath `
    --configuration $Configuration `
    --runtime $Runtime `
    --self-contained true `
    --output $PublishDir `
    -p:PublishSingleFile=true `
    -p:IncludeNativeLibrariesForSelfExtract=true

$ExePath = Join-Path $PublishDir "Buildermark.exe"

if (-not (Test-Path $ExePath)) {
    Write-Host "Error: published executable not found at $ExePath" -ForegroundColor Red
    exit 1
}

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------

Step "Build complete"
Write-Host "  Executable: $ExePath"
Write-Host "  Output dir: $PublishDir"
Write-Host ""
Write-Host "  To run: $ExePath"
Write-Host "  Place buildermark-server.exe alongside Buildermark.exe before running."
