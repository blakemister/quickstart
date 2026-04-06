package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/bcmister/qs/internal/config"
	"github.com/bcmister/qs/internal/monitor"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type setupStep int

const (
	stepWelcome setupStep = iota
	stepProjectsRoot
	stepMonitors
	stepAccounts
	stepKeys
	stepConfirm
	stepDone
)

// SetupModel is the multi-step setup wizard
type SetupModel struct {
	existingCfg  *config.Config
	step         setupStep
	width        int
	height       int
	err          error

	// Step 1: Projects root
	rootInput    textinput.Model
	projectsRoot string

	// Step 2: Monitors
	monitors     []monitor.Monitor
	monitorIdx   int
	windowCounts []int

	// Step 3: Accounts
	accounts     []config.Account
	accountIdx   int

	// Step 3 sub-form: guided add wizard
	addStep      addStep
	addMethod    addMethod
	addMethodIdx int
	addTools     []config.Account
	addToolIdx   int
	addLabel     textinput.Model
	addInputs    []textinput.Model // for custom path
	addInputIdx  int
	addKeyName   textinput.Model
	addKeyValue  textinput.Model
	addKeyIdx    int
	authMessage  string

	// Step 4: API Keys
	keys           config.AccountKeys
	keysAccountIdx int
	keysMode       string // "select" or "edit"
	keysList       []setupKeyEntry
	keysEditIdx    int
	keysAdding     bool
	keysKeyInput   textinput.Model
	keysValInput   textinput.Model
	keysAddIdx     int // 0=name, 1=value

	// Final config
	savedPath    string
}

// setupKeyEntry holds a key name and value for display in setup
type setupKeyEntry struct {
	name  string
	value string
}

// NewSetup creates a new setup wizard model
func NewSetup(existingCfg *config.Config) SetupModel {
	// Projects root input
	rootInput := textinput.New()
	rootInput.Placeholder = config.DefaultProjectsRoot()
	rootInput.CharLimit = 256
	rootInput.Width = 50

	defaultRoot := config.DefaultProjectsRoot()
	if existingCfg != nil && existingCfg.ProjectsRoot != "" {
		defaultRoot = existingCfg.ProjectsRoot
	}
	rootInput.SetValue(defaultRoot)

	// Copy default accounts
	accounts := make([]config.Account, len(config.DefaultAccounts))
	for i, a := range config.DefaultAccounts {
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

	// If existing config has accounts, use those instead
	if existingCfg != nil && len(existingCfg.Accounts) > 0 {
		accounts = make([]config.Account, len(existingCfg.Accounts))
		for i, a := range existingCfg.Accounts {
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
	}

	keys, _ := config.LoadKeys()

	return SetupModel{
		existingCfg: existingCfg,
		step:        stepWelcome,
		rootInput:   rootInput,
		accounts:    accounts,
		keys:        keys,
		keysMode:    "select",
	}
}

func (m SetupModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m SetupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case authDoneMsg:
		if msg.err != nil {
			m.authMessage = "Auth failed: " + msg.err.Error()
		} else {
			m.authMessage = "Auth completed — probing status..."
			accountID := msg.accountID
			a := config.AccountByID(m.accounts, accountID)
			if a != nil {
				env := accountEnvSlice(m.keys, accountID)
				return m, probeAuthCmd(accountID, a.Command, env)
			}
			m.authMessage = "Auth completed successfully"
		}
		return m, nil

	case authProbeMsg:
		if msg.err != nil {
			m.authMessage = "Auth OK (could not probe status: " + msg.err.Error() + ")"
		} else if msg.email != "" {
			authUser := msg.email
			if msg.org != "" {
				authUser += " (" + msg.org + ")"
			}
			for i := range m.accounts {
				if m.accounts[i].ID == msg.accountID {
					m.accounts[i].AuthUser = authUser
					break
				}
			}
			m.authMessage = "Logged in as " + authUser
		} else {
			m.authMessage = "Auth completed successfully"
		}
		return m, nil

	case installDoneMsg:
		if msg.err != nil {
			m.authMessage = "Install failed: " + msg.err.Error()
		} else {
			m.authMessage = "Install completed successfully"
		}
		return m, nil

	case monitorsDetectedMsg:
		m.monitors = msg.monitors
		m.windowCounts = make([]int, len(m.monitors))
		for i := range m.windowCounts {
			m.windowCounts[i] = 1
			// Use existing config if available
			if m.existingCfg != nil && i < len(m.existingCfg.Monitors) {
				count := m.existingCfg.Monitors[i].WindowCount()
				if count >= 1 {
					m.windowCounts[i] = count
				}
			}
		}
		return m, nil

	case tea.KeyMsg:
		// Handle guided add wizard
		if m.addStep != addStepNone {
			return m.updateSetupAddWizard(msg)
		}
		// Handle custom form
		if len(m.addInputs) > 0 {
			return m.updateAddAccount(msg)
		}

		switch m.step {
		case stepWelcome:
			return m.updateWelcome(msg)
		case stepProjectsRoot:
			return m.updateProjectsRoot(msg)
		case stepMonitors:
			return m.updateMonitors(msg)
		case stepAccounts:
			return m.updateAccounts(msg)
		case stepKeys:
			return m.updateStepKeys(msg)
		case stepConfirm:
			return m.updateConfirm(msg)
		case stepDone:
			return m, tea.Quit
		}
	}

	// Pass to text input if active
	if m.step == stepProjectsRoot {
		var cmd tea.Cmd
		m.rootInput, cmd = m.rootInput.Update(msg)
		return m, cmd
	}

	if len(m.addInputs) > 0 {
		var cmd tea.Cmd
		m.addInputs[m.addInputIdx], cmd = m.addInputs[m.addInputIdx].Update(msg)
		return m, cmd
	}

	if m.step == stepKeys && m.keysAdding {
		var cmd tea.Cmd
		if m.keysAddIdx == 0 {
			m.keysKeyInput, cmd = m.keysKeyInput.Update(msg)
		} else {
			m.keysValInput, cmd = m.keysValInput.Update(msg)
		}
		return m, cmd
	}

	return m, nil
}

func (m SetupModel) updateWelcome(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, DefaultKeyMap.Enter):
		m.step = stepProjectsRoot
		m.rootInput.Focus()
		return m, textinput.Blink
	case key.Matches(msg, DefaultKeyMap.Quit):
		return m, tea.Quit
	}
	return m, nil
}

