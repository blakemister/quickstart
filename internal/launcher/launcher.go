package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/bcmister/qs/internal/monitor"
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
	Command    string            // executable name, e.g. "claude"
	Args       []string          // arguments, e.g. ["--dangerously-skip-permissions"]
	Env        map[string]string // extra env vars to inject (nil = inherit parent env as-is)
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

// LaunchTerminal launches a single terminal window using wt.exe.
// The command runs: wt.exe --title <title> -d <workingDir> <command> <args...>
func LaunchTerminal(cfg LaunchConfig) error {
	args := []string{"--title", cfg.Title, "-d", cfg.WorkingDir}
	args = append(args, cfg.Command)
	args = append(args, cfg.Args...)

	cmd := exec.Command("wt", args...)
	applyEnv(cmd, cfg.Env)
	return cmd.Start()
}

// LaunchAll launches all terminals in parallel and positions them.
func LaunchAll(configs []LaunchConfig) []LaunchResult {
	results := make([]LaunchResult, len(configs))

	// Phase 1: Launch all wt processes
	for i, cfg := range configs {
		results[i].Title = cfg.Title

		args := []string{"--title", cfg.Title, "-d", cfg.WorkingDir}
		args = append(args, cfg.Command)
		args = append(args, cfg.Args...)

		cmd := exec.Command("wt", args...)
		applyEnv(cmd, cfg.Env)
		if err := cmd.Start(); err != nil {
			results[i].Err = fmt.Errorf("failed to launch: %w", err)
		}
	}

	// Phase 2: Wait for windows to appear, then find and position all in parallel
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

// LaunchAllWithCurrent launches terminals where index 0 uses the current terminal
// and indexes 1+ spawn new windows. Each spawned window runs "wt.exe ... qs" so
// it gets its own picker TUI.
func LaunchAllWithCurrent(configs []LaunchConfig) LaunchAllWithCurrentResult {
	if len(configs) == 0 {
		return LaunchAllWithCurrentResult{
			Results: nil,
		}
	}

	results := make([]LaunchResult, len(configs))
	for i, cfg := range configs {
		results[i].Title = cfg.Title
	}

	// Get current console window handle for positioning
	currentHwnd := GetCurrentConsoleWindow()

	// Launch additional windows (configs[1:]) — each runs "qs" for its own picker
	if len(configs) > 1 {
		for i := 1; i < len(configs); i++ {
			cfg := configs[i]
			args := []string{"--title", cfg.Title, "-d", cfg.WorkingDir}
			args = append(args, cfg.Command)
			args = append(args, cfg.Args...)
			cmd := exec.Command("wt", args...)
			applyEnv(cmd, cfg.Env)
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

	return LaunchAllWithCurrentResult{
		Results: results,
	}
}

// LaunchAllWithCurrentResult holds the results of a LaunchAllWithCurrent call
type LaunchAllWithCurrentResult struct {
	Results []LaunchResult
}

// PositionCurrentWindow positions the current console window
func PositionCurrentWindow(x, y, w, h int) error {
	hwnd := GetCurrentConsoleWindow()
	if hwnd == 0 {
		return fmt.Errorf("could not get current console window")
	}
	return setWindowPosition(hwnd, x, y, w, h)
}

// GetCurrentConsoleWindow returns the HWND of the current console window
func GetCurrentConsoleWindow() uintptr {
	hwnd, _, _ := procGetConsoleWindow.Call()
	return hwnd
}

func findWindowByTitle(title string) (uintptr, error) {
	var foundHwnd uintptr

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

// applyEnv sets extra environment variables on a command.
// If env is nil or empty, the command inherits the parent environment as-is.
func applyEnv(cmd *exec.Cmd, env map[string]string) {
	if len(env) == 0 {
		return
	}
	cmd.Env = os.Environ()
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
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
