# qs Installer

$ErrorActionPreference = "Stop"

# ANSI color codes
$R  = [char]27 + '[0m'
$BC = [char]27 + '[96m'
$C  = [char]27 + '[36m'
$DG = [char]27 + '[90m'
$BW = [char]27 + '[97m'
$BG = [char]27 + '[92m'
$BY = [char]27 + '[93m'
$BR = [char]27 + '[91m'
$BB = [char]27 + '[1;97m'

$line = $DG + ([string]::new([char]0x2500, 18)) + " $([char]0x25C6) " + ([string]::new([char]0x2500, 18)) + $R

# Logo
Write-Host ""
Write-Host "  ${BC} ██████╗  ███████╗      ▗▄█▄▖${R}"
Write-Host "  ${BC}██╔═══██╗ ██╔════╝     ▐█████▌${R}  ${BB}installer${R}"
Write-Host "  ${C}██║   ██║ ███████╗      ▝▀█▀▘${R}"
Write-Host "  ${C}██║▄▄ ██║ ╚════██║       ▘ ▝${R}"
Write-Host "  ${DG}╚██████╔╝ ███████║${R}"
Write-Host "  ${DG} ╚══▀▀═╝  ╚══════╝${R}"
Write-Host ""
Write-Host "  $line"

# Build (no -ldflags stripping - WDAC blocks binaries without Go build ID)
Write-Host ""
Write-Host "  ${BC}◆${R} ${BW}Building${R}"

go build -o qs-new.exe .
if ($LASTEXITCODE -ne 0) {
    Write-Host "   ${BR}✗ Build failed${R}"
    exit 1
}
Write-Host "   ${DG}▪${R} qs-new.exe                  ${BG}✓${R}"

# Install
$installDir = "$env:USERPROFILE\.qs\bin"

Write-Host ""
Write-Host "  ${BC}◆${R} ${BW}Installing${R}"

if (-not (Test-Path $installDir)) {
    New-Item -ItemType Directory -Path $installDir -Force | Out-Null
    Write-Host "   ${DG}▪${R} Created ~/.qs/bin/          ${BG}✓${R}"
}

# Rename running binary out of the way (Windows allows renaming locked files)
if (Test-Path "$installDir\qs.exe") {
    $ts = Get-Date -Format "yyyyMMddHHmmss"
    Rename-Item "$installDir\qs.exe" "qs-old-${ts}.exe" -ErrorAction SilentlyContinue
    if ($?) {
        Write-Host "   ${DG}▪${R} Renamed locked qs.exe      ${BG}✓${R}"
    }
}

Copy-Item "qs-new.exe" -Destination "$installDir\qs.exe" -Force
Write-Host "   ${DG}▪${R} Copied qs.exe               ${BG}✓${R}"

# Best-effort cleanup of old binaries (skip still-locked ones)
Get-ChildItem "$installDir\qs-old-*.exe" -ErrorAction SilentlyContinue | ForEach-Object {
    Remove-Item $_.FullName -ErrorAction SilentlyContinue
}

Remove-Item "qs-new.exe" -ErrorAction SilentlyContinue

# Check for legacy config and print migration notice
$legacyConfig = "$env:USERPROFILE\.cc\config.yaml"
if (Test-Path $legacyConfig) {
    Write-Host ""
    Write-Host "  ${BC}◆${R} ${BW}Migration${R}"
    Write-Host "   ${DG}▪${R} Found ~/.cc/config.yaml"
    Write-Host "   ${DG}▪${R} Will auto-migrate to ~/.qs/ on first run"
}

# PATH
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$installDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$userPath;$installDir", "User")
    Write-Host "   ${DG}▪${R} Added to PATH               ${BG}✓${R}"
    Write-Host ""
    Write-Host "   ${BY}▪ Restart terminal for PATH changes${R}"
} else {
    Write-Host "   ${DG}▪${R} Already in PATH             ${BG}✓${R}"
}

# Clean up old binaries if they exist
$oldBinDir = "$env:USERPROFILE\.cc\bin"
if (Test-Path "$oldBinDir\cc.exe") {
    Write-Host ""
    Write-Host "  ${BC}◆${R} ${BW}Cleanup${R}"
    Write-Host "   ${DG}▪${R} Old cc/cx/all binaries found in ~/.cc/bin/"
    Write-Host "   ${DG}▪${R} You can safely remove them: ${DG}rm ~/.cc/bin/*.exe${R}"
}

# Done
Write-Host ""
Write-Host "  $line"
Write-Host "  ${BG}◆${R} ${BW}Ready${R} ${DG}·${R} ${BC}qs${R} ${DG}(launcher)${R} ${BC}qs setup${R} ${DG}(wizard)${R} ${BC}qs accounts${R} ${DG}(manage)${R}"
Write-Host ""
