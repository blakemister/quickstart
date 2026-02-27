package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/bcmister/qs/internal/config"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// authDoneMsg is sent when an auth process completes
type authDoneMsg struct {
	err       error
	accountID string // which account triggered auth
}

// installDoneMsg is sent when an install process completes
type installDoneMsg struct {
	err error
}

// authProbeMsg is sent when auth status probing completes
type authProbeMsg struct {
	accountID string
	email     string
	org       string
	err       error
}

// addStep tracks the guided account-add wizard steps.
type addStep int

const (
	addStepNone   addStep = iota
	addStepMethod         // pick: Sub login / API key / Custom
	addStepTool           // pick which tool template
	addStepLabel          // enter account name
	addStepKey            // API key path: enter key value
)

// addMethod represents the three add-account paths.
type addMethod int

const (
	addMethodSub    addMethod = iota // subscription login
	addMethodAPI                     // API key
	addMethodCustom                  // full manual form
)

// AccountsModel is the account management TUI
type AccountsModel struct {
	cfg      *config.Config
	accounts []config.Account
	cursor   int
	width    int
	height   int
	editing  bool
	deleting bool
	inputs   []textinput.Model
	inputIdx int
	message  string

	// Guided add wizard
	addStep      addStep
	addMethod    addMethod
	addMethodIdx int            // cursor for method picker
	addTools     []config.Account // filtered tool templates for the chosen method
	addToolIdx   int            // cursor for tool picker
	addLabel     textinput.Model
	addKeyName   textinput.Model // env var name for API key path
	addKeyValue  textinput.Model // env var value for API key path
	addKeyIdx    int            // 0=name, 1=value

	// API keys management
	keys        config.AccountKeys
	editKeys    bool
	keysList    []keyEntry
	keysIdx     int
	addingKey   bool
	keyInputs   []textinput.Model
	keyInputIdx int
}

// keyEntry holds a key name and masked value for display
type keyEntry struct {
	name  string
	value string
}

// NewAccounts creates a new account management model
func NewAccounts(cfg *config.Config) AccountsModel {
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
			AuthUser:   a.AuthUser,
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
			m.message = "Auth completed — probing status..."
			// Fire auth probe for the account that just authenticated
			accountID := msg.accountID
			a := config.AccountByID(m.accounts, accountID)
			if a != nil {
				env := accountEnvSlice(m.keys, accountID)
				return m, probeAuthCmd(accountID, a.Command, env)
			}
			m.message = "Auth completed successfully"
		}
		return m, nil

	case authProbeMsg:
		if msg.err != nil {
			m.message = "Auth OK (could not probe status: " + msg.err.Error() + ")"
		} else if msg.email != "" {
			authUser := msg.email
			if msg.org != "" {
				authUser += " (" + msg.org + ")"
			}
			// Update the account's AuthUser
			for i := range m.accounts {
				if m.accounts[i].ID == msg.accountID {
					m.accounts[i].AuthUser = authUser
					break
				}
			}
			m.message = "Logged in as " + authUser
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
		// Handle guided add wizard
		if m.addStep != addStepNone {
			return m.updateAddWizard(msg)
		}
		// Handle edit form
		if m.editing {
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
			applyAccountEnv(c, m.keys, a.ID)
			accountID := a.ID
			return m, tea.ExecProcess(c, func(err error) tea.Msg {
				return authDoneMsg{err: err, accountID: accountID}
			})

		case msg.String() == "i":
			a := m.accounts[m.cursor]
			if !a.HasInstall() {
				m.message = a.Label + " has no install command configured"
				return m, nil
			}
			cmd, args := a.InstallCommand()
			c := exec.Command(cmd, args...)
			applyAccountEnv(c, m.keys, a.ID)
			return m, tea.ExecProcess(c, func(err error) tea.Msg {
				return installDoneMsg{err: err}
			})

		case msg.String() == "a":
			m.addStep = addStepMethod
			m.addMethodIdx = 0
			m.message = ""
			return m, nil

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

// updateAddWizard handles the multi-step guided add flow.
func (m AccountsModel) updateAddWizard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.addStep {
	case addStepMethod:
		return m.updateAddMethod(msg)
	case addStepTool:
		return m.updateAddTool(msg)
	case addStepLabel:
		return m.updateAddLabel(msg)
	case addStepKey:
		return m.updateAddKeyInput(msg)
	}
	return m, nil
}

func (m AccountsModel) updateAddMethod(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, DefaultKeyMap.Escape):
		m.addStep = addStepNone
		return m, nil
	case key.Matches(msg, DefaultKeyMap.Up):
		if m.addMethodIdx > 0 {
			m.addMethodIdx--
		}
	case key.Matches(msg, DefaultKeyMap.Down):
		if m.addMethodIdx < 2 {
			m.addMethodIdx++
		}
	case key.Matches(msg, DefaultKeyMap.Enter):
		switch m.addMethodIdx {
		case 0:
			m.addMethod = addMethodSub
			m.addTools = toolsWithAuth()
		case 1:
			m.addMethod = addMethodAPI
			m.addTools = toolsWithEnvVars()
		case 2:
			// Custom — go straight to full 6-field form
			m.addMethod = addMethodCustom
			m.addStep = addStepNone
			m.editing = false
			m.inputs = makeAccountFormInputs("", "", "", "", "", "")
			m.inputIdx = 0
			m.inputs[0].Focus()
			// Use a temporary flag — we reuse the edit form with adding semantics
			// We'll track this via a nil check: if not editing, the form submit creates a new account
			return m, textinput.Blink
		}
		if m.addMethod != addMethodCustom {
			m.addStep = addStepTool
			m.addToolIdx = 0
		}
	}
	return m, nil
}

