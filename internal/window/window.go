package window

import (
	"encoding/base64"
	"fmt"
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
	return "$d = '" + workingDir + "'\n" +
		"$p = Get-ChildItem $d -Directory\n" +
		"if ($p.Count -eq 0) {\n" +
		"    Write-Host 'No projects found in' $d\n" +
		"    Read-Host 'Press Enter to exit'\n" +
		"    exit\n" +
		"}\n" +
		"Write-Host ''\n" +
		"Write-Host ('Projects in ' + $d + ':') -ForegroundColor Cyan\n" +
		"Write-Host ''\n" +
		"$i = 1\n" +
		"$p | ForEach-Object { Write-Host ('  [' + $i + '] ' + $_.Name); $i++ }\n" +
		"Write-Host ''\n" +
		"$s = Read-Host 'Pick'\n" +
		"$idx = [int]$s - 1\n" +
		"if ($idx -lt 0 -or $idx -ge $p.Count) {\n" +
		"    Write-Host 'Invalid selection.' -ForegroundColor Red\n" +
		"    Read-Host 'Press Enter to exit'\n" +
		"    exit\n" +
		"}\n" +
		"$t = $p[$idx].FullName\n" +
		"Set-Location $t\n" +
		"Write-Host ''\n" +
		"Write-Host ('Opening ' + $p[$idx].Name + '...') -ForegroundColor Green\n" +
		"Write-Host ''\n" +
		command + "\n"
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
