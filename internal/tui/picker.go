package tui

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/bcmister/qs/internal/config"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// pickerStage tracks which stage of the picker we're in.
type pickerStage int

const (
	stageProject pickerStage = iota
	stageCreate
	stageAccount
)

var windowsReservedNames = map[string]struct{}{
	"CON": {}, "PRN": {}, "AUX": {}, "NUL": {},
	"COM1": {}, "COM2": {}, "COM3": {}, "COM4": {}, "COM5": {}, "COM6": {}, "COM7": {}, "COM8": {}, "COM9": {},
	"LPT1": {}, "LPT2": {}, "LPT3": {}, "LPT4": {}, "LPT5": {}, "LPT6": {}, "LPT7": {}, "LPT8": {}, "LPT9": {},
}

// PickerModel is the two-stage project->account picker.
type PickerModel struct {
	cfg      *config.Config
	keys     config.AccountKeys
	stage    pickerStage
	quitting bool
	err      error
	width    int
	height   int

	// Project stage
	projects           []string
	filtered           []string
	filter             string
	cursor             int // 0 is create-new-folder, 1..n are projects
	viewOffset         int
	statusMsg          string
	statusErr          bool
	preselectedProject string

	// Create folder stage
	createInput string
	createErr   string

	// Account stage
	selected   string
	accounts   []config.Account
	accountIdx int
}

// NewPicker creates a new picker model.
func NewPicker(cfg *config.Config) PickerModel {
	projects := scanProjects(cfg.ProjectsRoot)
	accounts := config.EnabledAccounts(cfg.Accounts)
	keys, _ := config.LoadKeys()

	cursor := 0
	if len(projects) > 0 {
		cursor = 1
	}

	accountIdx := 0
	preselect := cfg.LastAccount
	if preselect == "" {
		preselect = cfg.DefaultAccount
	}
	if preselect != "" {
		for i, a := range accounts {
			if a.ID == preselect {
				accountIdx = i
				break
			}
		}
	}

	return PickerModel{
		cfg:        cfg,
		keys:       keys,
		stage:      stageProject,
		projects:   projects,
		filtered:   projects,
		cursor:     cursor,
		accounts:   accounts,
		accountIdx: accountIdx,
	}
}

// NewPickerWithProject creates a picker that skips straight to account selection
// for the given project name.
func NewPickerWithProject(cfg *config.Config, project string) PickerModel {
	m := NewPicker(cfg)
	m.preselectedProject = project
	return m
}

// preselectedProjectMsg is sent when a project was pre-selected via --project flag.
type preselectedProjectMsg struct{}

func (m PickerModel) Init() tea.Cmd {
	if m.preselectedProject != "" {
		return func() tea.Msg { return preselectedProjectMsg{} }
	}
	return nil
}

func (m PickerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch m.stage {
		case stageProject:
			return m.updateProject(msg)
		case stageCreate:
			return m.updateCreate(msg)
		default:
			return m.updateAccount(msg)
		}
	case preselectedProjectMsg:
		m.selected = m.preselectedProject
		return m.startAccountSelection()
	case execDoneMsg:
		m.err = msg.err
		return m, tea.Quit
	}

	return m, nil
}

func (m PickerModel) updateProject(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	projectRows := m.maxVisible() - 1 // one row for create-new-folder action
	if projectRows < 1 {
		projectRows = 1
	}
	maxCursor := len(m.filtered)

	switch msg.Type {
	case tea.KeyEsc, tea.KeyCtrlC:
		m.quitting = true
		return m, tea.Quit
	case tea.KeyEnter:
		m.statusMsg = ""
		m.statusErr = false
		if m.cursor == 0 {
			m.stage = stageCreate
			m.createInput = ""
			m.createErr = ""
			return m, nil
		}
		if m.cursor > 0 && m.cursor <= len(m.filtered) {
			m.selected = m.filtered[m.cursor-1]
			return m.startAccountSelection()
		}
	case tea.KeyUp:
		if m.cursor > 0 {
			m.cursor--
			if m.cursor > 0 {
				projectIdx := m.cursor - 1
				if projectIdx < m.viewOffset {
					m.viewOffset = projectIdx
				}
			}
		}
	case tea.KeyDown:
		if m.cursor < maxCursor {
			m.cursor++
			if m.cursor > 0 {
				projectIdx := m.cursor - 1
				if projectIdx >= m.viewOffset+projectRows {
					m.viewOffset = projectIdx - projectRows + 1
				}
			}
		}
	case tea.KeyBackspace:
		if len(m.filter) > 0 {
			m.filter = m.filter[:len(m.filter)-1]
			m.applyFilter()
		}
	default:
		if msg.Type == tea.KeyRunes {
			for _, r := range msg.Runes {
				if (r > 32 && r <= 126) || unicode.IsLetter(r) || unicode.IsDigit(r) {
					m.filter += string(r)
				}
			}
			m.applyFilter()
		} else if msg.Type == tea.KeySpace {
			m.filter += " "
			m.applyFilter()
		}
	}

	return m, nil
}

