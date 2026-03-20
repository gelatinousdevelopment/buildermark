#
# Build the Buildermark Windows app (.NET Framework 4.8, AnyCPU).
#
# Prerequisites:
#   - .NET SDK (any recent version with SDK-style project support)
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
Add-Type -AssemblyName System.Drawing

if (-not $Configuration) { $Configuration = "Release" }
if (-not $Runtime) { $Runtime = "all" }

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectDir = Split-Path -Parent $ScriptDir
$BuildDir = Join-Path $ProjectDir "build"
$CsprojPath = Join-Path (Join-Path $ProjectDir "Buildermark") "Buildermark.csproj"
$TrayIconSourcePath = Join-Path $ProjectDir "tray-icon-128.png"
$LargeIconSourcePath = Join-Path $ProjectDir "app-icon-256.png"
$IconOutputPath = Join-Path (Join-Path $ProjectDir "Buildermark") "Resources\buildermark.ico"

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

    Step "Restoring NuGet packages"
    dotnet restore $CsprojPath

    Step "Publishing $Configuration (AnyCPU -> $rid)"
    dotnet publish $CsprojPath `
        --configuration $Configuration `
        --output $PublishDir

    $ExePath = Join-Path $PublishDir "Buildermark.exe"

    if (-not (Test-Path $ExePath)) {
        Write-Host "Error: published executable not found at $ExePath" -ForegroundColor Red
        exit 1
    }

    Write-Host "  OK: $ExePath" -ForegroundColor Green
}

function Assert-FileExists($path, $description) {
    if (-not (Test-Path $path)) {
        Write-Host "Error: missing $description at $path" -ForegroundColor Red
        exit 1
    }
}

function New-ResizedPngBytes($sourcePath, $size) {
    # Build the resize script as a string so that System.Drawing type references
    # are resolved at invocation time (after Add-Type), not at parse time.
    $script = @"
param(`$srcPath, `$sz)
`$sourceImage = [System.Drawing.Image]::FromFile(`$srcPath)
try {
    `$bitmap = New-Object System.Drawing.Bitmap `$sz, `$sz, ([System.Drawing.Imaging.PixelFormat]::Format32bppArgb)
    try {
        `$bitmap.SetResolution(`$sourceImage.HorizontalResolution, `$sourceImage.VerticalResolution)
        `$graphics = [System.Drawing.Graphics]::FromImage(`$bitmap)
        try {
            `$graphics.Clear([System.Drawing.Color]::Transparent)
            `$graphics.InterpolationMode = [System.Drawing.Drawing2D.InterpolationMode]::HighQualityBicubic
            `$graphics.SmoothingMode = [System.Drawing.Drawing2D.SmoothingMode]::HighQuality
            `$graphics.PixelOffsetMode = [System.Drawing.Drawing2D.PixelOffsetMode]::HighQuality
            `$graphics.CompositingQuality = [System.Drawing.Drawing2D.CompositingQuality]::HighQuality
            `$graphics.DrawImage(`$sourceImage, 0, 0, `$sz, `$sz)
        } finally {
            `$graphics.Dispose()
        }
        `$memoryStream = New-Object System.IO.MemoryStream
        try {
            `$bitmap.Save(`$memoryStream, [System.Drawing.Imaging.ImageFormat]::Png)
            return `$memoryStream.ToArray()
        } finally {
            `$memoryStream.Dispose()
        }
    } finally {
        `$bitmap.Dispose()
    }
} finally {
    `$sourceImage.Dispose()
}
"@
    $block = [ScriptBlock]::Create($script)
    return & $block $sourcePath $size
}

function New-IconEntry($sourcePath, $size) {
    return [PSCustomObject]@{
        Size = $size
        Bytes = New-ResizedPngBytes -sourcePath $sourcePath -size $size
    }
}

function Write-IcoFile($outputPath, $entries) {
    $outputDir = Split-Path -Parent $outputPath
    New-Item -ItemType Directory -Path $outputDir -Force | Out-Null

    $fileStream = [System.IO.File]::Open($outputPath, [System.IO.FileMode]::Create, [System.IO.FileAccess]::Write)
    try {
        $writer = New-Object System.IO.BinaryWriter $fileStream
        try {
            $writer.Write([UInt16]0)
            $writer.Write([UInt16]1)
            $writer.Write([UInt16]$entries.Count)

            $offset = 6 + (16 * $entries.Count)
            foreach ($entry in $entries) {
                $dimension = if ($entry.Size -ge 256) { [byte]0 } else { [byte]$entry.Size }
                $writer.Write([byte]$dimension)
                $writer.Write([byte]$dimension)
                $writer.Write([byte]0)
                $writer.Write([byte]0)
                $writer.Write([UInt16]1)
                $writer.Write([UInt16]32)
                $writer.Write([UInt32]$entry.Bytes.Length)
                $writer.Write([UInt32]$offset)
                $offset += $entry.Bytes.Length
            }

            foreach ($entry in $entries) {
                $bytes = [byte[]]$entry.Bytes
                $writer.Write($bytes, 0, $bytes.Length)
            }
        } finally {
            $writer.Dispose()
        }
    } finally {
        $fileStream.Dispose()
    }
}

function Update-AppIcon() {
    Step "Generating Windows app icon"

    Assert-FileExists -path $TrayIconSourcePath -description "tray icon source"
    Assert-FileExists -path $LargeIconSourcePath -description "large app icon source"

    $entries = @(
        (New-IconEntry -sourcePath $LargeIconSourcePath -size 16)
        (New-IconEntry -sourcePath $LargeIconSourcePath -size 24)
        (New-IconEntry -sourcePath $LargeIconSourcePath -size 32)
        (New-IconEntry -sourcePath $LargeIconSourcePath -size 48)
        (New-IconEntry -sourcePath $LargeIconSourcePath -size 64)
        (New-IconEntry -sourcePath $LargeIconSourcePath -size 128)
        (New-IconEntry -sourcePath $LargeIconSourcePath -size 256)
    )

    Write-IcoFile -outputPath $IconOutputPath -entries $entries
    Write-Host "  OK: $IconOutputPath" -ForegroundColor Green
}

# ---------------------------------------------------------------------------
# Preflight
# ---------------------------------------------------------------------------

Step "Checking prerequisites"
Check-Tool "dotnet" "Install .NET SDK: https://dotnet.microsoft.com/download"

$dotnetVersion = dotnet --version
Write-Host "  .NET SDK: $dotnetVersion"

Update-AppIcon

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
# Clean obj to avoid stale build cache
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
