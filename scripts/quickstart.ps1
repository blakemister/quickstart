# Quickstart - Proof of Concept
# A PowerShell script to launch multiple terminals across monitors
# Each terminal opens a project picker and then runs Claude Code

param(
    [string]$ProjectsDir = "$env:USERPROFILE\.1dev",
    [string]$PostCommand = "claude --dangerously-skip-permissions",
    [string]$Windows = "",           # Override: "1,2,4" means 1 on monitor 1, 2 on monitor 2, etc.
    [switch]$Init,                   # Run interactive setup
    [switch]$List,                   # Just list monitors and exit
    [switch]$Verbose
)

# ============================================================================
# Configuration
# ============================================================================

# Default monitor config (edit this for your setup, or use -Windows parameter)
# Format: @{ MonitorIndex = @{ Windows = N; Layout = "grid"|"vertical"|"horizontal"|"full" } }
$DefaultMonitorConfig = @{
    0 = @{ Windows = 1; Layout = "full" }       # Monitor 1 (usually laptop/primary)
    1 = @{ Windows = 2; Layout = "vertical" }   # Monitor 2
    2 = @{ Windows = 4; Layout = "grid" }       # Monitor 3
}

# Build config - can be overridden by -Windows parameter
$Config = @{
    ProjectsDir = $ProjectsDir
    PostCommand = $PostCommand
    Monitors = $DefaultMonitorConfig
}

# Override monitor config if -Windows parameter provided (e.g., "1,2,4")
if ($Windows -ne "") {
    $windowCounts = $Windows -split ","
    $Config.Monitors = @{}
    for ($i = 0; $i -lt $windowCounts.Count; $i++) {
        $count = [int]$windowCounts[$i]
        $layout = if ($count -eq 1) { "full" } elseif ($count -le 2) { "vertical" } else { "grid" }
        $Config.Monitors[$i] = @{ Windows = $count; Layout = $layout }
    }
}

# ============================================================================
# Windows API Definitions
# ============================================================================

Add-Type @"
using System;
using System.Runtime.InteropServices;
using System.Collections.Generic;

public class MonitorInfo {
    // Full monitor bounds
    public int Left;
    public int Top;
    public int Right;
    public int Bottom;
    // Work area (excludes taskbar)
    public int WorkLeft;
    public int WorkTop;
    public int WorkRight;
    public int WorkBottom;
    public bool IsPrimary;

    public int Width { get { return WorkRight - WorkLeft; } }
    public int Height { get { return WorkBottom - WorkTop; } }
    // Use work area for positioning (accounts for taskbar)
    public int X { get { return WorkLeft; } }
    public int Y { get { return WorkTop; } }
}

public class WinAPI {
    [DllImport("user32.dll")]
    public static extern bool EnumDisplayMonitors(IntPtr hdc, IntPtr lprcClip, MonitorEnumDelegate lpfnEnum, IntPtr dwData);

    [DllImport("user32.dll", CharSet = CharSet.Auto)]
    public static extern bool GetMonitorInfo(IntPtr hMonitor, ref MONITORINFOEX lpmi);

    [DllImport("user32.dll", SetLastError = true)]
    public static extern IntPtr FindWindow(string lpClassName, string lpWindowName);

    [DllImport("user32.dll", SetLastError = true)]
    public static extern bool SetWindowPos(IntPtr hWnd, IntPtr hWndInsertAfter, int X, int Y, int cx, int cy, uint uFlags);

    [DllImport("user32.dll")]
    public static extern bool EnumWindows(EnumWindowsProc lpEnumFunc, IntPtr lParam);

    [DllImport("user32.dll", CharSet = CharSet.Auto, SetLastError = true)]
    public static extern int GetWindowText(IntPtr hWnd, System.Text.StringBuilder lpString, int nMaxCount);

    public delegate bool MonitorEnumDelegate(IntPtr hMonitor, IntPtr hdcMonitor, ref RECT lprcMonitor, IntPtr dwData);
    public delegate bool EnumWindowsProc(IntPtr hWnd, IntPtr lParam);

    [StructLayout(LayoutKind.Sequential, CharSet = CharSet.Auto)]
    public struct MONITORINFOEX {
        public int cbSize;
        public RECT rcMonitor;
        public RECT rcWork;
        public uint dwFlags;
        [MarshalAs(UnmanagedType.ByValTStr, SizeConst = 32)]
        public string szDevice;
    }