func (m PickerModel) updateCreate(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.stage = stageProject
		m.createErr = ""
		return m, nil
	case tea.KeyCtrlC:
		m.quitting = true
		return m, tea.Quit
	case tea.KeyEnter:
		return m.createProjectAndContinue()
	case tea.KeyBackspace:
		if len(m.createInput) > 0 {
			m.createInput = m.createInput[:len(m.createInput)-1]
		}
		m.createErr = ""
	default:
		if msg.Type == tea.KeyRunes {
			for _, r := range msg.Runes {
				if r > 31 && r != 127 {
					m.createInput += string(r)
				}
			}
			m.createErr = ""
		} else if msg.Type == tea.KeySpace {
			m.createInput += " "
			m.createErr = ""
		}
	}

	return m, nil
}

func (m PickerModel) updateAccount(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.stage = stageProject
		return m, nil
	case tea.KeyCtrlC:
		m.quitting = true
		return m, tea.Quit
	case tea.KeyEnter:
		if len(m.accounts) > 0 {
			return m.launchAccount(m.accounts[m.accountIdx])
		}
	case tea.KeyUp:
		if m.accountIdx > 0 {
			m.accountIdx--
		}
	case tea.KeyDown:
		if m.accountIdx < len(m.accounts)-1 {
			m.accountIdx++
		}
	}

	return m, nil
}

func (m PickerModel) startAccountSelection() (tea.Model, tea.Cmd) {
	if len(m.accounts) == 0 {
		m.stage = stageProject
		m.statusMsg = "No enabled tools configured. Run qs setup or qs accounts."
		m.statusErr = true
		return m, nil
	}
	if len(m.accounts) == 1 {
		return m.launchAccount(m.accounts[0])
	}
	m.stage = stageAccount
	return m, nil
}

func (m PickerModel) launchAccount(account config.Account) (tea.Model, tea.Cmd) {
	m.cfg.LastAccount = account.ID
	_ = config.Save(m.cfg, "")

	projectDir := filepath.Join(m.cfg.ProjectsRoot, m.selected)
	c := exec.Command(account.Command, account.Args...)
	c.Dir = projectDir

	// Inject API keys as env vars
	accountKeys := config.KeysForAccount(m.keys, account.ID)
	if len(accountKeys) > 0 {
		c.Env = os.Environ()
		for k, v := range accountKeys {
			c.Env = append(c.Env, k+"="+v)
		}
	}

	return m, tea.ExecProcess(c, func(err error) tea.Msg {
		return execDoneMsg{err: err}
	})
}

func (m PickerModel) createProjectAndContinue() (tea.Model, tea.Cmd) {
	name, err := sanitizeProjectName(m.createInput)
	if err != nil {
		m.createErr = err.Error()
		return m, nil
	}

	projectPath := filepath.Join(m.cfg.ProjectsRoot, name)
	info, statErr := os.Stat(projectPath)
	if statErr == nil {
		m.stage = stageProject
		m.createErr = ""
		m.refreshProjects()
		if info.IsDir() {
			m.statusMsg = fmt.Sprintf("Folder \"%s\" already exists. Select it from the list.", name)
			m.statusErr = true
			m.filter = ""
			m.applyFilter()
			m.setCursorForProject(name)
			return m, nil
		}
		m.statusMsg = fmt.Sprintf("A file named \"%s\" already exists.", name)
		m.statusErr = true
		return m, nil
	}
	if !os.IsNotExist(statErr) {
		m.createErr = fmt.Sprintf("Failed to check folder: %v", statErr)
		return m, nil
	}

	if err := os.Mkdir(projectPath, 0755); err != nil {
		m.createErr = fmt.Sprintf("Failed to create folder: %v", err)
		return m, nil
	}

	m.refreshProjects()
	m.selected = name
	m.stage = stageProject
	m.createErr = ""
	m.statusMsg = ""
	m.statusErr = false
	return m.startAccountSelection()
}

