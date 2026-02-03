package window

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"

	"github.com/bcmister/qk/internal/monitor"
)

var (
	user32             = syscall.NewLazyDLL("user32.dll")
	procFindWindowW    = user32.NewProc("FindWindowW")
	procSetWindowPos   = user32.NewProc("SetWindowPos")
	procEnumWindows    = user32.NewProc("EnumWindows")
	procGetWindowTextW = user32.NewProc("GetWindowTextW")

	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleWindow = kernel32.NewProc("GetConsoleWindow")
)

const (
	SWP_NOZORDER   = 0x0004
	SWP_SHOWWINDOW = 0x0040
	HWND_TOP       = 0
)

// Position represents a window position and size
type Position struct {
	X      int
	Y      int
	Width  int
	Height int
}

// LaunchConfig holds configuration for launching a terminal
type LaunchConfig struct {
	Title      string
	WorkingDir string
	X          int
	Y          int
	Width      int
	Height     int
}

// LaunchResult holds the outcome of a terminal launch
type LaunchResult struct {
	Title string
	Err   error
}

// CalculateLayout calculates window positions based on layout type
func CalculateLayout(mon *monitor.Monitor, count int, layout string) []Position {
	switch layout {
	case "grid":
		return calculateGrid(mon, count)
	case "vertical":
		return calculateVertical(mon, count)
	case "horizontal":
		return calculateHorizontal(mon, count)
	case "full":
		return []Position{{
			X:      mon.X,
			Y:      mon.Y,
			Width:  mon.Width,
			Height: mon.Height,
		}}
	default:
		return calculateGrid(mon, count)
	}
}

func calculateGrid(mon *monitor.Monitor, count int) []Position {
	positions := make([]Position, count)

	cols := 1
	rows := 1
	for cols*rows < count {
		if cols <= rows {
			cols++
		} else {
			rows++
		}
	}

	cellWidth := mon.Width / cols
	cellHeight := mon.Height / rows

	for i := 0; i < count; i++ {
		row := i / cols
		col := i % cols
		positions[i] = Position{
			X:      mon.X + (col * cellWidth),
			Y:      mon.Y + (row * cellHeight),
			Width:  cellWidth,
			Height: cellHeight,
		}
	}

	return positions
}

func calculateVertical(mon *monitor.Monitor, count int) []Position {
	positions := make([]Position, count)
	cellWidth := mon.Width / count

	for i := 0; i < count; i++ {
		positions[i] = Position{
			X:      mon.X + (i * cellWidth),
			Y:      mon.Y,
			Width:  cellWidth,
			Height: mon.Height,
		}
	}

	return positions
}

func calculateHorizontal(mon *monitor.Monitor, count int) []Position {
	positions := make([]Position, count)
	cellHeight := mon.Height / count

	for i := 0; i < count; i++ {
		positions[i] = Position{
			X:      mon.X,
			Y:      mon.Y + (i * cellHeight),
			Width:  mon.Width,
			Height: cellHeight,
		}
	}

	return positions
}