func (m SetupModel) updateProjectsRoot(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, DefaultKeyMap.Enter):
		m.projectsRoot = m.rootInput.Value()
		if m.projectsRoot == "" {
			m.projectsRoot = config.DefaultProjectsRoot()
		}

		// Create directory if it doesn't exist
		if _, err := os.Stat(m.projectsRoot); os.IsNotExist(err) {
			os.MkdirAll(m.projectsRoot, 0755)
		}

		// Detect monitors and move to that step
		m.step = stepMonitors
		return m, detectMonitors
	case key.Matches(msg, DefaultKeyMap.Escape):
		m.step = stepWelcome
		m.rootInput.Blur()
		return m, nil
	default:
		var cmd tea.Cmd
		m.rootInput, cmd = m.rootInput.Update(msg)
		return m, cmd
	}
}

func (m SetupModel) updateMonitors(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, DefaultKeyMap.Enter):
		m.step = stepAccounts
		return m, nil
	case key.Matches(msg, DefaultKeyMap.Escape):
		m.step = stepProjectsRoot
		m.rootInput.Focus()
		return m, textinput.Blink
	case msg.String() == "left", msg.String() == "h":
		if m.monitorIdx > 0 {
			m.monitorIdx--
		}
	case msg.String() == "right", msg.String() == "l":
		if m.monitorIdx < len(m.monitors)-1 {
			m.monitorIdx++
		}
	case key.Matches(msg, DefaultKeyMap.Up):
		if len(m.monitors) > 0 && m.windowCounts[m.monitorIdx] < 9 {
			m.windowCounts[m.monitorIdx]++
		}
	case key.Matches(msg, DefaultKeyMap.Down):
		if len(m.monitors) > 0 && m.windowCounts[m.monitorIdx] > 1 {
			m.windowCounts[m.monitorIdx]--
		}
	}
	return m, nil
}

func (m SetupModel) updateAccounts(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, DefaultKeyMap.Enter):
		// Check at least one account enabled
		enabled := config.EnabledAccounts(m.accounts)
		if len(enabled) == 0 {
			return m, nil
		}
		m.step = stepKeys
		m.keysMode = "select"
		m.keysAccountIdx = 0
		return m, nil
	case key.Matches(msg, DefaultKeyMap.Escape):
		m.step = stepMonitors
		return m, nil
	case key.Matches(msg, DefaultKeyMap.Up):
		if m.accountIdx > 0 {
			m.accountIdx--
		}
	case key.Matches(msg, DefaultKeyMap.Down):
		if m.accountIdx < len(m.accounts)-1 {
			m.accountIdx++
		}
	case key.Matches(msg, DefaultKeyMap.Space):
		m.accounts[m.accountIdx].Enabled = !m.accounts[m.accountIdx].Enabled
	case msg.String() == "a":
		m.addStep = addStepMethod
		m.addMethodIdx = 0
		m.authMessage = ""
		return m, nil
	case msg.String() == "l":
		a := m.accounts[m.accountIdx]
		if !a.HasAuth() {
			m.authMessage = a.Label + " uses env vars for auth (no login command)"
			return m, nil
		}
		m.authMessage = ""
		cmd, args := a.AuthCommand()
		c := exec.Command(cmd, args...)
		applyAccountEnv(c, m.keys, a.ID)
		accountID := a.ID
		return m, tea.ExecProcess(c, func(err error) tea.Msg {
			return authDoneMsg{err: err, accountID: accountID}
		})
	case msg.String() == "i":
		a := m.accounts[m.accountIdx]
		if !a.HasInstall() {
			m.authMessage = a.Label + " has no install command configured"
			return m, nil
		}
		m.authMessage = ""
		cmd, args := a.InstallCommand()
		c := exec.Command(cmd, args...)
		applyAccountEnv(c, m.keys, a.ID)
		return m, tea.ExecProcess(c, func(err error) tea.Msg {
			return installDoneMsg{err: err}
		})
	}
	return m, nil
}