    [StructLayout(LayoutKind.Sequential)]
    public struct RECT {
        public int Left;
        public int Top;
        public int Right;
        public int Bottom;
    }

    public const uint SWP_NOZORDER = 0x0004;
    public const uint SWP_SHOWWINDOW = 0x0040;
    public const int MONITORINFOF_PRIMARY = 0x00000001;

    public static List<MonitorInfo> GetMonitors() {
        var monitors = new List<MonitorInfo>();

        EnumDisplayMonitors(IntPtr.Zero, IntPtr.Zero,
            delegate (IntPtr hMonitor, IntPtr hdcMonitor, ref RECT lprcMonitor, IntPtr dwData) {
                MONITORINFOEX mi = new MONITORINFOEX();
                mi.cbSize = Marshal.SizeOf(mi);

                if (GetMonitorInfo(hMonitor, ref mi)) {
                    monitors.Add(new MonitorInfo {
                        Left = mi.rcMonitor.Left,
                        Top = mi.rcMonitor.Top,
                        Right = mi.rcMonitor.Right,
                        Bottom = mi.rcMonitor.Bottom,
                        // Use work area (excludes taskbar) for better window placement
                        WorkLeft = mi.rcWork.Left,
                        WorkTop = mi.rcWork.Top,
                        WorkRight = mi.rcWork.Right,
                        WorkBottom = mi.rcWork.Bottom,
                        IsPrimary = (mi.dwFlags & MONITORINFOF_PRIMARY) != 0
                    });
                }
                return true;
            }, IntPtr.Zero);

        // Sort by X position (left to right)
        monitors.Sort((a, b) => a.Left.CompareTo(b.Left));

        return monitors;
    }

    public static IntPtr FindWindowByTitle(string titlePart) {
        IntPtr found = IntPtr.Zero;

        EnumWindows(delegate (IntPtr hWnd, IntPtr lParam) {
            var sb = new System.Text.StringBuilder(256);
            GetWindowText(hWnd, sb, 256);
            if (sb.ToString().Contains(titlePart)) {
                found = hWnd;
                return false; // Stop enumeration
            }
            return true;
        }, IntPtr.Zero);

        return found;
    }
}
"@

# ============================================================================
# Functions
# ============================================================================

function Get-Monitors {
    $monitors = [WinAPI]::GetMonitors()
    return $monitors
}

function Get-WindowPositions {
    param(
        [MonitorInfo]$Monitor,
        [int]$WindowCount,
        [string]$Layout
    )

    $positions = @()

    switch ($Layout) {
        "grid" {
            # Calculate grid dimensions
            $cols = 1
            $rows = 1
            while ($cols * $rows -lt $WindowCount) {
                if ($cols -le $rows) { $cols++ } else { $rows++ }
            }

            $cellWidth = [math]::Floor($Monitor.Width / $cols)
            $cellHeight = [math]::Floor($Monitor.Height / $rows)

            for ($i = 0; $i -lt $WindowCount; $i++) {
                $row = [math]::Floor($i / $cols)
                $col = $i % $cols

                $positions += @{
                    X = $Monitor.X + ($col * $cellWidth)
                    Y = $Monitor.Y + ($row * $cellHeight)
                    Width = $cellWidth
                    Height = $cellHeight
                }
            }
        }
        "vertical" {
            $cellWidth = [math]::Floor($Monitor.Width / $WindowCount)

            for ($i = 0; $i -lt $WindowCount; $i++) {
                $positions += @{
                    X = $Monitor.X + ($i * $cellWidth)
                    Y = $Monitor.Y
                    Width = $cellWidth
                    Height = $Monitor.Height
                }
            }
        }
        "horizontal" {
            $cellHeight = [math]::Floor($Monitor.Height / $WindowCount)

            for ($i = 0; $i -lt $WindowCount; $i++) {
                $positions += @{
                    X = $Monitor.X
                    Y = $Monitor.Y + ($i * $cellHeight)
                    Width = $Monitor.Width
                    Height = $cellHeight
                }
            }
        }
        "full" {
            $positions += @{
                X = $Monitor.X
                Y = $Monitor.Y
                Width = $Monitor.Width
                Height = $Monitor.Height
            }
        }
    }

    return $positions
}