func (m AccountsModel) updateAddTool(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, DefaultKeyMap.Escape):
		m.addStep = addStepMethod
		return m, nil
	case key.Matches(msg, DefaultKeyMap.Up):
		if m.addToolIdx > 0 {
			m.addToolIdx--
		}
	case key.Matches(msg, DefaultKeyMap.Down):
		if m.addToolIdx < len(m.addTools)-1 {
			m.addToolIdx++
		}
	case key.Matches(msg, DefaultKeyMap.Enter):
		if len(m.addTools) == 0 {
			return m, nil
		}
		tmpl := m.addTools[m.addToolIdx]
		// Pre-fill label with "ToolName (N)" where N avoids collision
		label := nextLabel(tmpl.Label, m.accounts)
		m.addLabel = textinput.New()
		m.addLabel.Placeholder = tmpl.Label
		m.addLabel.CharLimit = 32
		m.addLabel.Width = 30
		m.addLabel.SetValue(label)
		m.addLabel.Focus()
		m.addStep = addStepLabel
		return m, textinput.Blink
	}
	return m, nil
}

func (m AccountsModel) updateAddLabel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, DefaultKeyMap.Escape):
		m.addStep = addStepTool
		return m, nil
	case key.Matches(msg, DefaultKeyMap.Enter):
		label := m.addLabel.Value()
		if label == "" {
			m.message = "Account name is required"
			return m, nil
		}
		tmpl := m.addTools[m.addToolIdx]
		newAcct := config.CloneAccount(tmpl, label, m.accounts)

		// Auto-set isolated config dir for sub accounts
		if envVar, ok := config.ConfigDirEnvVars[newAcct.Command]; ok {
			config.SetAccountKey(m.keys, newAcct.ID, envVar, config.AccountConfigDir(newAcct.ID))
		}

		m.accounts = append(m.accounts, newAcct)
		m.cursor = len(m.accounts) - 1

		// Save immediately so auth/key operations can use the config
		m.cfg.Accounts = m.accounts
		_ = config.Save(m.cfg, "")
		_ = config.SaveKeys(m.keys)

		if m.addMethod == addMethodSub {
			// Launch auth command
			m.addStep = addStepNone
			if !newAcct.HasAuth() {
				m.message = "Account added (no auth command configured)"
				return m, nil
			}
			cmd, args := newAcct.AuthCommand()
			c := exec.Command(cmd, args...)
			applyAccountEnv(c, m.keys, newAcct.ID)
			accountID := newAcct.ID
			m.message = ""
			return m, tea.ExecProcess(c, func(err error) tea.Msg {
				return authDoneMsg{err: err, accountID: accountID}
			})
		}

		// API key path
		m.addStep = addStepKey
		m.addKeyName = textinput.New()
		m.addKeyName.Placeholder = "API_KEY_NAME"
		m.addKeyName.CharLimit = 64
		m.addKeyName.Width = 40
		// Pre-fill with suggested env var
		if vars, ok := config.SuggestedEnvVars[newAcct.Command]; ok && len(vars) > 0 {
			m.addKeyName.SetValue(vars[0])
		}
		m.addKeyValue = textinput.New()
		m.addKeyValue.Placeholder = "sk-..."
		m.addKeyValue.CharLimit = 256
		m.addKeyValue.Width = 40
		m.addKeyValue.EchoMode = textinput.EchoPassword
		m.addKeyIdx = 0
		m.addKeyName.Focus()
		return m, textinput.Blink

	default:
		var cmd tea.Cmd
		m.addLabel, cmd = m.addLabel.Update(msg)
		return m, cmd
	}
}

