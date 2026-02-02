package window

import (
	"encoding/base64"
	"fmt"
	"os/exec"
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
	SWP_NOZORDER  = 0x0004
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
	Command    string // command to run after project selection
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
// for use with powershell -EncodedCommand, which avoids all quoting/escaping issues
func encodePS(script string) string {
	u16 := utf16.Encode([]rune(script))
	b := make([]byte, len(u16)*2)
	for i, r := range u16 {
		b[i*2] = byte(r)
		b[i*2+1] = byte(r >> 8)
	}
	return base64.StdEncoding.EncodeToString(b)
}

// LaunchTerminal launches a Windows Terminal window with an inline project picker
func LaunchTerminal(cfg LaunchConfig) error {
	// Build the picker script as a clean multi-line PowerShell script
	// Using EncodedCommand avoids wt treating ';' as tab separators
	// and avoids cmd/start argument mangling
	script := "$d = '" + cfg.WorkingDir + "'\n" +
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
		cfg.Command + "\n"

	encoded := encodePS(script)

	// Launch wt directly (not through cmd /c start) to avoid argument mangling
	// No -w flag so each terminal is a separate window
	args := []string{
		"--title", cfg.Title,
		"-d", cfg.WorkingDir,
		"powershell", "-NoExit", "-EncodedCommand", encoded,
	}

	cmd := exec.Command("wt", args...)
	err := cmd.Start()
	if err != nil {
		return fmt.Errorf("failed to launch terminal: %w", err)
	}

	// Wait for window to appear
	time.Sleep(500 * time.Millisecond)

	// Find and position the window
	hwnd, err := findWindowByTitle(cfg.Title)
	if err != nil {
		return fmt.Errorf("failed to find window: %w", err)
	}

	err = setWindowPosition(hwnd, cfg.X, cfg.Y, cfg.Width, cfg.Height)
	if err != nil {
		return fmt.Errorf("failed to position window: %w", err)
	}

	return nil
}

func findWindowByTitle(title string) (uintptr, error) {
	var foundHwnd uintptr

	for attempts := 0; attempts < 10; attempts++ {
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

		time.Sleep(200 * time.Millisecond)
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