func (m SetupModel) updateConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, DefaultKeyMap.Enter):
		// Build and save config
		cfg := m.buildConfig()
		path := config.DefaultConfigPath()
		if err := config.Save(cfg, path); err != nil {
			m.err = err
			return m, nil
		}
		// Save API keys
		if err := config.SaveKeys(m.keys); err != nil {
			m.err = err
			return m, nil
		}
		m.savedPath = path
		m.step = stepDone
		return m, nil
	case key.Matches(msg, DefaultKeyMap.Escape):
		m.step = stepKeys
		m.keysMode = "select"
		return m, nil
	}
	return m, nil
}

// updateSetupAddWizard handles the guided add flow in setup wizard.
func (m SetupModel) updateSetupAddWizard(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.addStep {
	case addStepMethod:
		return m.updateSetupAddMethod(msg)
	case addStepTool:
		return m.updateSetupAddTool(msg)
	case addStepLabel:
		return m.updateSetupAddLabel(msg)
	case addStepKey:
		return m.updateSetupAddKeyInput(msg)
	}
	return m, nil
}

func (m SetupModel) updateSetupAddMethod(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
			m.addMethod = addMethodCustom
			m.addStep = addStepNone
			m.addInputs = makeAddAccountInputs()
			m.addInputIdx = 0
			m.addInputs[0].Focus()
			return m, textinput.Blink
		}
		if m.addMethod != addMethodCustom {
			m.addStep = addStepTool
			m.addToolIdx = 0
		}
	}
	return m, nil
}

func (m SetupModel) updateSetupAddTool(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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

func (m SetupModel) updateSetupAddLabel(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, DefaultKeyMap.Escape):
		m.addStep = addStepTool
		return m, nil
	case key.Matches(msg, DefaultKeyMap.Enter):
		label := m.addLabel.Value()
		if label == "" {
			m.authMessage = "Account name is required"
			return m, nil
		}
		tmpl := m.addTools[m.addToolIdx]
		newAcct := config.CloneAccount(tmpl, label, m.accounts)

		if envVar, ok := config.ConfigDirEnvVars[newAcct.Command]; ok {
			config.SetAccountKey(m.keys, newAcct.ID, envVar, config.AccountConfigDir(newAcct.ID))
		}

		m.accounts = append(m.accounts, newAcct)
		m.accountIdx = len(m.accounts) - 1

		if m.addMethod == addMethodSub {
			m.addStep = addStepNone
			if !newAcct.HasAuth() {
				m.authMessage = "Account added (no auth command configured)"
				return m, nil
			}
			cmd, args := newAcct.AuthCommand()
			c := exec.Command(cmd, args...)
			applyAccountEnv(c, m.keys, newAcct.ID)
			accountID := newAcct.ID
			m.authMessage = ""
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

func (m SetupModel) updateSetupAddKeyInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, DefaultKeyMap.Escape):
		m.addStep = addStepNone
		m.authMessage = "Account added (no API key set)"
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
		name := m.addKeyName.Value()
		value := m.addKeyValue.Value()
		if err := config.ValidateEnvVarName(name); err != nil {
			m.authMessage = err.Error()
			return m, nil
		}
		if value == "" {
			m.authMessage = "Value cannot be empty"
			return m, nil
		}
		accountID := m.accounts[m.accountIdx].ID
		config.SetAccountKey(m.keys, accountID, name, value)
		m.addStep = addStepNone
		m.authMessage = "Account added with API key"
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

func (m SetupModel) updateAddAccount(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, DefaultKeyMap.Escape):
		m.addInputs = nil
		return m, nil
	case key.Matches(msg, DefaultKeyMap.Tab):
		m.addInputs[m.addInputIdx].Blur()
		m.addInputIdx = (m.addInputIdx + 1) % len(m.addInputs)
		m.addInputs[m.addInputIdx].Focus()
		return m, textinput.Blink
	case key.Matches(msg, DefaultKeyMap.Enter):
		if m.addInputIdx < len(m.addInputs)-1 {
			m.addInputs[m.addInputIdx].Blur()
			m.addInputIdx++
			m.addInputs[m.addInputIdx].Focus()
			return m, textinput.Blink
		}
		name := m.addInputs[0].Value()
		command := m.addInputs[1].Value()
		args := m.addInputs[2].Value()
		authCmd := m.addInputs[3].Value()
		installCmd := m.addInputs[4].Value()
		icon := m.addInputs[5].Value()

		if name == "" || command == "" {
			return m, nil
		}

		id := config.UniqueAccountID(name, m.accounts)
		if icon == "" {
			icon = "⬜"
		}

		var argList []string
		if args != "" {
			argList = strings.Fields(args)
		}

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
		m.addInputs = nil
		m.accountIdx = len(m.accounts) - 1
		return m, nil
	default:
		var cmd tea.Cmd
		m.addInputs[m.addInputIdx], cmd = m.addInputs[m.addInputIdx].Update(msg)
		return m, cmd
	}
}

