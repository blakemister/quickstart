package tui

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/bcmister/qs/internal/config"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// authDoneMsg is sent when an auth process completes
type authDoneMsg struct {
	err error
}

// installDoneMsg is sent when an install process completes
type installDoneMsg struct {
	err error
}

// AccountsModel is the account management TUI
type AccountsModel struct {
	cfg        *config.Config
	accounts   []config.Account
	cursor     int
	width      int
	height     int
	editing    bool
	deleting   bool
	adding     bool
	inputs     []textinput.Model
	inputIdx   int
	message    string

	// API keys management
	keys       config.AccountKeys
	editKeys   bool
	keysList   []keyEntry
	keysIdx    int
	addingKey  bool
	keyInputs  []textinput.Model
	keyInputIdx int
}

// keyEntry holds a key name and masked value for display
type keyEntry struct {
	name  string
	value string
}

// NewAccounts creates a new account management model
func NewAccounts(cfg *config.Config) AccountsModel {
	// Copy accounts so we can edit them
	accounts := make([]config.Account, len(cfg.Accounts))
	for i, a := range cfg.Accounts {
		accounts[i] = config.Account{
			ID:         a.ID,
			Label:      a.Label,
			Command:    a.Command,
			Args:       append([]string{}, a.Args...),
			AuthCmd:    a.AuthCmd,
			InstallCmd: a.InstallCmd,
			Icon:       a.Icon,
			Enabled:    a.Enabled,
		}
	}

	keys, _ := config.LoadKeys()

	return AccountsModel{
		cfg:      cfg,
		accounts: accounts,
		keys:     keys,
	}
}

func (m AccountsModel) Init() tea.Cmd {
	return nil
}

func (m AccountsModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case authDoneMsg:
		if msg.err != nil {
			m.message = "Auth failed: " + msg.err.Error()
		} else {
			m.message = "Auth completed successfully"
		}
		return m, nil

	case installDoneMsg:
		if msg.err != nil {
			m.message = "Install failed: " + msg.err.Error()
		} else {
			m.message = "Install completed successfully"
		}
		return m, nil

	case tea.KeyMsg:
		// Handle sub-forms
		if m.editing || m.adding {
			return m.updateForm(msg)
		}
		if m.deleting {
			return m.updateDelete(msg)
		}
		if m.editKeys {
			if m.addingKey {
				return m.updateAddKey(msg)
			}
			return m.updateKeys(msg)
		}

		switch {
		case key.Matches(msg, DefaultKeyMap.Escape):
			// Save and quit
			m.cfg.Accounts = m.accounts
			_ = config.Save(m.cfg, "")
			_ = config.SaveKeys(m.keys)
			return m, tea.Quit

		// Note: "k" must be checked before Up since "k" is also bound to Up navigation
		case msg.String() == "k":
			m.editKeys = true
			m.keysIdx = 0
			m.addingKey = false
			m.refreshKeysList()
			m.message = ""

		case msg.String() == "l":
			a := m.accounts[m.cursor]
			if !a.HasAuth() {
				m.message = a.Label + " uses env vars for auth (no login command)"
				return m, nil
			}
			cmd, args := a.AuthCommand()
			c := exec.Command(cmd, args...)
			return m, tea.ExecProcess(c, func(err error) tea.Msg {
				return authDoneMsg{err: err}
			})

		case msg.String() == "i":
			a := m.accounts[m.cursor]
			if !a.HasInstall() {
				m.message = a.Label + " has no install command configured"
				return m, nil
			}
			cmd, args := a.InstallCommand()
			c := exec.Command(cmd, args...)
			return m, tea.ExecProcess(c, func(err error) tea.Msg {
				return installDoneMsg{err: err}
			})

		case msg.String() == "a":
			m.adding = true
			m.inputs = makeAccountFormInputs("", "", "", "", "", "")
			m.inputIdx = 0
			m.inputs[0].Focus()
			m.message = ""
			return m, textinput.Blink

		case msg.String() == "e":
			a := m.accounts[m.cursor]
			m.editing = true
			m.inputs = makeAccountFormInputs(a.Label, a.Command, strings.Join(a.Args, " "), a.AuthCmd, a.InstallCmd, a.Icon)
			m.inputIdx = 0
			m.inputs[0].Focus()
			m.message = ""
			return m, textinput.Blink

		case key.Matches(msg, DefaultKeyMap.Up):
			if m.cursor > 0 {
				m.cursor--
			}

		case key.Matches(msg, DefaultKeyMap.Down):
			if m.cursor < len(m.accounts)-1 {
				m.cursor++
			}

		case key.Matches(msg, DefaultKeyMap.Space):
			// Toggle enabled — but don't allow disabling last enabled account
			if m.accounts[m.cursor].Enabled {
				enabledCount := len(config.EnabledAccounts(m.accounts))
				if enabledCount <= 1 {
					m.message = "Cannot disable last enabled account"
					return m, nil
				}
			}
			m.accounts[m.cursor].Enabled = !m.accounts[m.cursor].Enabled
			m.message = ""

		case key.Matches(msg, DefaultKeyMap.Delete):
			if len(m.accounts) <= 1 {
				m.message = "Cannot delete last account"
				return m, nil
			}
			if m.accounts[m.cursor].Enabled {
				enabledCount := len(config.EnabledAccounts(m.accounts))
				if enabledCount <= 1 {
					m.message = "Cannot delete last enabled account"
					return m, nil
				}
			}
			m.deleting = true
			m.message = ""
		}
	}
	return m, nil
}

