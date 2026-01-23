# Quickstart - Multi-monitor Terminal Launcher
# Launch multiple terminal windows across monitors for vibe coding with Claude
# https://github.com/blakemister/quickstart

param(
    [string]$ProjectsDir = "",       # Will prompt if not set
    [string]$PostCommand = "claude --dangerously-skip-permissions",
    [string]$Windows = "",           # Override: "1,2,4" means 1 on monitor 1, 2 on monitor 2, etc.
    [switch]$Init,                   # Run interactive setup
    [switch]$List,                   # Just list monitors and exit
    [switch]$Verbose
)

# ============================================================================
# Configuration
# ============================================================================

# Default: 1 window per monitor (safe default for any setup)
# Users should run with -Init or -Windows to customize
$DefaultMonitorConfig = @{
    0 = @{ Windows = 1; Layout = "full" }
    1 = @{ Windows = 1; Layout = "full" }
    2 = @{ Windows = 1; Layout = "full" }
    3 = @{ Windows = 1; Layout = "full" }
}

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
    public int Left;
    public int Top;
    public int Right;
    public int Bottom;
    public int WorkLeft;
    public int WorkTop;
    public int WorkRight;
    public int WorkBottom;
    public bool IsPrimary;

    public int Width { get { return WorkRight - WorkLeft; } }
    public int Height { get { return WorkBottom - WorkTop; } }
    public int X { get { return WorkLeft; } }
    public int Y { get { return WorkTop; } }
}

public class WinAPI {
    [DllImport("user32.dll")]
    public static extern bool EnumDisplayMonitors(IntPtr hdc, IntPtr lprcClip, MonitorEnumDelegate lpfnEnum, IntPtr dwData);

    [DllImport("user32.dll", CharSet = CharSet.Auto)]
    public static extern bool GetMonitorInfo(IntPtr hMonitor, ref MONITORINFOEX lpmi);

    [DllImport("user32.dll")]
    public static extern bool EnumWindows(EnumWindowsProc lpEnumFunc, IntPtr lParam);

    [DllImport("user32.dll", CharSet = CharSet.Auto, SetLastError = true)]
    public static extern int GetWindowText(IntPtr hWnd, System.Text.StringBuilder lpString, int nMaxCount);

    [DllImport("user32.dll", SetLastError = true)]
    public static extern bool SetWindowPos(IntPtr hWnd, IntPtr hWndInsertAfter, int X, int Y, int cx, int cy, uint uFlags);

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
                        WorkLeft = mi.rcWork.Left,
                        WorkTop = mi.rcWork.Top,
                        WorkRight = mi.rcWork.Right,
                        WorkBottom = mi.rcWork.Bottom,
                        IsPrimary = (mi.dwFlags & MONITORINFOF_PRIMARY) != 0
                    });
                }
                return true;
            }, IntPtr.Zero);

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
                return false;
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
    return [WinAPI]::GetMonitors()
}

function Write-PickerScript {
    param([string]$WorkingDir, [string]$PostCommand)

    # Create a picker script file (avoids escaping issues with WT)
    $scriptPath = Join-Path $env:TEMP "quickstart-picker.ps1"

    $scriptContent = @"
Set-Location '$WorkingDir'
`$projects = Get-ChildItem -Directory | Select-Object -ExpandProperty Name

if (`$projects.Count -eq 0) {
    Write-Host ''
    Write-Host '  No project folders found in: $WorkingDir' -ForegroundColor Red
    Write-Host ''
    Read-Host '  Press Enter to exit'
    exit
}