func (m SetupModel) updateStepKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.keysAdding {
		return m.updateKeysAdd(msg)
	}
	if m.keysMode == "edit" {
		return m.updateKeysEdit(msg)
	}
	return m.updateKeysSelect(msg)
}

func (m SetupModel) updateKeysSelect(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, DefaultKeyMap.Enter):
		// Enter on an account opens its key editor
		m.keysMode = "edit"
		m.keysEditIdx = 0
		m.refreshSetupKeysList()
		return m, nil
	case msg.String() == "s":
		// Skip — go to confirm
		m.step = stepConfirm
		return m, nil
	case key.Matches(msg, DefaultKeyMap.Escape):
		m.step = stepAccounts
		return m, nil
	case key.Matches(msg, DefaultKeyMap.Up):
		if m.keysAccountIdx > 0 {
			m.keysAccountIdx--
		}
	case key.Matches(msg, DefaultKeyMap.Down):
		if m.keysAccountIdx < len(m.accounts)-1 {
			m.keysAccountIdx++
		}
	}
	return m, nil
}

func (m SetupModel) updateKeysEdit(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, DefaultKeyMap.Escape):
		m.keysMode = "select"
		return m, nil
	case msg.String() == "a":
		m.keysAdding = true
		m.keysKeyInput = textinput.New()
		m.keysKeyInput.Placeholder = "API_KEY_HERE"
		m.keysKeyInput.CharLimit = 64
		m.keysKeyInput.Width = 40
		// Auto-suggest env var name based on account's command
		a := m.accounts[m.keysAccountIdx]
		if vars, ok := config.SuggestedEnvVars[a.Command]; ok && len(vars) > 0 {
			m.keysKeyInput.SetValue(vars[0])
		}
		m.keysKeyInput.Focus()
		m.keysValInput = textinput.New()
		m.keysValInput.Placeholder = "sk-..."
		m.keysValInput.CharLimit = 256
		m.keysValInput.Width = 40
		m.keysValInput.EchoMode = textinput.EchoPassword
		m.keysAddIdx = 0
		return m, textinput.Blink
	case key.Matches(msg, DefaultKeyMap.Delete):
		if len(m.keysList) > 0 {
			entry := m.keysList[m.keysEditIdx]
			accountID := m.accounts[m.keysAccountIdx].ID
			config.DeleteAccountKey(m.keys, accountID, entry.name)
			m.refreshSetupKeysList()
		}
	case key.Matches(msg, DefaultKeyMap.Up):
		if m.keysEditIdx > 0 {
			m.keysEditIdx--
		}
	case key.Matches(msg, DefaultKeyMap.Down):
		if m.keysEditIdx < len(m.keysList)-1 {
			m.keysEditIdx++
		}
	}
	return m, nil
}