func (m AccountsModel) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, DefaultKeyMap.Escape):
		m.editing = false
		m.adding = false
		return m, nil

	case key.Matches(msg, DefaultKeyMap.Tab):
		m.inputs[m.inputIdx].Blur()
		m.inputIdx = (m.inputIdx + 1) % len(m.inputs)
		m.inputs[m.inputIdx].Focus()
		return m, textinput.Blink

	case key.Matches(msg, DefaultKeyMap.Enter):
		if m.inputIdx < len(m.inputs)-1 {
			m.inputs[m.inputIdx].Blur()
			m.inputIdx++
			m.inputs[m.inputIdx].Focus()
			return m, textinput.Blink
		}

		// Submit
		name := m.inputs[0].Value()
		command := m.inputs[1].Value()
		args := m.inputs[2].Value()
		authCmd := m.inputs[3].Value()
		installCmd := m.inputs[4].Value()
		icon := m.inputs[5].Value()

		if name == "" || command == "" {
			m.message = "Name and command are required"
			return m, nil
		}

		if icon == "" {
			icon = "⬜"
		}

		var argList []string
		if args != "" {
			argList = strings.Fields(args)
		}

		if m.editing {
			m.accounts[m.cursor].Label = name
			m.accounts[m.cursor].Command = command
			m.accounts[m.cursor].Args = argList
			m.accounts[m.cursor].AuthCmd = authCmd
			m.accounts[m.cursor].InstallCmd = installCmd
			m.accounts[m.cursor].Icon = icon
			m.editing = false
		} else {
			id := strings.ToLower(strings.ReplaceAll(name, " ", "-"))
			m.accounts = append(m.accounts, config.Account{
				ID:         id,
				Label:      name,
				Command:    command,
				Args:       argList,
				AuthCmd:    authCmd,
				InstallCmd: installCmd,
				Icon:       icon,
				Enabled:    true,
			})
			m.cursor = len(m.accounts) - 1
			m.adding = false
		}
		m.message = ""
		return m, nil

	default:
		var cmd tea.Cmd
		m.inputs[m.inputIdx], cmd = m.inputs[m.inputIdx].Update(msg)
		return m, cmd
	}
}

func (m AccountsModel) updateDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		m.accounts = append(m.accounts[:m.cursor], m.accounts[m.cursor+1:]...)
		if m.cursor >= len(m.accounts) {
			m.cursor = len(m.accounts) - 1
		}
		m.deleting = false
		m.message = ""
	case "n", "N", "esc":
		m.deleting = false
		m.message = ""
	}
	return m, nil
}

