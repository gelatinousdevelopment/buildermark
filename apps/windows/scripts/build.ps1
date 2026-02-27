#
# Build the Buildermark Windows app for x64 and ARM64.
#
# Prerequisites:
#   - .NET 8 SDK (https://dotnet.microsoft.com/download/dotnet/8.0)
#
# Usage:
#   powershell -ExecutionPolicy Bypass -File scripts\build.ps1
#   powershell -ExecutionPolicy Bypass -File scripts\build.ps1 -Runtime win-x64
#   powershell -ExecutionPolicy Bypass -File scripts\build.ps1 -Runtime win-arm64
#
# Parameters / environment variables (override defaults):
#   CONFIGURATION  - Build configuration (default: "Release")
#   RUNTIME        - Target runtime: "win-x64", "win-arm64", or "all" (default: "all")
#

param(
    [string]$Configuration = $env:CONFIGURATION,
    [string]$Runtime = $env:RUNTIME
)

$ErrorActionPreference = "Stop"

if (-not $Configuration) { $Configuration = "Release" }
if (-not $Runtime) { $Runtime = "all" }

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectDir = Split-Path -Parent $ScriptDir
$BuildDir = Join-Path $ProjectDir "build"
$CsprojPath = Join-Path (Join-Path $ProjectDir "Buildermark") "Buildermark.csproj"

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

function Build-Runtime($rid) {
    $PublishDir = Join-Path $BuildDir $rid

    Step "Restoring NuGet packages ($rid)"
    dotnet restore $CsprojPath --runtime $rid

    Step "Publishing $Configuration ($rid)"
    dotnet publish $CsprojPath `
        --configuration $Configuration `
        --runtime $rid `
        --self-contained true `
        --output $PublishDir `
        -p:PublishSingleFile=true `
        -p:IncludeNativeLibrariesForSelfExtract=true

    $ExePath = Join-Path $PublishDir "Buildermark.exe"

    if (-not (Test-Path $ExePath)) {
        Write-Host "Error: published executable not found at $ExePath" -ForegroundColor Red
        exit 1
    }

    Write-Host "  OK: $ExePath" -ForegroundColor Green
}

# ---------------------------------------------------------------------------
# Preflight
# ---------------------------------------------------------------------------

Step "Checking prerequisites"
Check-Tool "dotnet" "Install .NET 8 SDK: https://dotnet.microsoft.com/download/dotnet/8.0"

$dotnetVersion = dotnet --version
Write-Host "  .NET SDK: $dotnetVersion"

# ---------------------------------------------------------------------------
# Resolve target runtimes
# ---------------------------------------------------------------------------

if ($Runtime -eq "all") {
    $Runtimes = @("win-x64", "win-arm64")
} else {
    $Runtimes = @($Runtime)
}

# ---------------------------------------------------------------------------
# Clean
# ---------------------------------------------------------------------------

Step "Cleaning previous build"
# Stop any running instances that may lock files in the build directory
Get-Process -Name "Buildermark", "buildermark-server" -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
# Clean obj to avoid stale XAML cache
$ObjDir = Join-Path (Join-Path $ProjectDir "Buildermark") "obj"
if (Test-Path $ObjDir) {
    Remove-Item -Recurse -Force $ObjDir
}
# Clean only the runtime-specific build subdirectories, not the entire build dir
foreach ($rid in $Runtimes) {
    $RidDir = Join-Path $BuildDir $rid
    if (Test-Path $RidDir) {
        Remove-Item -Recurse -Force $RidDir
    }
}
New-Item -ItemType Directory -Path $BuildDir -Force | Out-Null

# ---------------------------------------------------------------------------
# Build
# ---------------------------------------------------------------------------

foreach ($rid in $Runtimes) {
    Build-Runtime $rid
}

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------

Step "Build complete"
foreach ($rid in $Runtimes) {
    $ExePath = Join-Path (Join-Path $BuildDir $rid) "Buildermark.exe"
    Write-Host "  $rid : $ExePath"
}
Write-Host ""
Write-Host "  Place buildermark-server.exe alongside each Buildermark.exe before running."