func (m SetupModel) updateKeysAdd(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, DefaultKeyMap.Escape):
		m.keysAdding = false
		return m, nil
	case key.Matches(msg, DefaultKeyMap.Tab):
		if m.keysAddIdx == 0 {
			m.keysKeyInput.Blur()
			m.keysAddIdx = 1
			m.keysValInput.Focus()
		} else {
			m.keysValInput.Blur()
			m.keysAddIdx = 0
			m.keysKeyInput.Focus()
		}
		return m, textinput.Blink
	case key.Matches(msg, DefaultKeyMap.Enter):
		if m.keysAddIdx == 0 {
			// Move to value field
			m.keysKeyInput.Blur()
			m.keysAddIdx = 1
			m.keysValInput.Focus()
			return m, textinput.Blink
		}
		// Submit
		name := m.keysKeyInput.Value()
		value := m.keysValInput.Value()
		if err := config.ValidateEnvVarName(name); err != nil {
			return m, nil
		}
		if value == "" {
			return m, nil
		}
		accountID := m.accounts[m.keysAccountIdx].ID
		config.SetAccountKey(m.keys, accountID, name, value)
		m.keysAdding = false
		m.refreshSetupKeysList()
		return m, nil
	default:
		var cmd tea.Cmd
		if m.keysAddIdx == 0 {
			m.keysKeyInput, cmd = m.keysKeyInput.Update(msg)
		} else {
			m.keysValInput, cmd = m.keysValInput.Update(msg)
		}
		return m, cmd
	}
}

func (m *SetupModel) refreshSetupKeysList() {
	accountID := m.accounts[m.keysAccountIdx].ID
	ak := config.KeysForAccount(m.keys, accountID)
	m.keysList = nil
	for name, val := range ak {
		m.keysList = append(m.keysList, setupKeyEntry{name: name, value: val})
	}
	// Sort for stable display
	for i := 1; i < len(m.keysList); i++ {
		for j := i; j > 0 && m.keysList[j].name < m.keysList[j-1].name; j-- {
			m.keysList[j], m.keysList[j-1] = m.keysList[j-1], m.keysList[j]
		}
	}
	if m.keysEditIdx >= len(m.keysList) {
		m.keysEditIdx = 0
	}
}

func (m SetupModel) View() string {
	var s strings.Builder

	s.WriteString(RenderLogo("setup"))

	switch m.step {
	case stepWelcome:
		s.WriteString(m.viewWelcome())
	case stepProjectsRoot:
		s.WriteString(m.viewProjectsRoot())
	case stepMonitors:
		s.WriteString(m.viewMonitors())
	case stepAccounts:
		if m.addStep != addStepNone {
			s.WriteString(m.viewSetupAddWizard())
		} else if len(m.addInputs) > 0 {
			s.WriteString(m.viewAddAccount())
		} else {
			s.WriteString(m.viewAccounts())
		}
	case stepKeys:
		s.WriteString(m.viewStepKeys())
	case stepConfirm:
		s.WriteString(m.viewConfirm())
	case stepDone:
		s.WriteString(m.viewDone())
	}

	return s.String()
}

func (m SetupModel) viewWelcome() string {
	var s strings.Builder
	s.WriteString(RenderSep())
	s.WriteString("\n")
	s.WriteString("  " + SubtitleStyle.Render("Welcome to quickstart!") + "\n\n")
	s.WriteString("  " + DimStyle.Render("This wizard will configure your terminal launcher.") + "\n")
	s.WriteString("  " + DimStyle.Render("You'll set up:") + "\n\n")
	s.WriteString("    " + TitleStyle.Render("1.") + " " + WhiteStyle.Render("Projects folder") + "\n")
	s.WriteString("    " + TitleStyle.Render("2.") + " " + WhiteStyle.Render("Monitor layout") + "\n")
	s.WriteString("    " + TitleStyle.Render("3.") + " " + WhiteStyle.Render("AI tool accounts") + "\n")
	s.WriteString("    " + TitleStyle.Render("4.") + " " + WhiteStyle.Render("API keys") + "\n")
	s.WriteString("\n")
	s.WriteString("  " + DimStyle.Render("Press Enter to begin, q to quit") + "\n")
	return s.String()
}

func (m SetupModel) viewProjectsRoot() string {
	var s strings.Builder
	s.WriteString(RenderSep())
	s.WriteString("\n")
	s.WriteString("  " + TitleStyle.Render("Step 1") + " " + SubtitleStyle.Render("Projects Root") + "\n\n")
	s.WriteString("  " + DimStyle.Render("Directory containing your project folders:") + "\n\n")
	s.WriteString("  " + m.rootInput.View() + "\n\n")

	// Check if directory exists
	if m.rootInput.Value() != "" {
		if _, err := os.Stat(m.rootInput.Value()); os.IsNotExist(err) {
			s.WriteString("  " + WarningStyle.Render("Directory does not exist — will be created") + "\n")
		} else {
			s.WriteString("  " + SuccessStyle.Render("✓ Directory exists") + "\n")
		}
	}

	s.WriteString("\n  " + DimStyle.Render("Enter to continue, Esc to go back") + "\n")
	return s.String()
}