func (m *PickerModel) applyFilter() {
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

	if len(m.filtered) > 0 {
		m.cursor = 1
	} else {
		m.cursor = 0
	}
	m.viewOffset = 0
}

func (m *PickerModel) refreshProjects() {
	m.projects = scanProjects(m.cfg.ProjectsRoot)
	m.applyFilter()
}

func (m *PickerModel) setCursorForProject(name string) {
	for i, project := range m.filtered {
		if project == name {
			m.cursor = i + 1
			projectRows := m.maxVisible() - 1
			if projectRows < 1 {
				projectRows = 1
			}
			if i < m.viewOffset {
				m.viewOffset = i
			} else if i >= m.viewOffset+projectRows {
				m.viewOffset = i - projectRows + 1
			}
			return
		}
	}
}

func (m PickerModel) maxVisible() int {
	if m.height > 0 {
		max := m.height - 8 // header + filter + separator + footer
		if max < 1 {
			return 1
		}
		return max
	}
	return 12
}

func (m PickerModel) View() string {
	if m.quitting {
		return ""
	}

	switch m.stage {
	case stageProject:
		return m.viewProject()
	case stageCreate:
		return m.viewCreate()
	default:
		return m.viewAccount()
	}
}

func (m PickerModel) viewProject() string {
	var s strings.Builder
	maxShow := m.maxVisible() - 1
	if maxShow < 1 {
		maxShow = 1
	}

	title := lipgloss.NewStyle().Foreground(ColorBrCyan)
	dim := lipgloss.NewStyle().Foreground(ColorDimGray)
	white := lipgloss.NewStyle().Foreground(ColorWhite)
	sel := lipgloss.NewStyle().Foreground(ColorBrCyan).Bold(true)

	s.WriteString("\n")
	s.WriteString(fmt.Sprintf(" %s %s\n", title.Render("qs"), dim.Render("- select project")))
	s.WriteString("\n")

	if m.filter == "" {
		s.WriteString(fmt.Sprintf("  %s %s\n", sel.Render(">"), dim.Render("type to filter...")))
	} else {
		s.WriteString(fmt.Sprintf("  %s %s\n", sel.Render(">"), white.Render(m.filter)))
	}

	s.WriteString(fmt.Sprintf("  %s\n", dim.Render("---------------------------------")))

	if m.cursor == 0 {
		s.WriteString(fmt.Sprintf("  %s %s\n", sel.Render(">"), white.Render("+ create new folder")))
	} else {
		s.WriteString(fmt.Sprintf("    %s\n", dim.Render("+ create new folder")))
	}

	if len(m.filtered) == 0 {
		s.WriteString(fmt.Sprintf("\n  %s\n", dim.Render("no matches")))
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
			if m.cursor > 0 && idx == m.cursor-1 {
				s.WriteString(fmt.Sprintf("  %s %s\n", sel.Render(">"), white.Render(name)))
			} else {
				s.WriteString(fmt.Sprintf("    %s\n", dim.Render(name)))
			}
		}
	}

	s.WriteString("\n")
	if m.statusMsg != "" {
		statusStyle := dim
		if m.statusErr {
			statusStyle = lipgloss.NewStyle().Foreground(ColorYellow)
		}
		s.WriteString(fmt.Sprintf("  %s\n", statusStyle.Render(m.statusMsg)))
		s.WriteString("\n")
	}

	if len(m.filtered) > maxShow {
		s.WriteString(fmt.Sprintf("  %s navigate  %s  %s quit\n",
			dim.Render("up/down"),
			dim.Render(fmt.Sprintf("(%d/%d)", m.cursor+1, len(m.filtered)+1)),
			dim.Render("esc")))
	} else {
		s.WriteString(fmt.Sprintf("  %s navigate  %s select  %s quit\n",
			dim.Render("up/down"),
			dim.Render("enter"),
			dim.Render("esc")))
	}

	return s.String()
}

