package tui

import (
	"fmt"
	"strings"

	"github.com/bcmister/qs/internal/config"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// FirstRunAction represents the action selected from the first-run flow.
type FirstRunAction int

const (
	FirstRunQuit FirstRunAction = iota
	FirstRunLaunchSetup
	FirstRunSetPathNow
)

type firstRunStep int

const (
	firstRunChoice firstRunStep = iota
	firstRunPathInput
)

// FirstRunResult stores the outcome of the first-run flow.
type FirstRunResult struct {
	Action       FirstRunAction
	ProjectsRoot string
}

// FirstRunModel is the first-run choice + quick path input flow.
type FirstRunModel struct {
	step      firstRunStep
	choice    int
	rootInput textinput.Model
	errText   string
	result    FirstRunResult
}

// NewFirstRun creates a first-run flow model.
func NewFirstRun(existing *config.Config) FirstRunModel {
	defaultRoot := config.DefaultProjectsRoot()
	if existing != nil && strings.TrimSpace(existing.ProjectsRoot) != "" {
		defaultRoot = strings.TrimSpace(existing.ProjectsRoot)
	}

	input := textinput.New()
	input.Placeholder = config.DefaultProjectsRoot()
	input.Width = 56
	input.CharLimit = 512
	input.SetValue(defaultRoot)

	return FirstRunModel{
		step:      firstRunChoice,
		choice:    0,
		rootInput: input,
	}
}

func (m FirstRunModel) Init() tea.Cmd {
	return nil
}

func (m FirstRunModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch m.step {
		case firstRunChoice:
			return m.updateChoice(msg)
		case firstRunPathInput:
			return m.updatePathInput(msg)
		}
	}
	return m, nil
}

func (m FirstRunModel) updateChoice(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC, tea.KeyEsc:
		m.result.Action = FirstRunQuit
		return m, tea.Quit
	case tea.KeyUp:
		if m.choice > 0 {
			m.choice--
		}
	case tea.KeyDown:
		if m.choice < 1 {
			m.choice++
		}
	case tea.KeyEnter:
		if m.choice == 0 {
			m.result.Action = FirstRunLaunchSetup
			return m, tea.Quit
		}
		m.step = firstRunPathInput
		m.rootInput.Focus()
		return m, textinput.Blink
	}
	return m, nil
}

func (m FirstRunModel) updatePathInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		m.result.Action = FirstRunQuit
		return m, tea.Quit
	case tea.KeyEsc:
		m.step = firstRunChoice
		m.errText = ""
		m.rootInput.Blur()
		return m, nil
	case tea.KeyEnter:
		root := strings.TrimSpace(m.rootInput.Value())
		if root == "" {
			m.errText = "Project path is required."
			return m, nil
		}
		m.result.Action = FirstRunSetPathNow
		m.result.ProjectsRoot = root
		return m, tea.Quit
	default:
		var cmd tea.Cmd
		m.rootInput, cmd = m.rootInput.Update(msg)
		m.errText = ""
		return m, cmd
	}
	return m, nil
}

func (m FirstRunModel) View() string {
	var s strings.Builder
	s.WriteString(RenderLogo("first run"))
	s.WriteString(RenderSep())
	s.WriteString("\n")

	if m.step == firstRunChoice {
		s.WriteString("  " + SubtitleStyle.Render("No default project path is configured.") + "\n\n")
		s.WriteString("  " + DimStyle.Render("Choose how you want to continue:") + "\n\n")
		options := []string{"Run full setup", "Set project path now"}
		for i, option := range options {
			prefix := "   "
			style := DimStyle
			if i == m.choice {
				prefix = " > "
				style = WhiteStyle
			}
			s.WriteString(fmt.Sprintf(" %s%s\n", prefix, style.Render(option)))
		}
		s.WriteString("\n  " + DimStyle.Render("up/down navigate  enter select  esc quit") + "\n")
		return s.String()
	}

	s.WriteString("  " + TitleStyle.Render("Set project path") + "\n\n")
	s.WriteString("  " + DimStyle.Render("This is the folder qs will open by default.") + "\n\n")
	s.WriteString("  " + m.rootInput.View() + "\n")
	if m.errText != "" {
		s.WriteString("\n  " + ErrorStyle.Render(m.errText) + "\n")
	}
	s.WriteString("\n  " + DimStyle.Render("enter save  esc back") + "\n")
	return s.String()
}

// Result returns the selected first-run action and payload.
func (m FirstRunModel) Result() FirstRunResult {
	return m.result
}
