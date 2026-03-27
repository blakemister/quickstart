package tui

import (
	"fmt"
	"strings"

	"github.com/bcmister/qs/internal/config"
	"github.com/bcmister/qs/internal/monitor"
	tea "github.com/charmbracelet/bubbletea"
)

// AllModel is the TUI for `qs all` — configure window counts per monitor.
type AllModel struct {
	cfg      *config.Config
	monitors []monitor.Monitor
	width    int
	height   int

	// Monitor step
	monitorIdx   int
	windowCounts []int

	// Results
	confirmed bool
	quitting  bool
}

// NewAll creates the AllModel TUI.
func NewAll(cfg *config.Config) AllModel {
	return AllModel{
		cfg: cfg,
	}
}

// Cancelled returns true if the user quit without confirming.
func (m AllModel) Cancelled() bool { return m.quitting }

// Confirmed returns true if the user pressed Enter to launch.
func (m AllModel) Confirmed() bool { return m.confirmed }

// WindowCounts returns the per-monitor window counts.
func (m AllModel) WindowCounts() []int { return m.windowCounts }

func (m AllModel) Init() tea.Cmd {
	return detectMonitors
}

func (m AllModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case monitorsDetectedMsg:
		m.monitors = msg.monitors
		m.windowCounts = make([]int, len(m.monitors))
		for i := range m.windowCounts {
			m.windowCounts[i] = 2
			if i < len(m.cfg.Monitors) {
				count := m.cfg.Monitors[i].WindowCount()
				if count >= 1 {
					m.windowCounts[i] = count
				}
			}
		}
		return m, nil

	case tea.KeyMsg:
		return m.updateMonitors(msg)
	}

	return m, nil
}

func (m AllModel) updateMonitors(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case msg.Type == tea.KeyEsc || msg.Type == tea.KeyCtrlC:
		m.quitting = true
		return m, tea.Quit
	case msg.Type == tea.KeyEnter:
		if len(m.monitors) == 0 {
			return m, nil
		}
		m.confirmed = true
		return m, tea.Quit
	case msg.String() == "left" || msg.String() == "h":
		if m.monitorIdx > 0 {
			m.monitorIdx--
		}
	case msg.String() == "right" || msg.String() == "l":
		if m.monitorIdx < len(m.monitors)-1 {
			m.monitorIdx++
		}
	case msg.Type == tea.KeyUp:
		if len(m.monitors) > 0 && m.windowCounts[m.monitorIdx] < 9 {
			m.windowCounts[m.monitorIdx]++
		}
	case msg.Type == tea.KeyDown:
		if len(m.monitors) > 0 && m.windowCounts[m.monitorIdx] > 1 {
			m.windowCounts[m.monitorIdx]--
		}
	}
	return m, nil
}

func (m AllModel) View() string {
	if m.quitting || m.confirmed {
		return ""
	}
	return m.viewMonitors()
}

func (m AllModel) viewMonitors() string {
	var s strings.Builder

	s.WriteString(RenderLogo("all"))
	s.WriteString(RenderSep())
	s.WriteString("\n")
	s.WriteString("  " + SubtitleStyle.Render("Configure windows per monitor") + "\n\n")

	if len(m.monitors) == 0 {
		s.WriteString("  " + DimStyle.Render("Detecting monitors...") + "\n")
		return s.String()
	}

	total := 0
	for _, count := range m.windowCounts {
		total += count
	}

	s.WriteString(fmt.Sprintf("  %s %d monitors detected, %d total windows\n\n",
		SuccessStyle.Render("*"),
		len(m.monitors), total))

	for i, mon := range m.monitors {
		selected := i == m.monitorIdx
		label := fmt.Sprintf("Monitor %d", i+1)
		if mon.Primary {
			label += " (Primary)"
		}

		res := fmt.Sprintf("%dx%d", mon.Width, mon.Height)
		windows := m.windowCounts[i]
		layout := layoutForCount(windows)

		if selected {
			s.WriteString(fmt.Sprintf("  %s %s  %s  %s windows  %s\n",
				TitleStyle.Render(">"),
				SubtitleStyle.Render(label),
				DimStyle.Render(res),
				TitleStyle.Render(fmt.Sprintf("%d", windows)),
				DimStyle.Render("("+layout+")")))
		} else {
			s.WriteString(fmt.Sprintf("    %s  %s  %d windows  %s\n",
				DimStyle.Render(label),
				DimStyle.Render(res),
				windows,
				DimStyle.Render("("+layout+")")))
		}

		if selected {
			s.WriteString(renderLayoutPreview(windows))
			s.WriteString("\n")
		}
	}

	s.WriteString("\n  " + DimStyle.Render("<-> select monitor  up/down adjust windows  Enter launch  Esc quit") + "\n")
	return s.String()
}
