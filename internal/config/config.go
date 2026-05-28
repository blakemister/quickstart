package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// WindowConfig represents configuration for a single window within a monitor
type WindowConfig struct {
	Tool string `yaml:"tool"`
}

// MonitorConfig represents configuration for a single monitor
type MonitorConfig struct {
	Layout  string         `yaml:"layout"`
	Windows []WindowConfig `yaml:"windows"`
}

// WindowCount returns the number of windows configured for this monitor
func (mc *MonitorConfig) WindowCount() int {
	return len(mc.Windows)
}

// ToolFor returns the tool name for the window at index idx, defaulting to "claude"
func (mc *MonitorConfig) ToolFor(idx int) string {
	if idx >= 0 && idx < len(mc.Windows) && mc.Windows[idx].Tool != "" {
		return mc.Windows[idx].Tool
	}
	return "claude"
}

// CurrentVersion is the config schema version written by Save. Older versions
// are migrated to this in memory on Load.
const CurrentVersion = 5

// Config represents the application configuration (v5)
type Config struct {
	Version        int             `yaml:"version"`
	ProjectsRoot   string          `yaml:"projectsRoot"`
	DefaultAccount string          `yaml:"defaultAccount,omitempty"`
	LastAccount    string          `yaml:"lastAccount,omitempty"`
	Accounts       []Account       `yaml:"accounts"`
	Monitors       []MonitorConfig `yaml:"monitors"`
	Profiles       []Profile       `yaml:"profiles,omitempty"`
	Services       []Service       `yaml:"services,omitempty"`
	Projects       []Project       `yaml:"projects,omitempty"`
}

// v2Config is the old format used for migration
type v2Config struct {
	Version      int               `yaml:"version"`
	ProjectsRoot string            `yaml:"projectsRoot"`
	Monitors     []v2MonitorConfig `yaml:"monitors"`
}

type v2MonitorConfig struct {
	Windows int    `yaml:"windows"`
	Layout  string `yaml:"layout"`
}

// v3Config is the intermediate format
type v3Config struct {
	Version      int             `yaml:"version"`
	ProjectsRoot string          `yaml:"projectsRoot"`
	Monitors     []MonitorConfig `yaml:"monitors"`
}

// DefaultConfigPath returns the default configuration file path (~/.qs/config.yaml)
func DefaultConfigPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".qs", "config.yaml")
}

// LegacyConfigPath returns the old configuration file path (~/.cc/config.yaml)
func LegacyConfigPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".cc", "config.yaml")
}

// DefaultProjectsRoot returns the default projects directory
func DefaultProjectsRoot() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".1dev")
}

// IsFirstRun returns true if no config file exists (neither new nor legacy)
func IsFirstRun() bool {
	return isFirstRunAt(DefaultConfigPath(), LegacyConfigPath())
}

// isFirstRunAt checks if either path exists (used by IsFirstRun, testable).
func isFirstRunAt(path, legacy string) bool {
	if _, err := os.Stat(path); err == nil {
		return false
	}
	if _, err := os.Stat(legacy); err == nil {
		return false
	}
	return true
}

// Load reads the configuration from a file, auto-migrating v2/v3 to v4 in memory.
// If path is empty, tries ~/.qs/config.yaml then falls back to ~/.cc/config.yaml with migration.
func Load(path string) (*Config, error) {
	if path == "" {
		// Try new path first
		path = DefaultConfigPath()
		if _, err := os.Stat(path); os.IsNotExist(err) {
			// Fall back to legacy path
			legacyPath := LegacyConfigPath()
			if _, err := os.Stat(legacyPath); err == nil {
				path = legacyPath
			} else {
				return nil, &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
			}
		}
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Peek at version to decide how to unmarshal
	var peek struct {
		Version int `yaml:"version"`
	}
	if err := yaml.Unmarshal(data, &peek); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	switch {
	case peek.Version < 3:
		return migrateV2(data)
	case peek.Version == 3:
		return migrateV3(data)
	case peek.Version == 4:
		return migrateV4(data)
	default:
		var cfg Config
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config: %w", err)
		}
		return &cfg, nil
	}
}

// migrateV4 converts a v4 config to v5. This is purely additive: a v4 file
// unmarshals cleanly into the v5 Config struct (the new Profiles/Services/
// Projects slices simply stay empty), so we only need to bump the version.
func migrateV4(data []byte) (*Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse v4 config: %w", err)
	}
	cfg.Version = CurrentVersion
	return &cfg, nil
}