func (m AccountsModel) updateAddKeyInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, DefaultKeyMap.Escape):
		// Cancel key entry, but account is already created
		m.addStep = addStepNone
		m.message = "Account added (no API key set)"
		return m, nil
	case key.Matches(msg, DefaultKeyMap.Tab):
		if m.addKeyIdx == 0 {
			m.addKeyName.Blur()
			m.addKeyIdx = 1
			m.addKeyValue.Focus()
		} else {
			m.addKeyValue.Blur()
			m.addKeyIdx = 0
			m.addKeyName.Focus()
		}
		return m, textinput.Blink
	case key.Matches(msg, DefaultKeyMap.Enter):
		if m.addKeyIdx == 0 {
			m.addKeyName.Blur()
			m.addKeyIdx = 1
			m.addKeyValue.Focus()
			return m, textinput.Blink
		}
		// Submit
		name := m.addKeyName.Value()
		value := m.addKeyValue.Value()
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
		_ = config.SaveKeys(m.keys)
		m.addStep = addStepNone
		m.message = "Account added with API key"
		return m, nil
	default:
		var cmd tea.Cmd
		if m.addKeyIdx == 0 {
			m.addKeyName, cmd = m.addKeyName.Update(msg)
		} else {
			m.addKeyValue, cmd = m.addKeyValue.Update(msg)
		}
		return m, cmd
	}
}

func (m AccountsModel) updateForm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, DefaultKeyMap.Escape):
		m.editing = false
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
			// Custom add path (from addMethodCustom)
			id := config.UniqueAccountID(name, m.accounts)
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
		deletedID := m.accounts[m.cursor].ID
		m.accounts = append(m.accounts[:m.cursor], m.accounts[m.cursor+1:]...)
		if m.cursor >= len(m.accounts) {
			m.cursor = len(m.accounts) - 1
		}
		delete(m.keys, deletedID)
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

	// Guided add wizard views
	if m.addStep != addStepNone {
		return s.String() + m.viewAddWizard()
	}

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

	if m.editing || m.hasCustomForm() {
		title := "Edit Account"
		if !m.editing {
			title = "Add Account (Custom)"
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

		authBadge := ""
		if a.AuthUser != "" {
			authBadge = SuccessStyle.Render(" (" + a.AuthUser + ")")
		} else if ak := config.KeysForAccount(m.keys, a.ID); len(ak) > 0 {
			authBadge = SuccessStyle.Render(" (API)")
		} else if a.HasAuth() {
			authBadge = DimStyle.Render(" (sub)")
		}

		s.WriteString(fmt.Sprintf("  %s %s %s%s  %s  %s\n",
			prefix, enabledMark, label, authBadge, pathMark, cmdStr))
	}

	if m.message != "" {
		s.WriteString("\n  " + WarningStyle.Render(m.message) + "\n")
	}

	s.WriteString("\n  " + DimStyle.Render("Space toggle  a add  e edit  i install  l login  k keys  d delete  Esc save & quit") + "\n")
	return s.String()
}