// encodePS converts a PowerShell script to a base64 UTF-16LE encoded string
func encodePS(script string) string {
	u16 := utf16.Encode([]rune(script))
	b := make([]byte, len(u16)*2)
	for i, r := range u16 {
		b[i*2] = byte(r)
		b[i*2+1] = byte(r >> 8)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// LaunchAll launches all terminals in parallel and positions them
func LaunchAll(configs []LaunchConfig, command string) []LaunchResult {
	results := make([]LaunchResult, len(configs))

	// Pre-encode the picker script once (same for all terminals)
	// Each terminal gets its own title but same picker logic
	scripts := make([]string, len(configs))
	for i, cfg := range configs {
		scripts[i] = buildPickerScript(cfg.WorkingDir, command)
		_ = scripts[i] // used below
		results[i].Title = cfg.Title
	}

	// Phase 1: Launch all wt processes as fast as possible
	for i, cfg := range configs {
		encoded := encodePS(scripts[i])
		args := []string{
			"--title", cfg.Title,
			"-d", cfg.WorkingDir,
			"powershell", "-NoExit", "-EncodedCommand", encoded,
		}
		cmd := exec.Command("wt", args...)
		if err := cmd.Start(); err != nil {
			results[i].Err = fmt.Errorf("failed to launch: %w", err)
		}
	}

	// Phase 2: Wait once for windows to start appearing, then find and position all in parallel
	time.Sleep(300 * time.Millisecond)

	var wg sync.WaitGroup
	for i, cfg := range configs {
		if results[i].Err != nil {
			continue
		}
		wg.Add(1)
		go func(idx int, c LaunchConfig) {
			defer wg.Done()
			hwnd, err := findWindowByTitle(c.Title)
			if err != nil {
				results[idx].Err = fmt.Errorf("failed to find window: %w", err)
				return
			}
			if err := setWindowPosition(hwnd, c.X, c.Y, c.Width, c.Height); err != nil {
				results[idx].Err = fmt.Errorf("failed to position: %w", err)
			}
		}(i, cfg)
	}
	wg.Wait()

	return results
}

func buildPickerScript(workingDir, command string) string {
	// Interactive arrow-key picker with fuzzy filtering
	// Inspired by fzf/gum - navigate with arrows, type to filter, enter to select
	return `
$R   = [char]27 + '[0m'
$DIM = [char]27 + '[90m'
$CYN = [char]27 + '[96m'
$WHT = [char]27 + '[97m'
$GRN = [char]27 + '[92m'
$YEL = [char]27 + '[93m'
$RED = [char]27 + '[91m'
$INV = [char]27 + '[7m'
$HID = [char]27 + '[?25l'
$SHW = [char]27 + '[?25h'

$d = '` + workingDir + `'
$all = @(Get-ChildItem $d -Directory | Select-Object -ExpandProperty Name)

if ($all.Count -eq 0) {
    Write-Host ""
    Write-Host "  ${RED}No projects in $d${R}"
    Write-Host ""
    Read-Host "  Press Enter"
    exit
}

$filter = ""
$sel = 0
$maxShow = [Math]::Min(12, $all.Count)

function Draw {
    param($items, $sel, $filter, $startY)

    $pos = $Host.UI.RawUI.CursorPosition
    $pos.Y = $startY
    $Host.UI.RawUI.CursorPosition = $pos

    # Header with filter
    if ($filter -eq "") {
        Write-Host "  ${CYN}>${R} ${DIM}type to filter...${R}                    " -NoNewline
    } else {
        Write-Host "  ${CYN}>${R} ${WHT}$filter${R}                              " -NoNewline
    }
    Write-Host ""
    Write-Host "  ${DIM}─────────────────────────────────${R}      "

    # Items
    $count = [Math]::Min($maxShow, $items.Count)
    for ($i = 0; $i -lt $maxShow; $i++) {
        if ($i -lt $items.Count) {
            $name = $items[$i]
            if ($i -eq $sel) {
                Write-Host "  ${INV}${CYN} > ${WHT}$name ${R}                              "
            } else {
                Write-Host "    ${DIM}$name${R}                                   "
            }
        } else {
            Write-Host "                                          "
        }
    }

    # Footer
    Write-Host ""
    Write-Host "  ${DIM}↑↓${R} navigate  ${DIM}enter${R} select  ${DIM}esc${R} quit     "
}

function FilterList {
    param($items, $query)
    if ($query -eq "") { return $items }
    $q = $query.ToLower()
    return @($items | Where-Object { $_.ToLower().Contains($q) })
}

# Setup
Clear-Host
Write-Host "${HID}" -NoNewline
Write-Host ""
Write-Host "  ${CYN}qk${R} ${DIM}· select project${R}"
Write-Host ""

$startY = $Host.UI.RawUI.CursorPosition.Y
$filtered = $all
Draw $filtered $sel $filter $startY

# Main loop
while ($true) {
    $key = $Host.UI.RawUI.ReadKey('NoEcho,IncludeKeyDown')
    $vk = $key.VirtualKeyCode
    $ch = $key.Character

    # Escape - quit
    if ($vk -eq 27) {
        Write-Host "${SHW}" -NoNewline
        Clear-Host
        exit
    }

    # Enter - select
    if ($vk -eq 13) {
        if ($filtered.Count -gt 0) {
            $chosen = $filtered[$sel]
            Write-Host "${SHW}" -NoNewline
            Clear-Host
            Write-Host ""
            Write-Host "  ${GRN}>${R} ${WHT}$chosen${R}"
            Write-Host ""
            Set-Location (Join-Path $d $chosen)
            ` + command + `
            break
        }
    }

    # Up arrow
    if ($vk -eq 38) {
        if ($sel -gt 0) { $sel-- }
        Draw $filtered $sel $filter $startY
        continue
    }

    # Down arrow
    if ($vk -eq 40) {
        if ($sel -lt ($filtered.Count - 1) -and $sel -lt ($maxShow - 1)) { $sel++ }
        Draw $filtered $sel $filter $startY
        continue
    }

    # Backspace
    if ($vk -eq 8) {
        if ($filter.Length -gt 0) {
            $filter = $filter.Substring(0, $filter.Length - 1)
            $filtered = FilterList $all $filter
            $sel = 0
            Draw $filtered $sel $filter $startY
        }
        continue
    }

    # Printable character - add to filter
    if ($ch -match '[\w\-\._]') {
        $filter += $ch
        $filtered = FilterList $all $filter
        $sel = 0
        Draw $filtered $sel $filter $startY
    }
}
`
}

// LaunchTab opens a new tab in the current Windows Terminal window
func LaunchTab(workingDir, command string) error {
	script := buildPickerScript(workingDir, command)
	encoded := encodePS(script)
	args := []string{
		"-w", "0",
		"new-tab",
		"-d", workingDir,
		"powershell", "-NoExit", "-EncodedCommand", encoded,
	}
	cmd := exec.Command("wt", args...)
	return cmd.Start()
}

// LaunchTerminal launches a single terminal (kept for backward compat)
func LaunchTerminal(cfg LaunchConfig, command string) error {
	results := LaunchAll([]LaunchConfig{cfg}, command)
	return results[0].Err
}

func findWindowByTitle(title string) (uintptr, error) {
	var foundHwnd uintptr

	// Poll at 50ms intervals instead of 200ms — find window as soon as it appears
	for attempts := 0; attempts < 40; attempts++ {
		callback := syscall.NewCallback(func(hwnd uintptr, lParam uintptr) uintptr {
			var windowTitle [256]uint16
			procGetWindowTextW.Call(hwnd, uintptr(unsafe.Pointer(&windowTitle[0])), 256)

			text := syscall.UTF16ToString(windowTitle[:])
			if text == title || containsSubstring(text, title) {
				foundHwnd = hwnd
				return 0
			}
			return 1
		})

		procEnumWindows.Call(callback, 0)

		if foundHwnd != 0 {
			return foundHwnd, nil
		}

		time.Sleep(50 * time.Millisecond)
	}

	return 0, fmt.Errorf("window with title '%s' not found", title)
}

func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func setWindowPosition(hwnd uintptr, x, y, width, height int) error {
	ret, _, err := procSetWindowPos.Call(
		hwnd,
		HWND_TOP,
		uintptr(x),
		uintptr(y),
		uintptr(width),
		uintptr(height),
		SWP_NOZORDER|SWP_SHOWWINDOW,
	)

	if ret == 0 {
		return fmt.Errorf("SetWindowPos failed: %v", err)
	}

	return nil
}

// GetCurrentConsoleWindow returns the HWND of the current console window
func GetCurrentConsoleWindow() uintptr {
	hwnd, _, _ := procGetConsoleWindow.Call()
	return hwnd
}

// RunPickerInCurrent runs the picker script in the current terminal (blocking)
func RunPickerInCurrent(workingDir, command string) error {
	script := buildPickerScript(workingDir, command)
	encoded := encodePS(script)

	cmd := exec.Command("powershell", "-NoExit", "-EncodedCommand", encoded)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// LaunchAllWithCurrentResult holds the results and a picker function
type LaunchAllWithCurrentResult struct {
	Results    []LaunchResult
	RunPicker  func() error
}

// LaunchAllWithCurrent launches terminals where index 0 uses the current terminal
// and indexes 1+ spawn new windows. Returns results and a picker function to run last.
func LaunchAllWithCurrent(configs []LaunchConfig, command string) LaunchAllWithCurrentResult {
	if len(configs) == 0 {
		return LaunchAllWithCurrentResult{
			Results:   nil,
			RunPicker: func() error { return nil },
		}
	}

	results := make([]LaunchResult, len(configs))
	for i, cfg := range configs {
		results[i].Title = cfg.Title
	}

	// Get current console window handle for positioning
	currentHwnd := GetCurrentConsoleWindow()

	// Launch additional windows (configs[1:]) via wt
	if len(configs) > 1 {
		for i := 1; i < len(configs); i++ {
			cfg := configs[i]
			script := buildPickerScript(cfg.WorkingDir, command)
			encoded := encodePS(script)
			args := []string{
				"--title", cfg.Title,
				"-d", cfg.WorkingDir,
				"powershell", "-NoExit", "-EncodedCommand", encoded,
			}
			cmd := exec.Command("wt", args...)
			if err := cmd.Start(); err != nil {
				results[i].Err = fmt.Errorf("failed to launch: %w", err)
			}
		}

		// Wait for windows to appear
		time.Sleep(300 * time.Millisecond)
	}

	// Position all windows in parallel
	var wg sync.WaitGroup

	// Position current terminal (index 0)
	if currentHwnd != 0 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cfg := configs[0]
			if err := setWindowPosition(currentHwnd, cfg.X, cfg.Y, cfg.Width, cfg.Height); err != nil {
				results[0].Err = fmt.Errorf("failed to position current window: %w", err)
			}
		}()
	}

	// Position spawned windows (configs[1:])
	for i := 1; i < len(configs); i++ {
		if results[i].Err != nil {
			continue
		}
		wg.Add(1)
		go func(idx int, cfg LaunchConfig) {
			defer wg.Done()
			hwnd, err := findWindowByTitle(cfg.Title)
			if err != nil {
				results[idx].Err = fmt.Errorf("failed to find window: %w", err)
				return
			}
			if err := setWindowPosition(hwnd, cfg.X, cfg.Y, cfg.Width, cfg.Height); err != nil {
				results[idx].Err = fmt.Errorf("failed to position: %w", err)
			}
		}(i, configs[i])
	}
	wg.Wait()

	// Return results and a picker function to call last
	picker := func() error {
		return RunPickerInCurrent(configs[0].WorkingDir, command)
	}

	return LaunchAllWithCurrentResult{
		Results:   results,
		RunPicker: picker,
	}
}
