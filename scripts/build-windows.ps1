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
#   powershell -ExecutionPolicy Bypass -File scripts\build-windows.ps1 -SelfSign
#
# Parameters / environment variables:
#   RUNTIME                     - "win-x64", "win-arm64", or "all" (default: "all")
#   CONFIGURATION               - .NET publish configuration (default: "Release")
#   SELF_SIGN                   - set to "1" to generate/use a self-signed dev certificate
#   WINDOWS_SKIP_SIGNING        - set to "1" to skip all signing for dev builds
#   WINDOWS_SIGNTOOL_PATH       - override path to signtool.exe
#   WINDOWS_INNO_COMPILER_PATH  - override path to ISCC.exe
#   WINDOWS_SIGN_TIMESTAMP_URL  - RFC3161 timestamp URL
#                                 (default: Azure TSA in Azure mode, DigiCert otherwise)
#
# Azure Artifact Signing (preferred for release builds):
#   AZURE_ARTIFACT_SIGNING_ACCOUNT_NAME              - signing account name
#   AZURE_ARTIFACT_SIGNING_CERTIFICATE_PROFILE_NAME  - certificate profile name
#   AZURE_ARTIFACT_SIGNING_ENDPOINT                  - regional endpoint URL
#   AZURE_ARTIFACT_SIGNING_CORRELATION_ID            - optional correlation ID
#   AZURE_ARTIFACT_SIGNING_EXCLUDE_CREDENTIALS       - comma-separated DefaultAzureCredential
#                                                      sources to exclude (e.g. "ManagedIdentity")
#   AZURE_ARTIFACT_SIGNING_METADATA_PATH             - path to a pre-built metadata JSON
#                                                      (overrides the auto-generated one)
#   AZURE_ARTIFACT_SIGNING_DLIB_PATH                 - path to Azure.CodeSigning.Dlib.dll
#   AZURE_TENANT_ID / AZURE_CLIENT_ID / AZURE_CLIENT_SECRET - service principal auth
#     (consumed directly by the Azure signing dlib)
#
# Legacy cert-file / thumbprint signing (dev / fallback):
#   WINDOWS_SIGN_CERT_FILE      - path to a PFX file for signtool
#   WINDOWS_SIGN_CERT_PASSWORD  - password for WINDOWS_SIGN_CERT_FILE
#   WINDOWS_SIGN_CERT_THUMBPRINT- certificate thumbprint to use from a cert store
#   WINDOWS_SIGN_CERT_STORE     - certificate store name when using thumbprint (default: "My")
#   WINDOWS_SIGN_CERT_MACHINE_STORE - set to "1" to use the machine cert store
#

param(
    [string]$Runtime = $env:RUNTIME,
    [string]$Configuration = $env:CONFIGURATION,
    [switch]$SelfSign,
    [switch]$SkipSigning
)

$ErrorActionPreference = "Stop"

if (-not $Runtime) { $Runtime = "all" }
if (-not $Configuration) { $Configuration = "Release" }
if (-not $PSBoundParameters.ContainsKey("SelfSign") -and $env:SELF_SIGN -eq "1") { $SelfSign = $true }
if (-not $PSBoundParameters.ContainsKey("SkipSigning") -and $env:WINDOWS_SKIP_SIGNING -eq "1") { $SkipSigning = $true }

$RootDir = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
$FrontendDir = Join-Path (Join-Path $RootDir "local") "frontend"
$ServerDir = Join-Path (Join-Path $RootDir "local") "server"
$WindowsDir = Join-Path (Join-Path $RootDir "apps") "windows"
$WindowsProjectDir = Join-Path $WindowsDir "Buildermark"
$WindowsBuildScript = Join-Path (Join-Path $WindowsDir "scripts") "build.ps1"
$InnoScriptPath = Join-Path $WindowsDir "Buildermark.iss"
$CsprojPath = Join-Path $WindowsProjectDir "Buildermark.csproj"
$AppName = "Buildermark"

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