func (m SetupModel) viewMonitors() string {
	var s strings.Builder
	s.WriteString(RenderSep())
	s.WriteString("\n")
	s.WriteString("  " + TitleStyle.Render("Step 2") + " " + SubtitleStyle.Render("Monitor Layout") + "\n\n")

	if len(m.monitors) == 0 {
		s.WriteString("  " + DimStyle.Render("Detecting monitors...") + "\n")
		return s.String()
	}

	s.WriteString(fmt.Sprintf("  %s %d monitors detected\n\n",
		SuccessStyle.Render("✓"),
		len(m.monitors)))

	for i, mon := range m.monitors {
		selected := i == m.monitorIdx

		label := fmt.Sprintf("Monitor %d", i+1)
		if mon.Primary {
			label += " (Primary)"
		}

		res := fmt.Sprintf("%d×%d", mon.Width, mon.Height)
		windows := m.windowCounts[i]
		layout := LayoutForCount(windows)

		var panel string
		if selected {
			panel = fmt.Sprintf("  %s %s  %s  %s windows  %s\n",
				TitleStyle.Render("▸"),
				SubtitleStyle.Render(label),
				DimStyle.Render(res),
				TitleStyle.Render(strconv.Itoa(windows)),
				DimStyle.Render("("+layout+")"))
		} else {
			panel = fmt.Sprintf("    %s  %s  %d windows  %s\n",
				DimStyle.Render(label),
				DimStyle.Render(res),
				windows,
				DimStyle.Render("("+layout+")"))
		}
		s.WriteString(panel)

		// Draw ASCII layout preview for selected monitor
		if selected {
			s.WriteString(renderLayoutPreview(windows))
			s.WriteString("\n")
		}
	}

	s.WriteString("\n  " + DimStyle.Render("←→ select monitor  ↑↓ adjust windows  Enter to continue") + "\n")
	return s.String()
}

func (m SetupModel) viewAccounts() string {
	var s strings.Builder
	s.WriteString(RenderSep())
	s.WriteString("\n")
	s.WriteString("  " + TitleStyle.Render("Step 3") + " " + SubtitleStyle.Render("AI Tool Accounts") + "\n\n")
	s.WriteString("  " + DimStyle.Render("Toggle accounts with Space, add custom with 'a'") + "\n\n")

	for i, a := range m.accounts {
		selected := i == m.accountIdx

		// Check if command is on PATH
		onPath := false
		if _, err := exec.LookPath(a.Command); err == nil {
			onPath = true
		}

		// Build status indicators
		enabledMark := DimStyle.Render("[ ]")
		if a.Enabled {
			enabledMark = SuccessStyle.Render("[✓]")
		}

		pathMark := ErrorStyle.Render("✗")
		if onPath {
			pathMark = SuccessStyle.Render("✓")
		}

		prefix := "  "
		if selected {
			prefix = TitleStyle.Render("▸ ")
		}

		label := a.Icon + " " + a.Label
		if selected {
			label = SubtitleStyle.Render(a.Icon + " " + a.Label)
		} else {
			label = DimStyle.Render(a.Icon + " " + a.Label)
		}

		authBadge := ""
		if a.AuthUser != "" {
			authBadge = SuccessStyle.Render(" (" + a.AuthUser + ")")
		} else if ak := config.UserAPIKeys(m.keys, a.ID); len(ak) > 0 {
			authBadge = SuccessStyle.Render(" (API)")
		} else if a.HasAuth() {
			authBadge = DimStyle.Render(" (sub)")
		}

		s.WriteString(fmt.Sprintf("  %s %s %s%s  %s  %s\n",
			prefix, enabledMark, label, authBadge, pathMark,
			DimStyle.Render(a.Command)))
	}

	enabledCount := len(config.EnabledAccounts(m.accounts))
	s.WriteString(fmt.Sprintf("\n  %s %d accounts enabled\n",
		DimStyle.Render("·"),
		enabledCount))

	if m.authMessage != "" {
		s.WriteString("\n  " + WarningStyle.Render(m.authMessage) + "\n")
	}

	s.WriteString("\n  " + DimStyle.Render("Space toggle  a add  i install  l login  Enter to continue  Esc back") + "\n")
	return s.String()
}

func (m SetupModel) viewAddAccount() string {
	var s strings.Builder
	s.WriteString(RenderSep())
	s.WriteString("\n")
	s.WriteString("  " + TitleStyle.Render("Add Account (Custom)") + "\n\n")

	labels := []string{"Name", "Command", "Args", "Auth Cmd", "Install", "Icon"}
	for i, input := range m.addInputs {
		active := i == m.addInputIdx
		label := DimStyle.Render(labels[i] + ":")
		if active {
			label = TitleStyle.Render(labels[i] + ":")
		}
		s.WriteString(fmt.Sprintf("  %s  %s\n", label, input.View()))
	}

	s.WriteString("\n  " + DimStyle.Render("Tab next field  Enter submit  Esc cancel") + "\n")
	return s.String()
}