func (m AccountsModel) viewAddWizard() string {
	var s strings.Builder

	switch m.addStep {
	case addStepMethod:
		s.WriteString("  " + TitleStyle.Render("Add Account") + "\n\n")
		s.WriteString("  " + DimStyle.Render("How do you want to authenticate?") + "\n\n")
		methods := []string{"Subscription login", "API key", "Custom (advanced)"}
		for i, label := range methods {
			if i == m.addMethodIdx {
				s.WriteString(fmt.Sprintf("  %s %s\n", TitleStyle.Render("▸"), WhiteStyle.Render(label)))
			} else {
				s.WriteString(fmt.Sprintf("    %s\n", DimStyle.Render(label)))
			}
		}
		s.WriteString("\n  " + DimStyle.Render("↑↓ select  Enter choose  Esc cancel") + "\n")

	case addStepTool:
		methodName := "Subscription login"
		if m.addMethod == addMethodAPI {
			methodName = "API key"
		}
		s.WriteString("  " + TitleStyle.Render("Add Account") + " " + DimStyle.Render("— "+methodName) + "\n\n")
		s.WriteString("  " + DimStyle.Render("Which tool?") + "\n\n")
		if len(m.addTools) == 0 {
			s.WriteString("  " + DimStyle.Render("No tools available for this method") + "\n")
		} else {
			for i, t := range m.addTools {
				if i == m.addToolIdx {
					s.WriteString(fmt.Sprintf("  %s %s %s\n", TitleStyle.Render("▸"), t.Icon, WhiteStyle.Render(t.Label)))
				} else {
					s.WriteString(fmt.Sprintf("    %s %s\n", t.Icon, DimStyle.Render(t.Label)))
				}
			}
		}
		s.WriteString("\n  " + DimStyle.Render("↑↓ select  Enter choose  Esc back") + "\n")

	case addStepLabel:
		s.WriteString("  " + TitleStyle.Render("Add Account") + "\n\n")
		s.WriteString("  " + DimStyle.Render("Account name:") + "\n\n")
		s.WriteString("  " + m.addLabel.View() + "\n")
		if m.message != "" {
			s.WriteString("\n  " + ErrorStyle.Render(m.message) + "\n")
		}
		s.WriteString("\n  " + DimStyle.Render("Enter continue  Esc back") + "\n")

	case addStepKey:
		a := m.accounts[m.cursor]
		s.WriteString("  " + TitleStyle.Render("Add API Key") + " " + DimStyle.Render("for "+a.ID) + "\n\n")
		nameLabel := DimStyle.Render("  Env Var ")
		valLabel := DimStyle.Render("  Value  ")
		if m.addKeyIdx == 0 {
			nameLabel = TitleStyle.Render("  Env Var ")
		} else {
			valLabel = TitleStyle.Render("  Value  ")
		}
		s.WriteString(fmt.Sprintf("%s  %s\n", nameLabel, m.addKeyName.View()))
		s.WriteString(fmt.Sprintf("%s  %s\n", valLabel, m.addKeyValue.View()))
		if m.message != "" {
			s.WriteString("\n  " + ErrorStyle.Render(m.message) + "\n")
		}
		s.WriteString("\n  " + DimStyle.Render("Tab next  Enter submit  Esc skip") + "\n")
	}

	return s.String()
}

// hasCustomForm returns true if the full 6-field form is active for a custom add
// (not editing an existing account).
func (m AccountsModel) hasCustomForm() bool {
	return !m.editing && len(m.inputs) > 0 && m.addStep == addStepNone
}

func (m *AccountsModel) refreshKeysList() {
	accountID := m.accounts[m.cursor].ID
	ak := config.KeysForAccount(m.keys, accountID)
	m.keysList = nil
	for name, val := range ak {
		m.keysList = append(m.keysList, keyEntry{name: name, value: val})
	}
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
		a := m.accounts[m.cursor]
		if vars, ok := config.SuggestedEnvVars[a.Command]; ok && len(vars) > 0 {
			m.keyInputs[0].SetValue(vars[0])
		}
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

// applyAccountEnv injects account API keys as env vars into the command.
func applyAccountEnv(c *exec.Cmd, keys config.AccountKeys, accountID string) {
	accountKeys := config.KeysForAccount(keys, accountID)
	if len(accountKeys) > 0 {
		if c.Env == nil {
			c.Env = os.Environ()
		}
		for k, v := range accountKeys {
			c.Env = append(c.Env, k+"="+v)
		}
	}
}

// accountEnvSlice returns env vars for an account as a []string slice.
func accountEnvSlice(keys config.AccountKeys, accountID string) []string {
	ak := config.KeysForAccount(keys, accountID)
	var env []string
	for k, v := range ak {
		env = append(env, k+"="+v)
	}
	return env
}

// probeAuthCmd returns a tea.Cmd that probes auth status for the given account.
func probeAuthCmd(accountID, command string, env []string) tea.Cmd {
	return func() tea.Msg {
		email, org, err := config.ProbeAuthUser(command, env)
		return authProbeMsg{
			accountID: accountID,
			email:     email,
			org:       org,
			err:       err,
		}
	}
}

// toolsWithAuth returns DefaultAccounts that have an AuthCmd configured.
func toolsWithAuth() []config.Account {
	var result []config.Account
	for _, a := range config.DefaultAccounts {
		if a.HasAuth() {
			result = append(result, a)
		}
	}
	return result
}

// toolsWithEnvVars returns DefaultAccounts that have SuggestedEnvVars entries.
func toolsWithEnvVars() []config.Account {
	var result []config.Account
	for _, a := range config.DefaultAccounts {
		if _, ok := config.SuggestedEnvVars[a.Command]; ok {
			result = append(result, a)
		}
	}
	return result
}

// nextLabel generates a non-colliding label like "Claude Code (2)".
func nextLabel(base string, existing []config.Account) string {
	// Check if base label is already in use
	taken := false
	for _, a := range existing {
		if strings.EqualFold(a.Label, base) {
			taken = true
			break
		}
	}
	if !taken {
		return base
	}
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s (%d)", base, n)
		found := false
		for _, a := range existing {
			if strings.EqualFold(a.Label, candidate) {
				found = true
				break
			}
		}
		if !found {
			return candidate
		}
	}
}
