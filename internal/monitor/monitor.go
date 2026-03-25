package monitor

import (
	"fmt"
	"syscall"
	"unsafe"
)

// Monitor represents a display monitor
type Monitor struct {
	Name    string
	X       int
	Y       int
	Width   int
	Height  int
	Primary bool
}

var (
	user32                  = syscall.NewLazyDLL("user32.dll")
	procEnumDisplayMonitors = user32.NewProc("EnumDisplayMonitors")
	procGetMonitorInfoW     = user32.NewProc("GetMonitorInfoW")
)

// RECT structure
type rect struct {
	Left, Top, Right, Bottom int32
}

// MONITORINFOEXW structure
type monitorInfoExW struct {
	CbSize    uint32
	RcMonitor rect
	RcWork    rect
	DwFlags   uint32
	SzDevice  [32]uint16
}

const (
	MONITORINFOF_PRIMARY = 0x00000001
)

// Detect returns a list of all connected monitors
func Detect() ([]Monitor, error) {
	var monitors []Monitor

	// Callback function for EnumDisplayMonitors
	callback := syscall.NewCallback(func(hMonitor uintptr, hdcMonitor uintptr, lprcMonitor uintptr, dwData uintptr) uintptr {
		var info monitorInfoExW
		info.CbSize = uint32(unsafe.Sizeof(info))

		ret, _, _ := procGetMonitorInfoW.Call(
			hMonitor,
			uintptr(unsafe.Pointer(&info)),
		)

		if ret != 0 {
			// Convert device name from UTF16 to string
			deviceName := syscall.UTF16ToString(info.SzDevice[:])

			m := Monitor{
				Name:    deviceName,
				X:       int(info.RcMonitor.Left),
				Y:       int(info.RcMonitor.Top),
				Width:   int(info.RcMonitor.Right - info.RcMonitor.Left),
				Height:  int(info.RcMonitor.Bottom - info.RcMonitor.Top),
				Primary: info.DwFlags&MONITORINFOF_PRIMARY != 0,
			}

			// Generate a friendly name if device name is technical
			if m.Name == "" || m.Name[0] == '\\' {
				m.Name = fmt.Sprintf("Display %d", len(monitors)+1)
			}

			monitors = append(monitors, m)
		}

		return 1 // Continue enumeration
	})

	ret, _, err := procEnumDisplayMonitors.Call(
		0,        // hdc - NULL for all monitors
		0,        // lprcClip - NULL for entire virtual screen
		callback, // lpfnEnum
		0,        // dwData
	)

	if ret == 0 {
		return nil, fmt.Errorf("EnumDisplayMonitors failed: %v", err)
	}

	// Sort monitors by X position (left to right)
	for i := 0; i < len(monitors)-1; i++ {
		for j := i + 1; j < len(monitors); j++ {
			if monitors[j].X < monitors[i].X {
				monitors[i], monitors[j] = monitors[j], monitors[i]
			}
		}
	}

	// Assign friendly names based on position
	for i := range monitors {
		if monitors[i].Primary {
			monitors[i].Name = fmt.Sprintf("Monitor %d (Primary)", i+1)
		} else {
			monitors[i].Name = fmt.Sprintf("Monitor %d", i+1)
		}
	}

	return monitors, nil
}

// GetPrimary returns the primary monitor
func GetPrimary() (*Monitor, error) {
	monitors, err := Detect()
	if err != nil {
		return nil, err
	}

	for i := range monitors {
		if monitors[i].Primary {
			return &monitors[i], nil
		}
	}

	// If no primary found, return the first one
	if len(monitors) > 0 {
		return &monitors[0], nil
	}

	return nil, fmt.Errorf("no monitors detected")
}