func (m SetupModel) viewSetupAddWizard() string {
	var s strings.Builder
	s.WriteString(RenderSep())
	s.WriteString("\n")

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
		if m.authMessage != "" {
			s.WriteString("\n  " + ErrorStyle.Render(m.authMessage) + "\n")
		}
		s.WriteString("\n  " + DimStyle.Render("Enter continue  Esc back") + "\n")

	case addStepKey:
		a := m.accounts[m.accountIdx]
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
		if m.authMessage != "" {
			s.WriteString("\n  " + ErrorStyle.Render(m.authMessage) + "\n")
		}
		s.WriteString("\n  " + DimStyle.Render("Tab next  Enter submit  Esc skip") + "\n")
	}

	return s.String()
}

func (m SetupModel) viewStepKeys() string {
	var s strings.Builder
	s.WriteString(RenderSep())
	s.WriteString("\n")

	if m.keysAdding {
		a := m.accounts[m.keysAccountIdx]
		s.WriteString("  " + TitleStyle.Render("Add API Key") + " " + DimStyle.Render("for "+a.ID) + "\n\n")

		nameLabel := DimStyle.Render("  Env Var ")
		valLabel := DimStyle.Render("  Value  ")
		if m.keysAddIdx == 0 {
			nameLabel = TitleStyle.Render("  Env Var ")
		} else {
			valLabel = TitleStyle.Render("  Value  ")
		}
		s.WriteString(fmt.Sprintf("%s  %s\n", nameLabel, m.keysKeyInput.View()))
		s.WriteString(fmt.Sprintf("%s  %s\n", valLabel, m.keysValInput.View()))

		s.WriteString("\n  " + DimStyle.Render("Tab next  Enter submit  Esc cancel") + "\n")
		return s.String()
	}

	if m.keysMode == "edit" {
		a := m.accounts[m.keysAccountIdx]
		s.WriteString("  " + TitleStyle.Render("Step 4") + " " + SubtitleStyle.Render("API Keys for "+a.ID) + "\n\n")

		if len(m.keysList) == 0 {
			s.WriteString("  " + DimStyle.Render("No keys configured") + "\n")
		} else {
			for i, entry := range m.keysList {
				prefix := "  "
				if i == m.keysEditIdx {
					prefix = TitleStyle.Render("▸ ")
				}
				nameStr := WhiteStyle.Render(entry.name)
				if i != m.keysEditIdx {
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

	// Account selection mode
	s.WriteString("  " + TitleStyle.Render("Step 4") + " " + SubtitleStyle.Render("API Keys") + "\n\n")
	s.WriteString("  " + DimStyle.Render("Select an account to add API keys, or press s to skip.") + "\n\n")

	for i, a := range m.accounts {
		prefix := "  "
		if i == m.keysAccountIdx {
			prefix = TitleStyle.Render("▸ ")
		}

		keyCount := len(config.KeysForAccount(m.keys, a.ID))
		badge := DimStyle.Render(fmt.Sprintf("[%d keys]", keyCount))

		label := a.Icon + " " + a.Label
		if i == m.keysAccountIdx {
			label = SubtitleStyle.Render(a.Icon + " " + a.Label)
		} else {
			label = DimStyle.Render(a.Icon + " " + a.Label)
		}

		s.WriteString(fmt.Sprintf("  %s %s %s  %s\n",
			prefix, badge, label, DimStyle.Render(a.ID)))
	}

	s.WriteString("\n  " + DimStyle.Render("↑↓ select  Enter edit keys  s skip  Esc back") + "\n")
	return s.String()
}

func (m SetupModel) viewConfirm() string {
	var s strings.Builder
	s.WriteString(RenderSep())
	s.WriteString("\n")
	s.WriteString("  " + TitleStyle.Render("Step 5") + " " + SubtitleStyle.Render("Confirm") + "\n\n")

	// Projects root
	s.WriteString(fmt.Sprintf("  %s  %s\n",
		DimStyle.Render("Projects:"),
		WhiteStyle.Render(m.projectsRoot)))

	// Monitors
	for i, count := range m.windowCounts {
		layout := LayoutForCount(count)
		s.WriteString(fmt.Sprintf("  %s  %d windows (%s)\n",
			DimStyle.Render(fmt.Sprintf("Monitor %d:", i+1)),
			count, layout))
	}

	// Enabled accounts
	s.WriteString("\n  " + DimStyle.Render("Accounts:") + "\n")
	for _, a := range m.accounts {
		if a.Enabled {
			keyCount := len(config.KeysForAccount(m.keys, a.ID))
			keysBadge := ""
			if keyCount > 0 {
				keysBadge = DimStyle.Render(fmt.Sprintf("  [%d keys]", keyCount))
			}
			s.WriteString(fmt.Sprintf("    %s %s  %s%s\n",
				a.Icon,
				WhiteStyle.Render(a.Label),
				DimStyle.Render(a.FullCommand()),
				keysBadge))
		}
	}

	if m.err != nil {
		s.WriteString("\n  " + ErrorStyle.Render("Error: "+m.err.Error()) + "\n")
	}

	s.WriteString("\n  " + DimStyle.Render("Enter to save, Esc to go back") + "\n")
	return s.String()
}

func (m SetupModel) viewDone() string {
	var s strings.Builder
	s.WriteString(RenderSep())
	s.WriteString("\n")
	s.WriteString("  " + SuccessStyle.Render("✓") + " " + SubtitleStyle.Render("Configuration saved!") + "\n\n")
	s.WriteString(fmt.Sprintf("  %s %s\n",
		DimStyle.Render("▸"),
		DimStyle.Render(m.savedPath)))
	s.WriteString("\n  " + DimStyle.Render("Run") + " " + TitleStyle.Render("qs") + " " + DimStyle.Render("to launch.") + "\n\n")
	return s.String()
}

func (m SetupModel) buildConfig() *config.Config {
	monitors := make([]config.MonitorConfig, len(m.windowCounts))
	for i, count := range m.windowCounts {
		windows := make([]config.WindowConfig, count)
		for j := range windows {
			windows[j] = config.WindowConfig{Tool: "claude"}
		}
		monitors[i] = config.MonitorConfig{
			Layout:  LayoutForCount(count),
			Windows: windows,
		}
	}

	return &config.Config{
		Version:        4,
		ProjectsRoot:   m.projectsRoot,
		DefaultAccount: "claude",
		LastAccount:    "claude",
		Accounts:       m.accounts,
		Monitors:       monitors,
	}
}

func LayoutForCount(count int) string {
	switch {
	case count <= 1:
		return "full"
	case count == 2:
		return "vertical"
	default:
		return "grid"
	}
}

func renderLayoutPreview(count int) string {
	var s strings.Builder
	switch {
	case count <= 1:
		s.WriteString("      ┌──────────┐\n")
		s.WriteString("      │          │\n")
		s.WriteString("      │          │\n")
		s.WriteString("      └──────────┘")
	case count == 2:
		s.WriteString("      ┌─────┬─────┐\n")
		s.WriteString("      │     │     │\n")
		s.WriteString("      │     │     │\n")
		s.WriteString("      └─────┴─────┘")
	case count == 3:
		s.WriteString("      ┌─────┬─────┐\n")
		s.WriteString("      │     │     │\n")
		s.WriteString("      ├─────┤     │\n")
		s.WriteString("      │     │     │\n")
		s.WriteString("      └─────┴─────┘")
	default:
		s.WriteString("      ┌─────┬─────┐\n")
		s.WriteString("      │     │     │\n")
		s.WriteString("      ├─────┼─────┤\n")
		s.WriteString("      │     │     │\n")
		s.WriteString("      └─────┴─────┘")
	}
	return DimStyle.Render(s.String())
}

func makeAddAccountInputs() []textinput.Model {
	inputs := make([]textinput.Model, 6)

	inputs[0] = textinput.New()
	inputs[0].Placeholder = "My Tool"
	inputs[0].CharLimit = 32
	inputs[0].Width = 30

	inputs[1] = textinput.New()
	inputs[1].Placeholder = "tool-name"
	inputs[1].CharLimit = 64
	inputs[1].Width = 30

	inputs[2] = textinput.New()
	inputs[2].Placeholder = "--flag1 --flag2"
	inputs[2].CharLimit = 128
	inputs[2].Width = 30

	inputs[3] = textinput.New()
	inputs[3].Placeholder = "command login"
	inputs[3].CharLimit = 128
	inputs[3].Width = 30

	inputs[4] = textinput.New()
	inputs[4].Placeholder = "npm i -g package-name"
	inputs[4].CharLimit = 128
	inputs[4].Width = 30

	inputs[5] = textinput.New()
	inputs[5].Placeholder = "⬜"
	inputs[5].CharLimit = 4
	inputs[5].Width = 10

	return inputs
}

// monitorsDetectedMsg is sent when monitor detection completes
type monitorsDetectedMsg struct {
	monitors []monitor.Monitor
}

func detectMonitors() tea.Msg {
	monitors, err := monitor.Detect()
	if err != nil {
		return monitorsDetectedMsg{monitors: nil}
	}
	return monitorsDetectedMsg{monitors: monitors}
}