function Start-QuickstartTerminal {
    param(
        [string]$Title,
        [string]$WorkingDir,
        [hashtable]$Position,
        [string]$PostCommand
    )

    # Create the picker command that will run in the terminal
    $pickerScript = @"

# Change to projects directory
Set-Location '$WorkingDir'

# Get list of project directories
`$projects = Get-ChildItem -Directory | Select-Object -ExpandProperty Name

# Check if fzf is available
`$fzfPath = Get-Command fzf -ErrorAction SilentlyContinue

if (`$fzfPath) {
    # Use fzf for selection
    `$selected = `$projects | fzf --height=80% --reverse --border --prompt='Select project: '
} else {
    # Fallback to simple numbered menu
    Write-Host ''
    Write-Host '  Quickstart - Select a Project' -ForegroundColor Cyan
    Write-Host '  =============================' -ForegroundColor Cyan
    Write-Host ''

    for (`$i = 0; `$i -lt `$projects.Count; `$i++) {
        Write-Host "    [`$(`$i + 1)] `$(`$projects[`$i])"
    }

    Write-Host ''
    `$selection = Read-Host '  Enter number'

    `$index = [int]`$selection - 1
    if (`$index -ge 0 -and `$index -lt `$projects.Count) {
        `$selected = `$projects[`$index]
    }
}

if (`$selected) {
    `$projectPath = Join-Path '$WorkingDir' `$selected
    Set-Location `$projectPath

    Write-Host ''
    Write-Host "  Opening: `$selected" -ForegroundColor Green
    Write-Host ''

    # Run the post-select command
    $PostCommand
} else {
    Write-Host '  No project selected.' -ForegroundColor Red
    Write-Host '  Press any key to exit...'
    `$null = `$Host.UI.RawUI.ReadKey('NoEcho,IncludeKeyDown')
}
"@

    # Encode the script for passing to PowerShell
    $bytes = [System.Text.Encoding]::Unicode.GetBytes($pickerScript)
    $encodedCommand = [Convert]::ToBase64String($bytes)

    # Launch Windows Terminal
    $wtArgs = @(
        "--title", $Title,
        "-d", $WorkingDir,
        "powershell", "-NoExit", "-EncodedCommand", $encodedCommand
    )

    Start-Process "wt" -ArgumentList $wtArgs

    # Wait for window to appear
    Start-Sleep -Milliseconds 800

    # Find and position the window
    $maxAttempts = 10
    $hwnd = [IntPtr]::Zero

    for ($attempt = 0; $attempt -lt $maxAttempts; $attempt++) {
        $hwnd = [WinAPI]::FindWindowByTitle($Title)
        if ($hwnd -ne [IntPtr]::Zero) {
            break
        }
        Start-Sleep -Milliseconds 300
    }

    if ($hwnd -ne [IntPtr]::Zero) {
        $result = [WinAPI]::SetWindowPos(
            $hwnd,
            [IntPtr]::Zero,
            $Position.X,
            $Position.Y,
            $Position.Width,
            $Position.Height,
            [WinAPI]::SWP_NOZORDER -bor [WinAPI]::SWP_SHOWWINDOW
        )

        if ($Verbose) {
            Write-Host "  Positioned '$Title' at ($($Position.X), $($Position.Y)) - $($Position.Width)x$($Position.Height)"
        }
    } else {
        Write-Host "  Warning: Could not find window '$Title' to position" -ForegroundColor Yellow
    }
}

# ============================================================================
# Main
# ============================================================================

Write-Host ""
Write-Host "  Quickstart - Development Environment Launcher" -ForegroundColor Cyan
Write-Host "  =============================================" -ForegroundColor Cyan
Write-Host ""

# Detect monitors
$monitors = Get-Monitors

Write-Host "  Detected $($monitors.Count) monitor(s):" -ForegroundColor White
for ($i = 0; $i -lt $monitors.Count; $i++) {
    $m = $monitors[$i]
    $primary = if ($m.IsPrimary) { " (Primary)" } else { "" }
    Write-Host "    Monitor $($i + 1): $($m.Width)x$($m.Height) work area$primary"
}
Write-Host ""