func (m AccountsModel) View() string {
	var s strings.Builder

	s.WriteString(RenderLogo("accounts"))
	s.WriteString(RenderSep())
	s.WriteString("\n")

	if m.editKeys {
		a := m.accounts[m.cursor]
		if m.addingKey {
			s.WriteString("  " + TitleStyle.Render("Add API Key") + " " + DimStyle.Render("for "+a.ID) + "\n\n")
			labels := []string{"Env Var", "Value"}
			for i, input := range m.keyInputs {
				active := i == m.keyInputIdx
				label := DimStyle.Render(fmt.Sprintf("  %-8s", labels[i]))
				if active {
					label = TitleStyle.Render(fmt.Sprintf("  %-8s", labels[i]))
				}
				s.WriteString(fmt.Sprintf("%s  %s\n", label, input.View()))
			}
			if m.message != "" {
				s.WriteString("\n  " + ErrorStyle.Render(m.message) + "\n")
			}
			s.WriteString("\n  " + DimStyle.Render("Tab next  Enter submit  Esc cancel") + "\n")
			return s.String()
		}

		s.WriteString("  " + TitleStyle.Render("API Keys") + " " + DimStyle.Render("for "+a.ID) + "\n\n")
		if len(m.keysList) == 0 {
			s.WriteString("  " + DimStyle.Render("No keys configured") + "\n")
		} else {
			for i, entry := range m.keysList {
				prefix := "  "
				if i == m.keysIdx {
					prefix = TitleStyle.Render("▸ ")
				}
				nameStr := WhiteStyle.Render(entry.name)
				if i != m.keysIdx {
					nameStr = DimStyle.Render(entry.name)
				}
				s.WriteString(fmt.Sprintf("  %s %s  %s\n",
					prefix,
					nameStr,
					DimStyle.Render(config.MaskValue(entry.value))))
			}
		}
		s.WriteString("\n  " + DimStyle.Render("a add  d delete  Esc back") + "\n")
		return s.String()
	}

	if m.editing || m.adding {
		title := "Edit Account"
		if m.adding {
			title = "Add Account"
		}
		s.WriteString("  " + TitleStyle.Render(title) + "\n\n")

		labels := []string{"Name", "Command", "Args", "Auth Cmd", "Install", "Icon"}
		for i, input := range m.inputs {
			active := i == m.inputIdx
			label := DimStyle.Render(fmt.Sprintf("  %-8s", labels[i]))
			if active {
				label = TitleStyle.Render(fmt.Sprintf("  %-8s", labels[i]))
			}
			s.WriteString(fmt.Sprintf("%s  %s\n", label, input.View()))
		}

		if m.message != "" {
			s.WriteString("\n  " + ErrorStyle.Render(m.message) + "\n")
		}

		s.WriteString("\n  " + DimStyle.Render("Tab next  Enter submit  Esc cancel") + "\n")
		return s.String()
	}

	if m.deleting {
		a := m.accounts[m.cursor]
		s.WriteString(fmt.Sprintf("  %s Delete %s %s? %s\n\n",
			WarningStyle.Render("⚠"),
			a.Icon, SubtitleStyle.Render(a.Label),
			DimStyle.Render("(y/n)")))
		return s.String()
	}

	// Main account list
	for i, a := range m.accounts {
		selected := i == m.cursor

		enabledMark := DimStyle.Render("[ ]")
		if a.Enabled {
			enabledMark = SuccessStyle.Render("[✓]")
		}

		pathMark := ErrorStyle.Render("✗")
		if _, err := exec.LookPath(a.Command); err == nil {
			pathMark = SuccessStyle.Render("✓")
		}

		prefix := "  "
		if selected {
			prefix = TitleStyle.Render("▸ ")
		}

		label := a.Icon + " " + a.Label
		if selected {
			label = SubtitleStyle.Render(a.Icon + " " + a.Label)
		} else if !a.Enabled {
			label = DimStyle.Render(a.Icon + " " + a.Label)
		} else {
			label = WhiteStyle.Render(a.Icon + " " + a.Label)
		}

		cmdStr := DimStyle.Render(truncate(a.FullCommand(), 40))

		keysBadge := ""
		if ak := config.KeysForAccount(m.keys, a.ID); len(ak) > 0 {
			keysBadge = DimStyle.Render(fmt.Sprintf(" [%d keys]", len(ak)))
		}

		s.WriteString(fmt.Sprintf("  %s %s %s  %s  %s%s\n",
			prefix, enabledMark, label, pathMark, cmdStr, keysBadge))
	}

	if m.message != "" {
		s.WriteString("\n  " + WarningStyle.Render(m.message) + "\n")
	}

	s.WriteString("\n  " + DimStyle.Render("Space toggle  a add  e edit  i install  l login  k keys  d delete  Esc save & quit") + "\n")
	return s.String()
}

func (m *AccountsModel) refreshKeysList() {
	accountID := m.accounts[m.cursor].ID
	ak := config.KeysForAccount(m.keys, accountID)
	m.keysList = nil
	for name, val := range ak {
		m.keysList = append(m.keysList, keyEntry{name: name, value: val})
	}
	// Sort for stable display
	sortKeyEntries(m.keysList)
	if m.keysIdx >= len(m.keysList) {
		m.keysIdx = 0
	}
}

