# fleetorch installer for Windows — PowerShell mirror of install.sh
#
# Usage (one line, from any PowerShell):
#   irm https://raw.githubusercontent.com/msnotfound/fleetorch/main/scripts/install.ps1 | iex
#
# Environment overrides:
#   $env:FLEETORCH_VERSION   pin a specific tag (default: latest)
#   $env:FLEETORCH_BIN_DIR   install destination (default: %LOCALAPPDATA%\Programs\fleetorch)
#   $env:FLEETORCH_NO_PATH   set to "1" to skip the PATH update step
#
# Requires PowerShell 5+ (built into Windows 10/11) or PowerShell 7+.

$ErrorActionPreference = 'Stop'

# Suppress the PowerShell 5.1 progress bar — its rendering cripples
# Invoke-WebRequest's throughput (~10x slower on PS5.1, less on PS7+).
# We restore the original preference before exit so we don't leak into
# the user's session if they ran this script (not iex) from a profile.
$origProgress = $ProgressPreference
$ProgressPreference = 'SilentlyContinue'
try {

$Repo = 'msnotfound/fleetorch'

function Resolve-Arch {
    # PowerShell exposes the OS arch via $env:PROCESSOR_ARCHITECTURE. Map to fleetorch's
    # release asset naming (x86_64 / arm64).
    switch ($env:PROCESSOR_ARCHITECTURE) {
        'AMD64' { 'x86_64' }
        'ARM64' { 'arm64'  }
        default { throw "Unsupported architecture: $($env:PROCESSOR_ARCHITECTURE). fleetorch ships x86_64 and arm64 for Windows." }
    }
}

function Resolve-LatestVersion {
    if ($env:FLEETORCH_VERSION) { return $env:FLEETORCH_VERSION }
    $api = "https://api.github.com/repos/$Repo/releases/latest"
    try {
        $resp = Invoke-RestMethod -Uri $api -UseBasicParsing -Headers @{ 'User-Agent' = 'fleetorch-installer' }
        return $resp.tag_name
    } catch {
        throw "Could not resolve latest version from GitHub. Set `$env:FLEETORCH_VERSION manually. ($_)"
    }
}

function Get-ChecksumFor($ChecksumsText, $AssetName) {
    foreach ($line in $ChecksumsText -split "`n") {
        $fields = $line.Trim() -split '\s+'
        if ($fields.Count -eq 2 -and $fields[1] -eq $AssetName) {
            return $fields[0].ToLower()
        }
    }
    return $null
}

# ---- main -----------------------------------------------------------------

Write-Host 'fleetorch installer for Windows' -ForegroundColor Cyan

$arch = Resolve-Arch
$version = Resolve-LatestVersion
$versionNum = $version.TrimStart('v')

$asset = "fleetorch_${versionNum}_windows_${arch}.zip"
$url = "https://github.com/$Repo/releases/download/$version/$asset"

Write-Host "  version: $version" -ForegroundColor Gray
Write-Host "  arch:    windows/$arch" -ForegroundColor Gray
Write-Host "  asset:   $asset" -ForegroundColor Gray

# Install dir
$binDir = $env:FLEETORCH_BIN_DIR
if (-not $binDir) {
    $binDir = Join-Path $env:LOCALAPPDATA 'Programs\fleetorch'
}
if (-not (Test-Path $binDir)) {
    New-Item -ItemType Directory -Force -Path $binDir | Out-Null
}

# Temp staging
$tmp = New-Item -ItemType Directory -Path (Join-Path $env:TEMP "fleetorch-install-$([guid]::NewGuid())") -Force
try {
    $archivePath = Join-Path $tmp.FullName $asset
    $checksumsPath = Join-Path $tmp.FullName 'checksums.txt'

    Write-Host "  downloading…" -ForegroundColor Gray
    Invoke-WebRequest -Uri $url -OutFile $archivePath -UseBasicParsing
    Invoke-WebRequest -Uri "https://github.com/$Repo/releases/download/$version/checksums.txt" -OutFile $checksumsPath -UseBasicParsing

    # Verify SHA-256
    $want = Get-ChecksumFor (Get-Content $checksumsPath -Raw) $asset
    if (-not $want) { throw "checksum for $asset not found in checksums.txt" }
    $got = (Get-FileHash $archivePath -Algorithm SHA256).Hash.ToLower()
    if ($want -ne $got) {
        throw "checksum mismatch for $asset`n  want: $want`n  got:  $got"
    }
    Write-Host "  checksum verified" -ForegroundColor Gray

    # Extract — the zip contains fleetorch.exe at the root
    Expand-Archive -LiteralPath $archivePath -DestinationPath $tmp.FullName -Force
    $extractedExe = Get-ChildItem -Path $tmp.FullName -Filter 'fleetorch.exe' -Recurse | Select-Object -First 1
    if (-not $extractedExe) { throw "fleetorch.exe not found in $asset after extraction" }

    $target = Join-Path $binDir 'fleetorch.exe'
    Copy-Item -LiteralPath $extractedExe.FullName -Destination $target -Force
    Write-Host "  installed: $target" -ForegroundColor Green
} finally {
    Remove-Item -Recurse -Force $tmp.FullName -ErrorAction SilentlyContinue
}

# PATH update — user scope, idempotent
if ($env:FLEETORCH_NO_PATH -ne '1') {
    $userPath = [Environment]::GetEnvironmentVariable('PATH', 'User')
    $userPathParts = @()
    if ($userPath) { $userPathParts = $userPath.Split(';') | Where-Object { $_ } }
    if ($userPathParts -notcontains $binDir) {
        $newPath = (@($binDir) + $userPathParts) -join ';'
        [Environment]::SetEnvironmentVariable('PATH', $newPath, 'User')
        Write-Host "  added $binDir to your user PATH (open a new shell to use it)" -ForegroundColor Gray
    } else {
        Write-Host "  $binDir already on your user PATH" -ForegroundColor Gray
    }
}

Write-Host ''
Write-Host "fleetorch is ready." -ForegroundColor Green
Write-Host "  open a new PowerShell window and run:  fleetorch --help"
Write-Host ''
Write-Host "  Optional — install tab completion:" -ForegroundColor Gray
Write-Host "    fleetorch completion powershell | Out-File -Append `$PROFILE" -ForegroundColor Gray

} finally {
    $ProgressPreference = $origProgress
}
