package config

import "strings"

// Account represents a configured AI tool account
type Account struct {
	ID         string   `yaml:"id"`
	Label      string   `yaml:"label"`
	Command    string   `yaml:"command"`
	Args       []string `yaml:"args"`
	AuthCmd    string   `yaml:"authCmd,omitempty"`
	InstallCmd string   `yaml:"installCmd,omitempty"`
	Icon       string   `yaml:"icon"`
	Enabled    bool     `yaml:"enabled"`
}

// AuthCommand splits AuthCmd into command and args.
func (a *Account) AuthCommand() (string, []string) {
	parts := strings.Fields(a.AuthCmd)
	if len(parts) == 0 {
		return "", nil
	}
	return parts[0], parts[1:]
}

// HasAuth returns true if this account has an auth command configured.
func (a *Account) HasAuth() bool {
	return strings.TrimSpace(a.AuthCmd) != ""
}

// InstallCommand splits InstallCmd into command and args.
func (a *Account) InstallCommand() (string, []string) {
	parts := strings.Fields(a.InstallCmd)
	if len(parts) == 0 {
		return "", nil
	}
	return parts[0], parts[1:]
}

// HasInstall returns true if this account has an install command configured.
func (a *Account) HasInstall() bool {
	return strings.TrimSpace(a.InstallCmd) != ""
}

// DefaultAccounts returns the built-in account definitions
var DefaultAccounts = []Account{
	{
		ID:         "claude",
		Label:      "Claude Code",
		Command:    "claude",
		Args:       []string{"--dangerously-skip-permissions"},
		AuthCmd:    "claude /login",
		InstallCmd: "npm i -g @anthropic-ai/claude-code",
		Icon:       "\U0001F7E0",
		Enabled:    true,
	},
	{
		ID:         "codex",
		Label:      "OpenAI Codex",
		Command:    "codex",
		Args:       []string{"--dangerously-bypass-approvals-and-sandbox"},
		AuthCmd:    "codex login",
		InstallCmd: "npm i -g @openai/codex",
		Icon:       "\U0001F7E2",
		Enabled:    true,
	},
	{
		ID:         "gemini",
		Label:      "Gemini CLI",
		Command:    "gemini",
		Args:       []string{"--yolo"},
		AuthCmd:    "gemini",
		InstallCmd: "npm i -g @google/gemini-cli",
		Icon:       "\U0001F535",
		Enabled:    true,
	},
	{
		ID:         "opencode",
		Label:      "OpenCode (z.ai)",
		Command:    "opencode",
		Args:       []string{"--yolo"},
		AuthCmd:    "opencode auth login",
		InstallCmd: "npm i -g opencode",
		Icon:       "\u26AB",
		Enabled:    true,
	},
	{
		ID:      "cursor",
		Label:   "Cursor Agent",
		Command: "agent",
		Args:    []string{},
		AuthCmd: "agent login",
		Icon:    "\U0001F7E1",
		Enabled: true,
	},
}

// AccountByID finds an account by its ID, returns nil if not found
func AccountByID(accounts []Account, id string) *Account {
	for i := range accounts {
		if accounts[i].ID == id {
			return &accounts[i]
		}
	}
	return nil
}

// EnabledAccounts returns only accounts that are enabled
func EnabledAccounts(accounts []Account) []Account {
	var result []Account
	for _, a := range accounts {
		if a.Enabled {
			result = append(result, a)
		}
	}
	return result
}

// FullCommand returns the display string "command args..." for an account
func (a *Account) FullCommand() string {
	if len(a.Args) == 0 {
		return a.Command
	}
	return a.Command + " " + strings.Join(a.Args, " ")
}