func sortKeyEntries(entries []keyEntry) {
	for i := 1; i < len(entries); i++ {
		for j := i; j > 0 && entries[j].name < entries[j-1].name; j-- {
			entries[j], entries[j-1] = entries[j-1], entries[j]
		}
	}
}

func (m AccountsModel) updateKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, DefaultKeyMap.Escape):
		m.editKeys = false
		return m, nil
	case key.Matches(msg, DefaultKeyMap.Up):
		if m.keysIdx > 0 {
			m.keysIdx--
		}
	case key.Matches(msg, DefaultKeyMap.Down):
		if m.keysIdx < len(m.keysList)-1 {
			m.keysIdx++
		}
	case msg.String() == "a":
		m.addingKey = true
		m.keyInputs = makeKeyInputs()
		m.keyInputIdx = 0
		m.keyInputs[0].Focus()
		return m, textinput.Blink
	case key.Matches(msg, DefaultKeyMap.Delete):
		if len(m.keysList) > 0 {
			entry := m.keysList[m.keysIdx]
			accountID := m.accounts[m.cursor].ID
			config.DeleteAccountKey(m.keys, accountID, entry.name)
			m.refreshKeysList()
		}
	}
	return m, nil
}

func (m AccountsModel) updateAddKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, DefaultKeyMap.Escape):
		m.addingKey = false
		return m, nil
	case key.Matches(msg, DefaultKeyMap.Tab):
		m.keyInputs[m.keyInputIdx].Blur()
		m.keyInputIdx = (m.keyInputIdx + 1) % len(m.keyInputs)
		m.keyInputs[m.keyInputIdx].Focus()
		return m, textinput.Blink
	case key.Matches(msg, DefaultKeyMap.Enter):
		if m.keyInputIdx < len(m.keyInputs)-1 {
			m.keyInputs[m.keyInputIdx].Blur()
			m.keyInputIdx++
			m.keyInputs[m.keyInputIdx].Focus()
			return m, textinput.Blink
		}
		// Submit
		name := m.keyInputs[0].Value()
		value := m.keyInputs[1].Value()
		if err := config.ValidateEnvVarName(name); err != nil {
			m.message = err.Error()
			return m, nil
		}
		if value == "" {
			m.message = "Value cannot be empty"
			return m, nil
		}
		accountID := m.accounts[m.cursor].ID
		config.SetAccountKey(m.keys, accountID, name, value)
		m.addingKey = false
		m.message = ""
		m.refreshKeysList()
		return m, nil
	default:
		var cmd tea.Cmd
		m.keyInputs[m.keyInputIdx], cmd = m.keyInputs[m.keyInputIdx].Update(msg)
		return m, cmd
	}
}

func makeKeyInputs() []textinput.Model {
	inputs := make([]textinput.Model, 2)

	inputs[0] = textinput.New()
	inputs[0].Placeholder = "API_KEY_HERE"
	inputs[0].CharLimit = 64
	inputs[0].Width = 40

	inputs[1] = textinput.New()
	inputs[1].Placeholder = "sk-..."
	inputs[1].CharLimit = 256
	inputs[1].Width = 40
	inputs[1].EchoMode = textinput.EchoPassword

	return inputs
}

func makeAccountFormInputs(name, command, args, authCmd, installCmd, icon string) []textinput.Model {
	inputs := make([]textinput.Model, 6)

	inputs[0] = textinput.New()
	inputs[0].Placeholder = "Tool Name"
	inputs[0].CharLimit = 32
	inputs[0].Width = 30
	inputs[0].SetValue(name)

	inputs[1] = textinput.New()
	inputs[1].Placeholder = "command-name"
	inputs[1].CharLimit = 64
	inputs[1].Width = 30
	inputs[1].SetValue(command)

	inputs[2] = textinput.New()
	inputs[2].Placeholder = "--flag1 --flag2"
	inputs[2].CharLimit = 128
	inputs[2].Width = 30
	inputs[2].SetValue(args)

	inputs[3] = textinput.New()
	inputs[3].Placeholder = "command login"
	inputs[3].CharLimit = 128
	inputs[3].Width = 30
	inputs[3].SetValue(authCmd)

	inputs[4] = textinput.New()
	inputs[4].Placeholder = "npm i -g package-name"
	inputs[4].CharLimit = 128
	inputs[4].Width = 30
	inputs[4].SetValue(installCmd)

	inputs[5] = textinput.New()
	inputs[5].Placeholder = "⬜"
	inputs[5].CharLimit = 4
	inputs[5].Width = 10
	inputs[5].SetValue(icon)

	return inputs
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
