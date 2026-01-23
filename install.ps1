# Quickstart Installer
# Adds 'quickstart' command to your system

$ErrorActionPreference = "Stop"

Write-Host ""
Write-Host "  Quickstart Installer" -ForegroundColor Cyan
Write-Host "  ====================" -ForegroundColor Cyan
Write-Host ""

# Determine install location
$installDir = "$env:USERPROFILE\.quickstart"
$scriptSource = Join-Path $PSScriptRoot "scripts\quickstart.ps1"

# Create install directory
if (-not (Test-Path $installDir)) {
    New-Item -ItemType Directory -Path $installDir -Force | Out-Null
    Write-Host "  Created: $installDir" -ForegroundColor Green
}

# Copy the script
Copy-Item $scriptSource -Destination "$installDir\quickstart.ps1" -Force
Write-Host "  Installed script to: $installDir\quickstart.ps1" -ForegroundColor Green

# Create a batch file wrapper for easy command-line use
$batchContent = @"
@echo off
powershell -ExecutionPolicy Bypass -File "%USERPROFILE%\.quickstart\quickstart.ps1" %*
"@
Set-Content -Path "$installDir\quickstart.bat" -Value $batchContent
Write-Host "  Created: $installDir\quickstart.bat" -ForegroundColor Green

# Add to PATH if not already there
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$installDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$userPath;$installDir", "User")
    Write-Host "  Added to PATH: $installDir" -ForegroundColor Green
    Write-Host ""
    Write-Host "  NOTE: Restart your terminal for PATH changes to take effect" -ForegroundColor Yellow
} else {
    Write-Host "  Already in PATH: $installDir" -ForegroundColor Gray
}

Write-Host ""
Write-Host "  Installation complete!" -ForegroundColor Cyan
Write-Host ""
Write-Host "  FIRST TIME SETUP:" -ForegroundColor Yellow
Write-Host "    quickstart -Init              # Interactive setup (recommended)"
Write-Host ""
Write-Host "  Or specify settings directly:" -ForegroundColor White
Write-Host "    quickstart -ProjectsDir 'C:\dev' -Windows '1,2,4'"
Write-Host ""
Write-Host "  Examples:" -ForegroundColor White
Write-Host "    -Windows '1'       # 1 window on 1 monitor"
Write-Host "    -Windows '1,2'     # 1 window + 2 windows on 2 monitors"
Write-Host "    -Windows '1,2,4'   # 1 + 2 + 4 windows on 3 monitors"
Write-Host ""
Write-Host "  Other commands:" -ForegroundColor White
Write-Host "    quickstart -List              # Show detected monitors"
Write-Host ""
