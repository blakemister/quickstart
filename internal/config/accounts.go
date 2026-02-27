package config

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// ConfigDirEnvVars maps tool commands to their config dir env var name.
// Setting this env var gives the tool an isolated auth/config session.
var ConfigDirEnvVars = map[string]string{
	"claude": "CLAUDE_CONFIG_DIR",
}

// AccountConfigDir returns the isolated config directory for an account (~/.qs/auth/<accountID>/).
func AccountConfigDir(accountID string) string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".qs", "auth", accountID)
}

// SuggestedEnvVars maps tool commands to their conventional API key env var names.
var SuggestedEnvVars = map[string][]string{
	"claude":   {"ANTHROPIC_API_KEY", "CLAUDE_CONFIG_DIR"},
	"codex":    {"OPENAI_API_KEY"},
	"gemini":   {"GEMINI_API_KEY"},
	"agent":    {"CURSOR_API_KEY"},
	"opencode": {"OPENAI_API_KEY", "ANTHROPIC_API_KEY"},
}

// UniqueAccountID generates a URL-safe ID from a label, appending -2, -3 etc. on collision.
func UniqueAccountID(label string, existing []Account) string {
	base := strings.ToLower(strings.TrimSpace(label))
	// Replace non-alphanumeric with hyphens
	re := regexp.MustCompile(`[^a-z0-9]+`)
	base = re.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if base == "" {
		base = "account"
	}

	// Check for collision
	candidate := base
	suffix := 2
	for {
		taken := false
		for _, a := range existing {
			if a.ID == candidate {
				taken = true
				break
			}
		}
		if !taken {
			return candidate
		}
		candidate = base + "-" + strconv.Itoa(suffix)
		suffix++
	}
}

// CloneAccount deep-copies an account with a new label and unique ID.
func CloneAccount(src Account, newLabel string, existing []Account) Account {
	args := make([]string, len(src.Args))
	copy(args, src.Args)

	return Account{
		ID:         UniqueAccountID(newLabel, existing),
		Label:      newLabel,
		Command:    src.Command,
		Args:       args,
		AuthCmd:    src.AuthCmd,
		InstallCmd: src.InstallCmd,
		Icon:       src.Icon,
		Enabled:    true,
		AuthUser:   "", // new clone needs fresh auth
	}
}

// AuthStatusCmds maps tool commands to their auth status command.
// The command should return JSON with at least "email" and "orgName" fields.
var AuthStatusCmds = map[string]string{
	"claude": "claude auth status",
}

// authStatusResult is the JSON structure returned by auth status commands.
type authStatusResult struct {
	Email   string `json:"email"`
	OrgName string `json:"orgName"`
}

// ParseAuthStatus extracts email and org from auth status JSON output.
func ParseAuthStatus(data []byte) (email string, org string, err error) {
	var result authStatusResult
	if err := json.Unmarshal(data, &result); err != nil {
		return "", "", err
	}
	return result.Email, result.OrgName, nil
}

// ProbeAuthUser runs the auth status command for the given tool and returns email and org.
// Returns empty strings (no error) if the tool has no auth status command.
func ProbeAuthUser(command string, env []string) (email string, org string, err error) {
	statusCmd, ok := AuthStatusCmds[command]
	if !ok {
		return "", "", nil
	}

	parts := strings.Fields(statusCmd)
	c := exec.Command(parts[0], parts[1:]...)
	if len(env) > 0 {
		c.Env = append(os.Environ(), env...)
	}
	out, err := c.Output()
	if err != nil {
		return "", "", err
	}
	return ParseAuthStatus(out)
}

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
	AuthUser   string   `yaml:"authUser,omitempty"`
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