`$hasFzf = Get-Command fzf -ErrorAction SilentlyContinue

if (`$hasFzf) {
    `$selected = `$projects | fzf --height=80% --reverse --border --prompt='Select project: '
} else {
    Write-Host ''
    Write-Host '  Select a Project' -ForegroundColor Cyan
    Write-Host ''
    for (`$i = 0; `$i -lt `$projects.Count; `$i++) {
        Write-Host ('  [' + (`$i + 1) + '] ' + `$projects[`$i])
    }
    Write-Host ''
    `$num = Read-Host '  Number'
    if (`$num -match '^\d+`$' -and [int]`$num -ge 1 -and [int]`$num -le `$projects.Count) {
        `$selected = `$projects[[int]`$num - 1]
    }
}

if (`$selected) {
    Set-Location `$selected
    Write-Host ''
    Write-Host "  Opening: `$selected" -ForegroundColor Green
    Write-Host ''
    $PostCommand
}
"@

    Set-Content -Path $scriptPath -Value $scriptContent -Force
    return $scriptPath
}

# Launch terminal(s) on a monitor using WT's native pane splitting
function Start-MonitorTerminals {
    param(
        [string]$WindowName,
        [string]$WorkingDir,
        [MonitorInfo]$Monitor,
        [int]$PaneCount,
        [string]$Layout,
        [string]$PostCommand
    )

    # Write picker script to temp file (avoids WT escaping issues)
    $pickerScript = Write-PickerScript -WorkingDir $WorkingDir -PostCommand $PostCommand

    # Position on correct monitor by using --pos with a point inside that monitor
    $posX = $Monitor.X + 100
    $posY = $Monitor.Y + 100

    # Base command to run the picker script
    $paneCmd = "powershell -ExecutionPolicy Bypass -NoExit -File `"$pickerScript`""

    if ($PaneCount -eq 1) {
        # Single pane - just maximize on the monitor
        $wtArgs = "--pos $posX,$posY -M --title `"$WindowName`" $paneCmd"
        Start-Process "wt" -ArgumentList $wtArgs
    }
    else {
        # Multiple panes - build a command with splits
        # WT uses ; to separate commands, sp for split-pane
        # -V = vertical split (side by side), -H = horizontal split (stacked)

        $wtCmd = "--pos $posX,$posY -M --title `"$WindowName`" $paneCmd"

        if ($Layout -eq "vertical" -or $PaneCount -eq 2) {
            # 2 panes side by side
            for ($i = 1; $i -lt $PaneCount; $i++) {
                $wtCmd += " `; sp -V $paneCmd"
            }
        }
        elseif ($Layout -eq "horizontal") {
            # Stacked horizontally
            for ($i = 1; $i -lt $PaneCount; $i++) {
                $wtCmd += " `; sp -H $paneCmd"
            }
        }
        elseif ($Layout -eq "grid") {
            # Grid layout: 4 panes = 2x2, 6 panes = 2x3, etc.
            if ($PaneCount -eq 4) {
                # Create 2x2 grid: split vertical, then split each horizontal
                $wtCmd = "--pos $posX,$posY -M --title `"$WindowName`" $paneCmd"
                $wtCmd += " `; sp -V $paneCmd"
                $wtCmd += " `; mf left `; sp -H $paneCmd"
                $wtCmd += " `; mf right `; sp -H $paneCmd"
            }
            elseif ($PaneCount -eq 6) {
                # 2x3 grid
                $wtCmd = "--pos $posX,$posY -M --title `"$WindowName`" $paneCmd"
                $wtCmd += " `; sp -V $paneCmd"
                $wtCmd += " `; sp -V $paneCmd"
                $wtCmd += " `; mf first `; sp -H $paneCmd"
                $wtCmd += " `; mf previous `; sp -H $paneCmd"
                $wtCmd += " `; mf last `; sp -H $paneCmd"
            }
            else {
                # Fallback: just do vertical splits
                for ($i = 1; $i -lt $PaneCount; $i++) {
                    $wtCmd += " `; sp -V $paneCmd"
                }
            }
        }

        Start-Process "wt" -ArgumentList $wtCmd
    }

    # Brief pause to let window open
    Start-Sleep -Milliseconds 800

    if ($Verbose) {
        Write-Host "  Launched '$WindowName' with $PaneCount pane(s) on monitor at ($($Monitor.X), $($Monitor.Y))"
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
    Write-Host "    Monitor $($i + 1): $($m.Width)x$($m.Height)$primary"
}
Write-Host ""

# Handle --List
if ($List) {
    Write-Host "  Usage Examples:" -ForegroundColor Yellow
    Write-Host "    quickstart -Init                          # Interactive setup"
    Write-Host "    quickstart -ProjectsDir 'C:\dev'          # Specify projects folder"
    Write-Host "    quickstart -Windows '1,2,4'               # 1 + 2 + 4 panes across 3 monitors"
    Write-Host "    quickstart -ProjectsDir 'C:\dev' -Windows '2,4'"
    Write-Host ""
    exit 0
}

# Handle --Init
if ($Init) {
    Write-Host "  Interactive Setup" -ForegroundColor Green
    Write-Host ""

    Write-Host "  Where are your project folders located?"
    Write-Host "  (This is the folder containing all your project subfolders)"
    Write-Host ""

    $inputDir = $null
    while ([string]::IsNullOrWhiteSpace($inputDir)) {
        $inputDir = Read-Host "  Projects directory (e.g., C:\dev)"

        if ([string]::IsNullOrWhiteSpace($inputDir)) {
            Write-Host "  Please enter a directory path" -ForegroundColor Yellow
            continue
        }

        if (-not (Test-Path -Path $inputDir -ErrorAction SilentlyContinue)) {
            Write-Host "  Directory doesn't exist. Create it? [Y/n]" -ForegroundColor Yellow
            $create = Read-Host
            if ([string]::IsNullOrWhiteSpace($create) -or $create -eq "y" -or $create -eq "Y") {
                New-Item -ItemType Directory -Path $inputDir -Force | Out-Null
                Write-Host "  Created: $inputDir" -ForegroundColor Green
            } else {
                Write-Host "  Please enter an existing directory" -ForegroundColor Yellow
                $inputDir = $null
            }
        }
    }

    Write-Host ""
    Write-Host "  How many terminal panes on each monitor?" -ForegroundColor White
    $windowConfig = @()
    for ($i = 0; $i -lt $monitors.Count; $i++) {
        $m = $monitors[$i]
        $input = Read-Host "    Monitor $($i + 1) ($($m.Width)x$($m.Height)) [1]"
        if ([string]::IsNullOrWhiteSpace($input)) { $input = "1" }
        $windowConfig += $input
    }

    $windowsParam = $windowConfig -join ","

    Write-Host ""
    Write-Host "  Your command:" -ForegroundColor Green
    Write-Host "    quickstart -ProjectsDir '$inputDir' -Windows '$windowsParam'" -ForegroundColor Yellow
    Write-Host ""

    $launch = Read-Host "  Launch now? [Y/n]"
    if ([string]::IsNullOrWhiteSpace($launch) -or $launch -eq "y" -or $launch -eq "Y") {
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
$needsProjectsDir = [string]::IsNullOrWhiteSpace($Config.ProjectsDir) -or -not (Test-Path -Path $Config.ProjectsDir -ErrorAction SilentlyContinue)

if ($needsProjectsDir) {
    if (-not [string]::IsNullOrWhiteSpace($Config.ProjectsDir)) {
        Write-Host "  Projects directory not found: $($Config.ProjectsDir)" -ForegroundColor Yellow
    }
    Write-Host ""
    $inputDir = Read-Host "  Enter your projects directory (e.g., C:\dev)"

    if ([string]::IsNullOrWhiteSpace($inputDir)) {
        Write-Host "  Error: Projects directory is required" -ForegroundColor Red
        Write-Host "  Run 'quickstart -Init' for guided setup" -ForegroundColor Yellow
        exit 1
    }

    if (-not (Test-Path -Path $inputDir -ErrorAction SilentlyContinue)) {
        Write-Host "  Error: Directory does not exist: $inputDir" -ForegroundColor Red
        Write-Host "  Run 'quickstart -Init' for guided setup" -ForegroundColor Yellow
        exit 1
    }

    $Config.ProjectsDir = $inputDir
}

$projectCount = (Get-ChildItem -Path $Config.ProjectsDir -Directory).Count
Write-Host "  Projects directory: $($Config.ProjectsDir) ($projectCount projects)" -ForegroundColor White
Write-Host ""

# Launch terminals on each monitor
$totalPanes = 0
foreach ($monitorIndex in $Config.Monitors.Keys | Sort-Object) {
    if ($monitorIndex -ge $monitors.Count) {
        continue  # Skip if monitor doesn't exist
    }

    $monitorConfig = $Config.Monitors[$monitorIndex]
    $monitor = $monitors[$monitorIndex]
    $paneCount = $monitorConfig.Windows
    $layout = $monitorConfig.Layout

    Write-Host "  Monitor $($monitorIndex + 1): Launching $paneCount pane(s)" -ForegroundColor Green

    Start-MonitorTerminals `
        -WindowName "Quickstart-Monitor$($monitorIndex + 1)" `
        -WorkingDir $Config.ProjectsDir `
        -Monitor $monitor `
        -PaneCount $paneCount `
        -Layout $layout `
        -PostCommand $Config.PostCommand

    $totalPanes += $paneCount

    # Delay between monitors
    Start-Sleep -Milliseconds 500
}

Write-Host ""
Write-Host "  Launched $totalPanes total pane(s) across $($monitors.Count) monitor(s)" -ForegroundColor Cyan
Write-Host ""
