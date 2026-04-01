#
# Check whether an Azure Artifact Signing certificate profile exists.
# If it doesn't, print the exact Azure CLI command to create a Public Trust
# profile for Windows code signing.
#
# Usage:
#   powershell -ExecutionPolicy Bypass -File scripts\check-artifact-signing-profile.ps1 `
#     -ResourceGroup MyResourceGroup `
#     -AccountName MyAccount `
#     -ProfileName MyProfile
#

param(
    [string]$ResourceGroup = $env:AZURE_ARTIFACT_SIGNING_RESOURCE_GROUP,
    [string]$AccountName = $env:AZURE_ARTIFACT_SIGNING_ACCOUNT_NAME,
    [string]$ProfileName = $env:AZURE_ARTIFACT_SIGNING_CERTIFICATE_PROFILE_NAME,
    [string]$IdentityValidationId = $env:AZURE_ARTIFACT_SIGNING_IDENTITY_VALIDATION_ID,
    [switch]$IncludeStreet,
    [switch]$IncludePostalCode
)

$ErrorActionPreference = "Stop"

function Require-Value($name, $value) {
    if (-not $value) {
        throw "$name is required."
    }
}

Require-Value "ResourceGroup" $ResourceGroup
Require-Value "AccountName" $AccountName
Require-Value "ProfileName" $ProfileName

$az = Get-Command "az" -ErrorAction SilentlyContinue
if (-not $az) {
    throw "Azure CLI (az) is not installed."
}

& $az.Path extension add --name artifact-signing --upgrade --only-show-errors | Out-Null

$showArgs = @(
    "artifact-signing", "certificate-profile", "show",
    "-g", $ResourceGroup,
    "--account-name", $AccountName,
    "-n", $ProfileName,
    "--output", "json"
)

$oldErrorActionPreference = $ErrorActionPreference
try {
    $ErrorActionPreference = "Continue"
    $profileJson = & $az.Path @showArgs 2>$null
    $showExitCode = $LASTEXITCODE
} finally {
    $ErrorActionPreference = $oldErrorActionPreference
}

if ($showExitCode -eq 0 -and $profileJson) {
    Write-Host "Certificate profile exists:" -ForegroundColor Green
    $profileJson
    exit 0
}

Write-Host "Certificate profile was not found." -ForegroundColor Yellow
Write-Host ""
Write-Host "Create a Public Trust profile with:" -ForegroundColor Cyan

$createCommand = @(
    "az artifact-signing certificate-profile create",
    "-g `"$ResourceGroup`"",
    "--account-name `"$AccountName`"",
    "-n `"$ProfileName`"",
    "--profile-type PublicTrust",
    "--identity-validation-id `"<identity-validation-id>`""
)

if ($IdentityValidationId) {
    $createCommand[-1] = "--identity-validation-id `"$IdentityValidationId`""
}
if ($IncludeStreet) {
    $createCommand += "--include-street true"
}
if ($IncludePostalCode) {
    $createCommand += "--include-postal-code true"
}

Write-Host ($createCommand -join " ")
Write-Host ""
Write-Host "Required values:" -ForegroundColor Cyan
Write-Host "  account name: $AccountName"
Write-Host "  certificate profile name: $ProfileName"
Write-Host "  profile type: PublicTrust"
if ($IdentityValidationId) {
    Write-Host "  identity validation ID: $IdentityValidationId"
} else {
    Write-Host "  identity validation ID: copy it from Azure portal > Artifact Signing account > Identity validations"
}
Write-Host "  include street address: $($IncludeStreet.IsPresent)"
Write-Host "  include postal code: $($IncludePostalCode.IsPresent)"