func (m PickerModel) viewCreate() string {
	var s strings.Builder

	title := lipgloss.NewStyle().Foreground(ColorBrCyan)
	dim := lipgloss.NewStyle().Foreground(ColorDimGray)
	white := lipgloss.NewStyle().Foreground(ColorWhite)
	sel := lipgloss.NewStyle().Foreground(ColorBrCyan).Bold(true)

	s.WriteString("\n")
	s.WriteString(fmt.Sprintf(" %s %s\n", title.Render("qs"), dim.Render("- create folder")))
	s.WriteString("\n")
	s.WriteString(fmt.Sprintf("  %s\n", dim.Render("single folder name only (spaces allowed)")))
	s.WriteString("\n")

	if m.createInput == "" {
		s.WriteString(fmt.Sprintf("  %s %s\n", sel.Render(">"), dim.Render("new folder name...")))
	} else {
		s.WriteString(fmt.Sprintf("  %s %s\n", sel.Render(">"), white.Render(m.createInput)))
	}

	if m.createErr != "" {
		errStyle := lipgloss.NewStyle().Foreground(ColorRed)
		s.WriteString("\n")
		s.WriteString(fmt.Sprintf("  %s\n", errStyle.Render(m.createErr)))
	}

	s.WriteString("\n")
	s.WriteString(fmt.Sprintf("  %s save  %s back\n", dim.Render("enter"), dim.Render("esc")))
	return s.String()
}

func (m PickerModel) viewAccount() string {
	var s strings.Builder

	title := lipgloss.NewStyle().Foreground(ColorBrCyan)
	dim := lipgloss.NewStyle().Foreground(ColorDimGray)
	white := lipgloss.NewStyle().Foreground(ColorWhite)
	sel := lipgloss.NewStyle().Foreground(ColorBrCyan).Bold(true)

	s.WriteString("\n")
	s.WriteString(fmt.Sprintf(" %s %s\n", title.Render("qs"), dim.Render("- "+m.selected)))
	s.WriteString("\n")
	s.WriteString(fmt.Sprintf("  %s\n", dim.Render("---------------------------------")))

	green := lipgloss.NewStyle().Foreground(ColorGreen)

	for i, a := range m.accounts {
		authBadge := ""
		if ak := config.KeysForAccount(m.keys, a.ID); len(ak) > 0 {
			authBadge = green.Render("(API) ")
		} else if a.HasAuth() {
			authBadge = dim.Render("(sub) ")
		}

		if i == m.accountIdx {
			s.WriteString(fmt.Sprintf("  %s %s %s%s  %s\n",
				sel.Render(">"),
				a.Icon,
				authBadge,
				white.Render(a.Label),
				dim.Render(a.FullCommand())))
		} else {
			s.WriteString(fmt.Sprintf("    %s %s%s  %s\n",
				a.Icon,
				authBadge,
				dim.Render(a.Label),
				dim.Render(a.FullCommand())))
		}
	}

	s.WriteString("\n")
	s.WriteString(fmt.Sprintf("  %s navigate  %s select  %s back\n",
		dim.Render("up/down"),
		dim.Render("enter"),
		dim.Render("esc")))

	return s.String()
}

// execDoneMsg is sent when the launched process finishes.
type execDoneMsg struct {
	err error
}

// scanProjects reads subdirectories from the projects root.
func scanProjects(root string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}

	var projects []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			projects = append(projects, e.Name())
		}
	}
	sort.Strings(projects)
	return projects
}

func sanitizeProjectName(input string) (string, error) {
	name := strings.TrimSpace(input)
	if name == "" {
		return "", fmt.Errorf("folder name is required")
	}
	if strings.ContainsRune(name, '/') || strings.ContainsRune(name, '\\') {
		return "", fmt.Errorf("only a single folder name is allowed")
	}
	if name == "." || name == ".." {
		return "", fmt.Errorf("invalid folder name")
	}
	if strings.ContainsAny(name, "<>:\"|?*") {
		return "", fmt.Errorf("folder name contains invalid characters")
	}
	if strings.HasSuffix(name, ".") {
		return "", fmt.Errorf("folder name cannot end with a dot")
	}
	for _, r := range name {
		if r < 32 {
			return "", fmt.Errorf("folder name contains control characters")
		}
	}

	base := strings.ToUpper(name)
	if dot := strings.IndexRune(base, '.'); dot >= 0 {
		base = base[:dot]
	}
	if _, reserved := windowsReservedNames[base]; reserved {
		return "", fmt.Errorf("folder name is reserved")
	}

	return name, nil
}