// migrateV2 converts a v2 config to v5
func migrateV2(data []byte) (*Config, error) {
	var old v2Config
	if err := yaml.Unmarshal(data, &old); err != nil {
		return nil, fmt.Errorf("failed to parse v2 config: %w", err)
	}

	monitors := make([]MonitorConfig, len(old.Monitors))
	for i, om := range old.Monitors {
		count := om.Windows
		if count < 1 {
			count = 1
		}
		windows := make([]WindowConfig, count)
		for j := range windows {
			windows[j] = WindowConfig{Tool: "claude"}
		}
		monitors[i] = MonitorConfig{
			Layout:  om.Layout,
			Windows: windows,
		}
	}

	cfg := &Config{
		Version:        CurrentVersion,
		ProjectsRoot:   old.ProjectsRoot,
		DefaultAccount: "claude",
		LastAccount:    "claude",
		Accounts:       copyDefaultAccounts(),
		Monitors:       monitors,
	}

	return cfg, nil
}

// migrateV3 converts a v3 config to v5
func migrateV3(data []byte) (*Config, error) {
	var old v3Config
	if err := yaml.Unmarshal(data, &old); err != nil {
		return nil, fmt.Errorf("failed to parse v3 config: %w", err)
	}

	// Map old tool names: "cc" -> "claude", "cx" -> "codex"
	monitors := make([]MonitorConfig, len(old.Monitors))
	for i, om := range old.Monitors {
		windows := make([]WindowConfig, len(om.Windows))
		for j, w := range om.Windows {
			windows[j] = WindowConfig{Tool: mapV3Tool(w.Tool)}
		}
		monitors[i] = MonitorConfig{
			Layout:  om.Layout,
			Windows: windows,
		}
	}

	cfg := &Config{
		Version:        CurrentVersion,
		ProjectsRoot:   old.ProjectsRoot,
		DefaultAccount: "claude",
		LastAccount:    "claude",
		Accounts:       copyDefaultAccounts(),
		Monitors:       monitors,
	}

	return cfg, nil
}

// mapV3Tool converts v3 tool names to v4 account IDs
func mapV3Tool(tool string) string {
	switch tool {
	case "cc":
		return "claude"
	case "cx":
		return "codex"
	default:
		if tool == "" {
			return "claude"
		}
		return tool
	}
}