$InstallerArchMap = @{
    "win-x64"   = "x64compatible"
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

function Assert-PathExists($path, $description) {
    if (-not (Test-Path $path)) {
        Write-Host "Error: missing $description at $path" -ForegroundColor Red
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

function Resolve-IsccPath() {
    if ($env:WINDOWS_INNO_COMPILER_PATH -and (Test-Path $env:WINDOWS_INNO_COMPILER_PATH)) {
        return [string]$env:WINDOWS_INNO_COMPILER_PATH
    }

    $directPath = Get-ExistingFilePath @(
        (Join-Path ${env:ProgramFiles(x86)} "Inno Setup 6\ISCC.exe"),
        (Join-Path $env:LOCALAPPDATA "Programs\Inno Setup 6\ISCC.exe")
    )
    if ($directPath) {
        return $directPath
    }

    foreach ($registryPath in @(
        "HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\Inno Setup 6_is1",
        "HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\Inno Setup 6_is1",
        "HKCU:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\Inno Setup 6_is1"
    )) {
        try {
            $installLocation = (Get-ItemProperty -Path $registryPath -ErrorAction Stop).InstallLocation
            $registryCandidate = Get-ExistingFilePath @((Join-Path $installLocation "ISCC.exe"))
            if ($registryCandidate) {
                return $registryCandidate
            }
        } catch {
            continue
        }
    }

    $iscc = Get-Command "ISCC.exe" -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($iscc) {
        $isccPath = Get-ExecutableCommandPath $iscc
        $commandPath = Get-ExistingFilePath @($isccPath)
        if ($commandPath) {
            return $commandPath
        }
    }

    return $null
}

function Resolve-SignToolPath() {
    if ($env:WINDOWS_SIGNTOOL_PATH -and (Test-Path $env:WINDOWS_SIGNTOOL_PATH)) {
        return $env:WINDOWS_SIGNTOOL_PATH
    }

    $command = Get-Command "signtool.exe" -ErrorAction SilentlyContinue
    if ($command) {
        $commandPath = Get-ExecutableCommandPath $command
        if ($commandPath -and (Test-Path $commandPath)) {
            return $commandPath
        }
    }

    $kitRoots = @(
        (Join-Path ${env:ProgramFiles(x86)} "Windows Kits\10\bin"),
        (Join-Path $env:ProgramFiles "Windows Kits\10\bin")
    )

    foreach ($root in $kitRoots) {
        if (-not $root -or -not (Test-Path $root)) {
            continue
        }

        $candidate = Get-ChildItem -Path $root -Directory -ErrorAction SilentlyContinue |
            Sort-Object Name -Descending |
            ForEach-Object { Join-Path $_.FullName "x64\signtool.exe" } |
            Where-Object { Test-Path $_ } |
            Select-Object -First 1

        if ($candidate) {
            return $candidate
        }
    }

    return $null
}

function Get-ProjectVersion($projectPath) {
    [xml]$projectXml = Get-Content $projectPath
    $versionNode = @($projectXml.Project.PropertyGroup | Where-Object { $_.Version } | Select-Object -First 1)[0]
    if (-not $versionNode -or -not $versionNode.Version) {
        throw "Unable to determine <Version> from $projectPath"
    }
    return $versionNode.Version.Trim()
}

function Ensure-SelfSignedCodeSigningCertificate() {
    $subject = "CN=Buildermark Dev Code Signing"
    $friendlyName = "Buildermark Dev Code Signing"

    $existing = Get-ChildItem Cert:\CurrentUser\My -CodeSigningCert -ErrorAction SilentlyContinue |
        Where-Object {
            $_.Subject -eq $subject -and
            $_.FriendlyName -eq $friendlyName -and
            $_.HasPrivateKey -and
            $_.NotAfter -gt (Get-Date)
        } |
        Sort-Object NotAfter -Descending |
        Select-Object -First 1

    if (-not $existing) {
        Step "Creating self-signed code signing certificate"
        $existing = New-SelfSignedCertificate `
            -Type Custom `
            -Subject $subject `
            -FriendlyName $friendlyName `
            -KeyUsage DigitalSignature `
            -KeyAlgorithm RSA `
            -KeyLength 2048 `
            -HashAlgorithm SHA256 `
            -KeyExportPolicy Exportable `
            -CertStoreLocation "Cert:\CurrentUser\My" `
            -NotAfter (Get-Date).AddYears(5) `
            -TextExtension @(
                "2.5.29.37={text}1.3.6.1.5.5.7.3.3",
                "2.5.29.19={text}"
            )
    } else {
        Step "Using existing self-signed code signing certificate"
    }

    Write-Host "  Thumbprint: $($existing.Thumbprint)" -ForegroundColor Green
    return @{
        Mode            = "Thumbprint"
        CertThumbprint  = $existing.Thumbprint
        CertStore       = "My"
        UseMachineStore = $false
        TimestampUrl    = if ($env:WINDOWS_SIGN_TIMESTAMP_URL) { $env:WINDOWS_SIGN_TIMESTAMP_URL } else { "http://timestamp.digicert.com" }
    }
}

function Resolve-AzureSigningDlibPath() {
    $script:AzureDlibSearched = [System.Collections.Generic.List[string]]::new()

    if ($env:AZURE_ARTIFACT_SIGNING_DLIB_PATH) {
        if (Test-Path $env:AZURE_ARTIFACT_SIGNING_DLIB_PATH) {
            return (Get-Item -LiteralPath $env:AZURE_ARTIFACT_SIGNING_DLIB_PATH).FullName
        }
        throw "AZURE_ARTIFACT_SIGNING_DLIB_PATH is set but does not exist: $($env:AZURE_ARTIFACT_SIGNING_DLIB_PATH)"
    }

    # 1. Try MSIX-installed winget package directly via its manifest.
    try {
        $appx = Get-AppxPackage -Name "Microsoft.Azure.ArtifactSigningClientTools*" -ErrorAction SilentlyContinue |
            Sort-Object Version -Descending |
            Select-Object -First 1
        if ($appx -and $appx.InstallLocation) {
            $script:AzureDlibSearched.Add($appx.InstallLocation) | Out-Null
            $candidate = Get-ChildItem -Path $appx.InstallLocation -Recurse -Filter "Azure.CodeSigning.Dlib.dll" -ErrorAction SilentlyContinue |
                Sort-Object LastWriteTime -Descending |
                Select-Object -First 1
            if ($candidate) { return $candidate.FullName }
        }
    } catch {
        # Get-AppxPackage is not available on all SKUs; fall through to path-based search.
    }

    # 2. Common install/package locations for the dlib.
    #    The package name used by Microsoft is "Microsoft.ArtifactSigning.Client"
    #    (lowercased on disk under the NuGet cache). A dedicated tools dir is
    #    also conventional.
    $searchRoots = @(
        (Join-Path $env:USERPROFILE "artifact-signing-tools"),
        (Join-Path $env:USERPROFILE ".nuget\packages\microsoft.artifactsigning.client"),
        (Join-Path $env:LOCALAPPDATA "Microsoft\ArtifactSigningClientTools"),
        (Join-Path $env:ProgramFiles "Microsoft\ArtifactSigningClientTools"),
        (Join-Path ${env:ProgramFiles(x86)} "Microsoft\ArtifactSigningClientTools"),
        (Join-Path $env:LOCALAPPDATA "Microsoft\WinGet\Packages"),
        (Join-Path $env:ProgramFiles "WindowsApps")
    )

    foreach ($root in $searchRoots) {
        if (-not $root -or -not (Test-Path $root)) { continue }
        $script:AzureDlibSearched.Add($root) | Out-Null
        # Prefer x64 variants (our signtool.exe is x64) and newest on disk.
        $candidate = Get-ChildItem -Path $root -Recurse -Filter "Azure.CodeSigning.Dlib.dll" -ErrorAction SilentlyContinue |
            Sort-Object @{ Expression = { $_.FullName -match '\\x64\\' }; Descending = $true },
                        @{ Expression = "LastWriteTime"; Descending = $true } |
            Select-Object -First 1
        if ($candidate) {
            return $candidate.FullName
        }
    }

    return $null
}

function Write-AzureSigningMetadata($outputPath) {
    $metadata = [ordered]@{
        Endpoint               = $env:AZURE_ARTIFACT_SIGNING_ENDPOINT
        CodeSigningAccountName = $env:AZURE_ARTIFACT_SIGNING_ACCOUNT_NAME
        CertificateProfileName = $env:AZURE_ARTIFACT_SIGNING_CERTIFICATE_PROFILE_NAME
    }

    if ($env:AZURE_ARTIFACT_SIGNING_CORRELATION_ID) {
        $metadata["CorrelationId"] = $env:AZURE_ARTIFACT_SIGNING_CORRELATION_ID
    }

    if ($env:AZURE_ARTIFACT_SIGNING_EXCLUDE_CREDENTIALS) {
        $excluded = $env:AZURE_ARTIFACT_SIGNING_EXCLUDE_CREDENTIALS -split '[,;\s]+' |
            Where-Object { $_ } |
            ForEach-Object { $_.Trim() }
        if ($excluded.Count -gt 0) {
            $metadata["ExcludeCredentials"] = @($excluded)
        }
    }

    $outputDir = Split-Path -Parent $outputPath
    if ($outputDir -and -not (Test-Path $outputDir)) {
        New-Item -ItemType Directory -Path $outputDir -Force | Out-Null
    }

    # Write UTF-8 WITHOUT a BOM — the Azure signing dlib parses the metadata
    # with System.Text.Json which rejects a leading 0xEF BOM as invalid JSON.
    $json = $metadata | ConvertTo-Json -Depth 4
    [System.IO.File]::WriteAllText($outputPath, $json, (New-Object System.Text.UTF8Encoding($false)))
    return (Get-Item -LiteralPath $outputPath).FullName
}

function Get-AzureSigningConfig() {
    $account = $env:AZURE_ARTIFACT_SIGNING_ACCOUNT_NAME
    $certProfile = $env:AZURE_ARTIFACT_SIGNING_CERTIFICATE_PROFILE_NAME
    $endpoint = $env:AZURE_ARTIFACT_SIGNING_ENDPOINT

    if (-not $account -or -not $certProfile -or -not $endpoint) {
        throw ("Azure Artifact Signing requires AZURE_ARTIFACT_SIGNING_ACCOUNT_NAME, " +
               "AZURE_ARTIFACT_SIGNING_CERTIFICATE_PROFILE_NAME, and AZURE_ARTIFACT_SIGNING_ENDPOINT.")
    }

    $dlibPath = Resolve-AzureSigningDlibPath
    if (-not $dlibPath) {
        $searched = if ($script:AzureDlibSearched) { ($script:AzureDlibSearched -join "`n    ") } else { "(none)" }
        throw (
            "Azure.CodeSigning.Dlib.dll was not found.`n" +
            "  Searched roots:`n    $searched`n" +
            "  To locate it on the signer, run one of:`n" +
            "    Get-AppxPackage -Name '*ArtifactSigning*' | Select-Object InstallLocation`n" +
            "    Get-ChildItem -Path C:\ -Recurse -Filter Azure.CodeSigning.Dlib.dll -ErrorAction SilentlyContinue`n" +
            "  Then set AZURE_ARTIFACT_SIGNING_DLIB_PATH to that file."
        )
    }

    if ($env:AZURE_ARTIFACT_SIGNING_METADATA_PATH) {
        if (-not (Test-Path $env:AZURE_ARTIFACT_SIGNING_METADATA_PATH)) {
            throw "AZURE_ARTIFACT_SIGNING_METADATA_PATH does not exist: $($env:AZURE_ARTIFACT_SIGNING_METADATA_PATH)"
        }
        $metadataPath = (Get-Item -LiteralPath $env:AZURE_ARTIFACT_SIGNING_METADATA_PATH).FullName
    } else {
        $metadataPath = Write-AzureSigningMetadata (Join-Path ([System.IO.Path]::GetTempPath()) "buildermark-azure-signing-metadata.json")
    }

    $timestampUrl = if ($env:WINDOWS_SIGN_TIMESTAMP_URL) {
        $env:WINDOWS_SIGN_TIMESTAMP_URL
    } else {
        "http://timestamp.acs.microsoft.com/"
    }

    Write-Host "  Azure account:  $account" -ForegroundColor Green
    Write-Host "  Azure profile:  $certProfile" -ForegroundColor Green
    Write-Host "  Dlib:           $dlibPath"
    Write-Host "  Metadata:       $metadataPath"

    return @{
        Mode         = "Azure"
        DlibPath     = $dlibPath
        MetadataPath = $metadataPath
        TimestampUrl = $timestampUrl
    }
}

function Get-SigningConfig() {
    if ($SelfSign) {
        return Ensure-SelfSignedCodeSigningCertificate
    }

    if ($env:AZURE_ARTIFACT_SIGNING_ACCOUNT_NAME -or
        $env:AZURE_ARTIFACT_SIGNING_CERTIFICATE_PROFILE_NAME -or
        $env:AZURE_ARTIFACT_SIGNING_ENDPOINT) {
        Step "Configuring Azure Artifact Signing"
        return Get-AzureSigningConfig
    }

    $certFile = $env:WINDOWS_SIGN_CERT_FILE
    $certThumbprint = $env:WINDOWS_SIGN_CERT_THUMBPRINT

    if ($certFile -and $certThumbprint) {
        throw "Set either WINDOWS_SIGN_CERT_FILE or WINDOWS_SIGN_CERT_THUMBPRINT, not both."
    }

    if (-not $certFile -and -not $certThumbprint) {
        throw ("Signing requires one of: Azure Artifact Signing env vars " +
               "(AZURE_ARTIFACT_SIGNING_ACCOUNT_NAME + ..._CERTIFICATE_PROFILE_NAME + ..._ENDPOINT), " +
               "WINDOWS_SIGN_CERT_FILE, or WINDOWS_SIGN_CERT_THUMBPRINT.")
    }

    if ($certFile -and -not (Test-Path $certFile)) {
        throw "WINDOWS_SIGN_CERT_FILE does not exist: $certFile"
    }

    return @{
        Mode            = if ($certFile) { "CertFile" } else { "Thumbprint" }
        CertFile        = $certFile
        CertPassword    = $env:WINDOWS_SIGN_CERT_PASSWORD
        CertThumbprint  = $certThumbprint
        CertStore       = if ($env:WINDOWS_SIGN_CERT_STORE) { $env:WINDOWS_SIGN_CERT_STORE } else { "My" }
        UseMachineStore = ($env:WINDOWS_SIGN_CERT_MACHINE_STORE -eq "1")
        TimestampUrl    = if ($env:WINDOWS_SIGN_TIMESTAMP_URL) { $env:WINDOWS_SIGN_TIMESTAMP_URL } else { "http://timestamp.digicert.com" }
    }
}

function Get-SignToolArguments($signingConfig) {
    $arguments = @("sign", "/v")

    if ($signingConfig.Mode -eq "Azure") {
        # /debug makes the Azure dlib emit useful diagnostics on auth/dependency failures.
        $arguments += "/debug"
    }

    $arguments += @("/fd", "SHA256", "/tr", $signingConfig.TimestampUrl, "/td", "SHA256")

    if ($signingConfig.Mode -eq "Azure") {
        $arguments += @("/dlib", $signingConfig.DlibPath, "/dmdf", $signingConfig.MetadataPath)
    } elseif ($signingConfig.CertFile) {
        $arguments += @("/f", $signingConfig.CertFile)
        if ($signingConfig.CertPassword) {
            $arguments += @("/p", $signingConfig.CertPassword)
        }
    } else {
        $arguments += @("/sha1", $signingConfig.CertThumbprint, "/s", $signingConfig.CertStore)
        if ($signingConfig.UseMachineStore) {
            $arguments += "/sm"
        }
    }

    return $arguments
}

function Convert-ToInnoSignToolToken($value) {
    $escaped = [string]$value -replace '\$', '$$'
    if ($escaped -match '[\s"]') {
        return '$q' + ($escaped -replace '"', '$q') + '$q'
    }
    return $escaped
}

function Get-InnoSignToolCommand($signToolPath, $signingConfig) {
    $parts = @((Convert-ToInnoSignToolToken $signToolPath))
    foreach ($argument in (Get-SignToolArguments -signingConfig $signingConfig)) {
        $parts += (Convert-ToInnoSignToolToken $argument)
    }
    $parts += '$f'
    return ($parts -join ' ')
}

function Sign-File($signToolPath, $signingConfig, $path) {
    Step "Signing $(Split-Path -Leaf $path)"
    $arguments = Get-SignToolArguments -signingConfig $signingConfig

    # Log the exact command so we can reproduce it manually if it fails.
    Write-Host "  Command: $signToolPath $($arguments -join ' ') `"$path`""

    $stdoutFile = [System.IO.Path]::GetTempFileName()
    $stderrFile = [System.IO.Path]::GetTempFileName()
    try {
        $proc = Start-Process -FilePath $signToolPath `
            -ArgumentList (@($arguments) + @($path)) `
            -NoNewWindow -Wait -PassThru `
            -RedirectStandardOutput $stdoutFile `
            -RedirectStandardError $stderrFile

        $stdoutText = Get-Content -Raw -LiteralPath $stdoutFile -ErrorAction SilentlyContinue
        $stderrText = Get-Content -Raw -LiteralPath $stderrFile -ErrorAction SilentlyContinue

        if ($stdoutText) { Write-Host "  [stdout]"; Write-Host $stdoutText }
        if ($stderrText) { Write-Host "  [stderr]" -ForegroundColor Yellow; Write-Host $stderrText -ForegroundColor Yellow }

        if ($proc.ExitCode -ne 0) {
            throw "signtool sign failed for $path (exit code $($proc.ExitCode))"
        }
    } finally {
        Remove-Item -LiteralPath $stdoutFile -ErrorAction SilentlyContinue
        Remove-Item -LiteralPath $stderrFile -ErrorAction SilentlyContinue
    }
}

function Verify-SignedFile($signToolPath, $path) {
    Step "Verifying signature for $(Split-Path -Leaf $path)"
    $verifyArgs = if ($SelfSign) {
        @("verify", "/all", "/tw")
    } else {
        @("verify", "/pa", "/all", "/tw")
    }

    $oldErrorActionPreference = $ErrorActionPreference
    try {
        $ErrorActionPreference = "Continue"
        $verifyOutput = & $signToolPath @verifyArgs $path 2>&1
        $verifyExitCode = $LASTEXITCODE
    } finally {
        $ErrorActionPreference = $oldErrorActionPreference
    }

    if ($verifyOutput) {
        $verifyOutput | ForEach-Object { Write-Host $_ }
    }

    if ($verifyExitCode -ne 0) {
        $verifyText = ($verifyOutput | Out-String)
        if ($SelfSign -and $verifyText -match 'not trusted by the trust provider') {
            Write-Host "  Warning: self-signed verification hit the expected untrusted-root result; continuing." -ForegroundColor Yellow
            return
        }

        throw "signtool verify failed for $path (exit code $verifyExitCode)"
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

$IsccPath = [string](Resolve-IsccPath)
if ($IsccPath) {
    $IsccPath = $IsccPath.Trim('"')
}
if (-not $IsccPath) {
    Write-Host "Error: ISCC.exe was not found." -ForegroundColor Red
    Write-Host "  Install Inno Setup 6 or set WINDOWS_INNO_COMPILER_PATH."
    exit 1
}

$SignToolPath = $null
$SigningConfig = $null
    if (-not $SkipSigning) {
        $SignToolPath = Resolve-SignToolPath
        if (-not $SignToolPath) {
            Write-Host "Error: signtool.exe was not found." -ForegroundColor Red
            Write-Host "  Install the Windows SDK or set WINDOWS_SIGNTOOL_PATH."
        exit 1
    }

    try {
        $SigningConfig = Get-SigningConfig
    } catch {
        Write-Host "Error: $($_.Exception.Message)" -ForegroundColor Red
        Write-Host "  Configure WINDOWS_SIGN_CERT_FILE or WINDOWS_SIGN_CERT_THUMBPRINT before building, or set SELF_SIGN=1."
        Write-Host "  Set WINDOWS_SKIP_SIGNING=1 only if you intentionally want an unsigned dev build."
        exit 1
    }
}

Assert-PathExists $WindowsBuildScript "Windows app build script"
Assert-PathExists $InnoScriptPath "Inno Setup script"
Assert-PathExists $CsprojPath "Windows project file"

$AppVersion = Get-ProjectVersion $CsprojPath

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
    if (-not $goArch) { throw "Unsupported runtime: $rid" }
    $installerArch = $InstallerArchMap[$rid]
    if (-not $installerArch) { throw "Unsupported installer architecture for runtime: $rid" }

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
    & $WindowsBuildScript -Configuration $Configuration -Runtime $rid
    if ($LASTEXITCODE -ne 0) { throw "Windows app build failed for $rid (exit code $LASTEXITCODE)" }

    # -- Stage the published app payload --
    $RuntimeDir = Join-Path (Join-Path $WindowsDir "build") $rid
    $PublishDir = Join-Path $RuntimeDir "publish"
    $InstallerDir = Join-Path $RuntimeDir "installer"
    $MainExePath = Join-Path $PublishDir "Buildermark.exe"
    $ServerExePath = Join-Path $PublishDir $ServerBinary
    $InstallerFileName = "Buildermark-$AppVersion-windows-$rid-Setup"
    $InstallerPath = Join-Path $InstallerDir "$InstallerFileName.exe"

    Assert-PathExists $PublishDir "published app directory"
    Assert-PathExists $MainExePath "main Windows executable"

    Copy-Item (Join-Path $ServerDir $ServerBinary) $ServerExePath -Force

    # Clean up the server binary from the source tree
    Remove-Item (Join-Path $ServerDir $ServerBinary)

    if (-not $SkipSigning) {
        Sign-File -signToolPath $SignToolPath -signingConfig $SigningConfig -path $MainExePath
        Sign-File -signToolPath $SignToolPath -signingConfig $SigningConfig -path $ServerExePath
        Verify-SignedFile -signToolPath $SignToolPath -path $MainExePath
        Verify-SignedFile -signToolPath $SignToolPath -path $ServerExePath
    }

    Step "Building Inno Setup installer ($rid)"
    if (-not (Test-Path $InstallerDir)) {
        New-Item -ItemType Directory -Path $InstallerDir -Force | Out-Null
    }
    Write-Host "  ISCC: $IsccPath"

    $enableSigning = if ($SkipSigning) { "no" } else { "yes" }
    $isccArgs = @(
        "/DAppName=$AppName",
        "/DAppVersion=$AppVersion",
        "/DPublishDir=$PublishDir",
        "/DOutputDir=$InstallerDir",
        "/DOutputBaseFilename=$InstallerFileName",
        "/DArchitecturesAllowed=$installerArch",
        "/DArchitecturesInstallIn64BitMode=$installerArch",
        "/DEnableSigning=$enableSigning"
    )

    if (-not $SkipSigning) {
        $isccArgs += "/Sbuildermark=$(Get-InnoSignToolCommand -signToolPath $SignToolPath -signingConfig $SigningConfig)"
    }

    $isccArgs += $InnoScriptPath
    & $IsccPath @isccArgs
    if ($LASTEXITCODE -ne 0) { throw "ISCC failed for $rid (exit code $LASTEXITCODE)" }

    Assert-PathExists $InstallerPath "installer output"

    if (-not $SkipSigning) {
        Verify-SignedFile -signToolPath $SignToolPath -path $InstallerPath
    }

    Write-Host "  Publish:   $PublishDir" -ForegroundColor Green
    Write-Host "  Installer: $InstallerPath" -ForegroundColor Green
}

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------

Step "Full build complete"
foreach ($rid in $Runtimes) {
    $RuntimeDir = Join-Path (Join-Path $WindowsDir "build") $rid
    Write-Host "  $rid : $(Join-Path $RuntimeDir "installer")"
}