# Handle --List: just show monitors and exit
if ($List) {
    Write-Host "  Usage: quickstart.ps1 -Windows '1,2,4' -ProjectsDir 'C:\dev'" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "  Example configurations:" -ForegroundColor White
    Write-Host "    -Windows '1'       # 1 window on monitor 1"
    Write-Host "    -Windows '1,2'     # 1 on monitor 1, 2 on monitor 2"
    Write-Host "    -Windows '1,2,4'   # 1 on monitor 1, 2 on monitor 2, 4 on monitor 3"
    Write-Host ""
    exit 0
}

# Handle --Init: interactive setup
if ($Init) {
    Write-Host "  Interactive Setup" -ForegroundColor Green
    Write-Host ""

    # Ask for projects directory
    $defaultDir = "$env:USERPROFILE\.1dev"
    $inputDir = Read-Host "  Projects directory [$defaultDir]"
    if ($inputDir -eq "") { $inputDir = $defaultDir }

    # Ask for windows per monitor
    Write-Host ""
    Write-Host "  How many terminal windows on each monitor?" -ForegroundColor White
    $windowConfig = @()
    for ($i = 0; $i -lt $monitors.Count; $i++) {
        $default = if ($i -eq 0) { 1 } elseif ($i -eq 1) { 2 } else { 4 }
        $input = Read-Host "    Monitor $($i + 1) [$default]"
        if ($input -eq "") { $input = $default }
        $windowConfig += $input
    }

    $windowsParam = $windowConfig -join ","

    Write-Host ""
    Write-Host "  To launch with this config, run:" -ForegroundColor Green
    Write-Host "    .\quickstart.ps1 -ProjectsDir '$inputDir' -Windows '$windowsParam'" -ForegroundColor Yellow
    Write-Host ""

    # Ask if they want to launch now
    $launch = Read-Host "  Launch now? [Y/n]"
    if ($launch -eq "" -or $launch.ToLower() -eq "y") {
        $Config.ProjectsDir = $inputDir
        $Config.Monitors = @{}
        for ($i = 0; $i -lt $windowConfig.Count; $i++) {
            $count = [int]$windowConfig[$i]
            $layout = if ($count -eq 1) { "full" } elseif ($count -le 2) { "vertical" } else { "grid" }
            $Config.Monitors[$i] = @{ Windows = $count; Layout = $layout }
        }
    } else {
        exit 0
    }
}


# Validate projects directory
if (-not (Test-Path $Config.ProjectsDir)) {
    Write-Host "  Error: Projects directory not found: $($Config.ProjectsDir)" -ForegroundColor Red
    Write-Host "  Please edit the script and set the correct ProjectsDir" -ForegroundColor Yellow
    exit 1
}

$projectCount = (Get-ChildItem -Path $Config.ProjectsDir -Directory).Count
Write-Host "  Projects directory: $($Config.ProjectsDir) ($projectCount projects)" -ForegroundColor White
Write-Host ""

# Launch terminals
$windowIndex = 0
foreach ($monitorIndex in $Config.Monitors.Keys | Sort-Object) {
    $monitorConfig = $Config.Monitors[$monitorIndex]

    if ($monitorIndex -ge $monitors.Count) {
        Write-Host "  Skipping monitor $monitorIndex (not connected)" -ForegroundColor Yellow
        continue
    }

    $monitor = $monitors[$monitorIndex]
    $positions = Get-WindowPositions -Monitor $monitor -WindowCount $monitorConfig.Windows -Layout $monitorConfig.Layout

    Write-Host "  Monitor $($monitorIndex + 1): Launching $($monitorConfig.Windows) window(s) in $($monitorConfig.Layout) layout" -ForegroundColor Green

    foreach ($pos in $positions) {
        $windowIndex++
        $title = "Quickstart-$windowIndex"

        Start-QuickstartTerminal `
            -Title $title `
            -WorkingDir $Config.ProjectsDir `
            -Position $pos `
            -PostCommand $Config.PostCommand

        # Small delay between launches to avoid race conditions
        Start-Sleep -Milliseconds 200
    }
}

Write-Host ""
Write-Host "  Launched $windowIndex terminal window(s)" -ForegroundColor Cyan
Write-Host ""