// copyDefaultAccounts returns a fresh copy of DefaultAccounts
func copyDefaultAccounts() []Account {
	accounts := make([]Account, len(DefaultAccounts))
	for i, a := range DefaultAccounts {
		accounts[i] = Account{
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
	return accounts
}

// mergeNewDefaults appends any DefaultAccounts whose ID is not already present in the config.
func mergeNewDefaults(cfg *Config) {
	existing := make(map[string]bool, len(cfg.Accounts))
	for _, a := range cfg.Accounts {
		existing[a.ID] = true
	}
	for _, d := range DefaultAccounts {
		if !existing[d.ID] {
			cfg.Accounts = append(cfg.Accounts, Account{
				ID:         d.ID,
				Label:      d.Label,
				Command:    d.Command,
				Args:       append([]string{}, d.Args...),
				AuthCmd:    d.AuthCmd,
				InstallCmd: d.InstallCmd,
				Icon:       d.Icon,
				Enabled:    d.Enabled,
			})
		}
	}
}

// NewDefaultConfig returns a config seeded with defaults and the given projects root.
func NewDefaultConfig(projectsRoot string) *Config {
	cfg := &Config{
		Version:        CurrentVersion,
		ProjectsRoot:   strings.TrimSpace(projectsRoot),
		DefaultAccount: "claude",
		LastAccount:    "claude",
		Accounts:       copyDefaultAccounts(),
		Monitors: []MonitorConfig{
			{
				Layout: "full",
				Windows: []WindowConfig{
					{Tool: "claude"},
				},
			},
		},
	}
	return cfg
}

// EnsureDefaults populates missing non-path configuration fields with safe defaults.
func EnsureDefaults(cfg *Config) {
	if cfg == nil {
		return
	}

	if cfg.Version == 0 {
		cfg.Version = CurrentVersion
	}
	if len(cfg.Accounts) == 0 {
		cfg.Accounts = copyDefaultAccounts()
	} else {
		mergeNewDefaults(cfg)
	}

	if cfg.DefaultAccount == "" {
		cfg.DefaultAccount = firstEnabledAccountID(cfg.Accounts)
		if cfg.DefaultAccount == "" {
			cfg.DefaultAccount = "claude"
		}
	}
	if cfg.LastAccount == "" {
		cfg.LastAccount = cfg.DefaultAccount
	}

	if len(cfg.Monitors) == 0 {
		cfg.Monitors = []MonitorConfig{
			{
				Layout: "full",
				Windows: []WindowConfig{
					{Tool: cfg.DefaultAccount},
				},
			},
		}
	}

	ensureAccountDefaults(cfg)
	ensureProfileDefaults(cfg)
	validateProjectRefs(cfg)
}

// DefaultProfileID is the ID of the always-present personal profile.
const DefaultProfileID = "personal"

// ensureProfileDefaults seeds a default "personal" profile when no profiles
// exist. The personal profile carries no secrets and no git override — it is a
// neutral baseline so that unbound projects resolve to a clean session.
//
// New optional fields are fill-only-if-empty: this never overwrites
// user-provided profile data (unlike ensureAccountDefaults, which intentionally
// force-syncs built-in account args). Guarding that distinction is critical so
// a user-set GitIdentity is not clobbered on every load.
func ensureProfileDefaults(cfg *Config) {
	if len(cfg.Profiles) == 0 {
		cfg.Profiles = []Profile{
			{
				ID:    DefaultProfileID,
				Label: "Personal",
			},
		}
	}
}

// validateProjectRefs drops dangling references on each project so a stale or
// hand-edited config never points a project at a profile/account/service that
// no longer exists. References are pruned silently (no panic); the project
// itself is retained even if all its references were dangling.
func validateProjectRefs(cfg *Config) {
	profileIDs := make(map[string]bool, len(cfg.Profiles))
	for _, p := range cfg.Profiles {
		profileIDs[p.ID] = true
	}
	accountIDs := make(map[string]bool, len(cfg.Accounts))
	for _, a := range cfg.Accounts {
		accountIDs[a.ID] = true
	}
	serviceIDs := make(map[string]bool, len(cfg.Services))
	for _, s := range cfg.Services {
		serviceIDs[s.ID] = true
	}

	for i := range cfg.Projects {
		p := &cfg.Projects[i]
		if p.Profile != "" && !profileIDs[p.Profile] {
			p.Profile = ""
		}
		p.Accounts = filterKnown(p.Accounts, accountIDs)
		if p.DefaultAccount != "" && !accountIDs[p.DefaultAccount] {
			p.DefaultAccount = ""
		}
		p.Services = filterKnown(p.Services, serviceIDs)
	}
}

// filterKnown returns the IDs present in known, preserving order. Returns nil
// when nothing survives so empty slices stay omitempty in YAML.
func filterKnown(ids []string, known map[string]bool) []string {
	var out []string
	for _, id := range ids {
		if known[id] {
			out = append(out, id)
		}
	}
	return out
}

// ensureAccountDefaults syncs args, auth, and install commands for built-in account IDs.
// Built-in accounts (those matching a DefaultAccounts ID) always get current defaults
// for Args, AuthCmd, and InstallCmd. Users who want custom args should clone the account.
func ensureAccountDefaults(cfg *Config) {
	defaults := make(map[string]Account, len(DefaultAccounts))
	for _, da := range DefaultAccounts {
		defaults[da.ID] = da
	}
	for i := range cfg.Accounts {
		da, ok := defaults[cfg.Accounts[i].ID]
		if !ok {
			continue
		}
		// Always sync args for built-in accounts so CLI flag changes are picked up
		cfg.Accounts[i].Args = append([]string{}, da.Args...)
		if cfg.Accounts[i].AuthCmd == "" {
			cfg.Accounts[i].AuthCmd = da.AuthCmd
		}
		if cfg.Accounts[i].InstallCmd == "" {
			cfg.Accounts[i].InstallCmd = da.InstallCmd
		}
	}
}

func firstEnabledAccountID(accounts []Account) string {
	for _, account := range accounts {
		if account.Enabled && account.ID != "" {
			return account.ID
		}
	}
	for _, account := range accounts {
		if account.ID != "" {
			return account.ID
		}
	}
	return ""
}

// Save writes the configuration to a file
func Save(cfg *Config, path string) error {
	if path == "" {
		path = DefaultConfigPath()
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	cfg.Version = CurrentVersion

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}
