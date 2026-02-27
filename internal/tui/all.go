package tui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/bcmister/qs/internal/config"
	"github.com/bcmister/qs/internal/monitor"
	tea "github.com/charmbracelet/bubbletea"
)

type allStep int

const (
	allStepMonitors allStep = iota
	allStepProject
)

// AllModel is the TUI for `qs all` — configure monitors then pick a project.
type AllModel struct {
	cfg      *config.Config
	monitors []monitor.Monitor
	step     allStep
	width    int
	height   int

	// Monitor step
	monitorIdx   int
	windowCounts []int

	// Project step
	projects   []string
	filtered   []string
	filter     string
	cursor     int
	viewOffset int

	// Results
	selectedProject string
	quitting        bool
}

// NewAll creates the AllModel TUI.
func NewAll(cfg *config.Config) AllModel {
	return AllModel{
		cfg:  cfg,
		step: allStepMonitors,
	}
}

// Cancelled returns true if the user quit without completing.
func (m AllModel) Cancelled() bool { return m.quitting }

// SelectedProject returns the project chosen by the user.
func (m AllModel) SelectedProject() string { return m.selectedProject }

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
			m.windowCounts[i] = 1
			if i < len(m.cfg.Monitors) {
				count := m.cfg.Monitors[i].WindowCount()
				if count >= 1 {
					m.windowCounts[i] = count
				}
			}
		}
		return m, nil

	case tea.KeyMsg:
		switch m.step {
		case allStepMonitors:
			return m.updateMonitors(msg)
		case allStepProject:
			return m.updateProject(msg)
		}
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
		// Transition to project selection
		m.step = allStepProject
		m.projects = scanProjects(m.cfg.ProjectsRoot)
		m.filtered = m.projects
		if len(m.projects) > 0 {
			m.cursor = 0
		}
		return m, nil
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

func (m AllModel) updateProject(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	maxShow := m.allMaxVisible()
	if maxShow < 1 {
		maxShow = 1
	}
	maxCursor := len(m.filtered) - 1

	switch msg.Type {
	case tea.KeyEsc:
		m.step = allStepMonitors
		m.filter = ""
		return m, nil
	case tea.KeyCtrlC:
		m.quitting = true
		return m, tea.Quit
	case tea.KeyEnter:
		if m.cursor >= 0 && m.cursor < len(m.filtered) {
			m.selectedProject = m.filtered[m.cursor]
			return m, tea.Quit
		}
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
			if m.cursor < m.viewOffset {
				m.viewOffset = m.cursor
			}
		}
	case tea.KeyDown:
		if m.cursor < maxCursor {
			m.cursor++
			if m.cursor >= m.viewOffset+maxShow {
				m.viewOffset = m.cursor - maxShow + 1
			}
		}
	case tea.KeyBackspace:
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
			m.allApplyFilter()
		}
	default:
		if msg.Type == tea.KeyRunes {
			for _, r := range msg.Runes {
				if (r > 32 && r <= 126) || unicode.IsLetter(r) || unicode.IsDigit(r) {
					m.filter += string(r)
				}
			}
			m.allApplyFilter()
		} else if msg.Type == tea.KeySpace {
			m.filter += " "
			m.allApplyFilter()
		}
	}

	return m, nil
}

func (m *AllModel) allApplyFilter() {
	if m.filter == "" {
		m.filtered = m.projects
	} else {
		q := strings.ToLower(m.filter)
		var result []string
		for _, p := range m.projects {
			if strings.Contains(strings.ToLower(p), q) {
				result = append(result, p)
			}
		}
		m.filtered = result
	}

	m.cursor = 0
	m.viewOffset = 0
}

func (m AllModel) allMaxVisible() int {
	if m.height > 0 {
		max := m.height - 8
		if max < 1 {
			return 1
		}
		return max
	}
	return 12
}

func (m AllModel) View() string {
	if m.quitting {
		return ""
	}

	switch m.step {
	case allStepMonitors:
		return m.viewMonitors()
	case allStepProject:
		return m.viewProject()
	}
	return ""
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

	s.WriteString("\n  " + DimStyle.Render("<-> select monitor  up/down adjust windows  Enter continue  Esc quit") + "\n")
	return s.String()
}

func (m AllModel) viewProject() string {
	var s strings.Builder
	maxShow := m.allMaxVisible()
	if maxShow < 1 {
		maxShow = 1
	}

	s.WriteString(RenderLogo("all"))
	s.WriteString(RenderSep())
	s.WriteString("\n")
	s.WriteString("  " + SubtitleStyle.Render("Select project for all windows") + "\n\n")

	if m.filter == "" {
		s.WriteString(fmt.Sprintf("  %s %s\n", SelectedStyle.Render(">"), DimStyle.Render("type to filter...")))
	} else {
		s.WriteString(fmt.Sprintf("  %s %s\n", SelectedStyle.Render(">"), WhiteStyle.Render(m.filter)))
	}

	s.WriteString(fmt.Sprintf("  %s\n", DimStyle.Render("---------------------------------")))

	if len(m.filtered) == 0 {
		s.WriteString(fmt.Sprintf("\n  %s\n", DimStyle.Render("no projects found")))
	} else {
		maxOff := len(m.filtered) - maxShow
		if maxOff < 0 {
			maxOff = 0
		}
		viewOffset := m.viewOffset
		if viewOffset > maxOff {
			viewOffset = maxOff
		}

		for i := 0; i < maxShow && viewOffset+i < len(m.filtered); i++ {
			idx := viewOffset + i
			name := m.filtered[idx]
			if idx == m.cursor {
				s.WriteString(fmt.Sprintf("  %s %s\n", SelectedStyle.Render(">"), WhiteStyle.Render(name)))
			} else {
				s.WriteString(fmt.Sprintf("    %s\n", DimStyle.Render(name)))
			}
		}
	}

	s.WriteString("\n")
	s.WriteString(fmt.Sprintf("  %s navigate  %s select  %s back\n",
		DimStyle.Render("up/down"),
		DimStyle.Render("enter"),
		DimStyle.Render("esc")))

	return s.String()
}
