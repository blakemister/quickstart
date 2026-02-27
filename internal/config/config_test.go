package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultAccounts(t *testing.T) {
	if len(DefaultAccounts) != 5 {
		t.Fatalf("expected 5 default accounts, got %d", len(DefaultAccounts))
	}

	// Check first account is Claude
	if DefaultAccounts[0].ID != "claude" {
		t.Errorf("expected first account ID 'claude', got %q", DefaultAccounts[0].ID)
	}
	if DefaultAccounts[0].Command != "claude" {
		t.Errorf("expected first account command 'claude', got %q", DefaultAccounts[0].Command)
	}
	if !DefaultAccounts[0].Enabled {
		t.Error("expected first account to be enabled")
	}

	// Check that cursor exists and is enabled
	cursor := AccountByID(DefaultAccounts, "cursor")
	if cursor == nil {
		t.Fatal("expected to find cursor account")
	}
	if !cursor.Enabled {
		t.Error("expected cursor to be enabled by default")
	}
}

func TestNewDefaultConfig(t *testing.T) {
	cfg := NewDefaultConfig("C:/dev")

	if cfg.Version != 4 {
		t.Fatalf("expected version 4, got %d", cfg.Version)
	}
	if cfg.ProjectsRoot != "C:/dev" {
		t.Fatalf("expected projects root C:/dev, got %q", cfg.ProjectsRoot)
	}
	if cfg.DefaultAccount == "" {
		t.Fatal("expected default account to be set")
	}
	if cfg.LastAccount == "" {
		t.Fatal("expected last account to be set")
	}
	if len(cfg.Accounts) != len(DefaultAccounts) {
		t.Fatalf("expected %d accounts, got %d", len(DefaultAccounts), len(cfg.Accounts))
	}
	if len(cfg.Monitors) != 1 {
		t.Fatalf("expected 1 monitor, got %d", len(cfg.Monitors))
	}
	if cfg.Monitors[0].WindowCount() != 1 {
		t.Fatalf("expected monitor to have 1 window, got %d", cfg.Monitors[0].WindowCount())
	}
}

func TestEnsureDefaults(t *testing.T) {
	cfg := &Config{}
	EnsureDefaults(cfg)

	if cfg.Version != 4 {
		t.Fatalf("expected version 4, got %d", cfg.Version)
	}
	if cfg.DefaultAccount == "" {
		t.Fatal("expected default account to be populated")
	}
	if cfg.LastAccount == "" {
		t.Fatal("expected last account to be populated")
	}
	if len(cfg.Accounts) == 0 {
		t.Fatal("expected accounts to be populated")
	}
	if len(cfg.Monitors) == 0 {
		t.Fatal("expected monitors to be populated")
	}
	if cfg.Monitors[0].WindowCount() == 0 {
		t.Fatal("expected at least one monitor window")
	}
}

func TestAccountByID(t *testing.T) {
	accounts := []Account{
		{ID: "claude", Label: "Claude Code"},
		{ID: "codex", Label: "OpenAI Codex"},
	}

	// Found
	a := AccountByID(accounts, "claude")
	if a == nil {
		t.Fatal("expected to find claude")
	}
	if a.Label != "Claude Code" {
		t.Errorf("expected label 'Claude Code', got %q", a.Label)
	}

	// Not found
	if AccountByID(accounts, "unknown") != nil {
		t.Error("expected nil for unknown account")
	}
}

func TestEnabledAccounts(t *testing.T) {
	accounts := []Account{
		{ID: "a", Enabled: true},
		{ID: "b", Enabled: false},
		{ID: "c", Enabled: true},
		{ID: "d", Enabled: false},
	}

	enabled := EnabledAccounts(accounts)
	if len(enabled) != 2 {
		t.Fatalf("expected 2 enabled accounts, got %d", len(enabled))
	}
	if enabled[0].ID != "a" || enabled[1].ID != "c" {
		t.Errorf("expected enabled accounts [a, c], got [%s, %s]", enabled[0].ID, enabled[1].ID)
	}
}

func TestMigrateV3toV4(t *testing.T) {
	v3yaml := []byte(`version: 3
projectsRoot: /home/test/.1dev
monitors:
  - layout: vertical
    windows:
      - tool: cc
      - tool: cx
  - layout: full
    windows:
      - tool: cc
`)

	cfg, err := migrateV3(v3yaml)
	if err != nil {
		t.Fatalf("migrateV3 failed: %v", err)
	}

	if cfg.Version != 4 {
		t.Errorf("expected version 4, got %d", cfg.Version)
	}

	if cfg.ProjectsRoot != "/home/test/.1dev" {
		t.Errorf("expected projectsRoot /home/test/.1dev, got %s", cfg.ProjectsRoot)
	}

	if cfg.DefaultAccount != "claude" {
		t.Errorf("expected defaultAccount 'claude', got %q", cfg.DefaultAccount)
	}

	if len(cfg.Monitors) != 2 {
		t.Fatalf("expected 2 monitors, got %d", len(cfg.Monitors))
	}

	// Check tool mapping: cc -> claude, cx -> codex
	if cfg.Monitors[0].Windows[0].Tool != "claude" {
		t.Errorf("expected window 0 tool 'claude', got %q", cfg.Monitors[0].Windows[0].Tool)
	}
	if cfg.Monitors[0].Windows[1].Tool != "codex" {
		t.Errorf("expected window 1 tool 'codex', got %q", cfg.Monitors[0].Windows[1].Tool)
	}

	// Check accounts are seeded
	if len(cfg.Accounts) != len(DefaultAccounts) {
		t.Errorf("expected %d accounts, got %d", len(DefaultAccounts), len(cfg.Accounts))
	}
}

func TestMigrateV2toV4(t *testing.T) {
	v2yaml := []byte(`version: 2
projectsRoot: /home/test/.1dev
monitors:
  - windows: 2
    layout: vertical
  - windows: 1
    layout: full
`)

	cfg, err := migrateV2(v2yaml)
	if err != nil {
		t.Fatalf("migrateV2 failed: %v", err)
	}

	if cfg.Version != 4 {
		t.Errorf("expected version 4, got %d", cfg.Version)
	}

	if len(cfg.Monitors) != 2 {
		t.Fatalf("expected 2 monitors, got %d", len(cfg.Monitors))
	}

	if cfg.Monitors[0].WindowCount() != 2 {
		t.Errorf("monitor 0: expected 2 windows, got %d", cfg.Monitors[0].WindowCount())
	}

	// V2 windows all map to "claude"
	for j := 0; j < 2; j++ {
		if cfg.Monitors[0].Windows[j].Tool != "claude" {
			t.Errorf("monitor 0, window %d: expected tool 'claude', got %q", j, cfg.Monitors[0].Windows[j].Tool)
		}
	}

	if cfg.Monitors[1].WindowCount() != 1 {
		t.Errorf("monitor 1: expected 1 window, got %d", cfg.Monitors[1].WindowCount())
	}
}

func TestLoadV4Direct(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	v4yaml := []byte(`version: 4
projectsRoot: /test
defaultAccount: claude
lastAccount: codex
accounts:
  - id: claude
    label: Claude Code
    command: claude
    args: ["--dangerously-skip-permissions"]
    icon: "C"
    enabled: true
  - id: codex
    label: OpenAI Codex
    command: codex
    args: ["--full-auto"]
    icon: "X"
    enabled: true
monitors:
  - layout: vertical
    windows:
      - tool: claude
      - tool: codex
`)

	if err := os.WriteFile(path, v4yaml, 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if cfg.Version != 4 {
		t.Errorf("expected version 4, got %d", cfg.Version)
	}
	if cfg.DefaultAccount != "claude" {
		t.Errorf("expected defaultAccount 'claude', got %q", cfg.DefaultAccount)
	}
	if cfg.LastAccount != "codex" {
		t.Errorf("expected lastAccount 'codex', got %q", cfg.LastAccount)
	}
	if len(cfg.Accounts) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(cfg.Accounts))
	}
	if cfg.Monitors[0].ToolFor(0) != "claude" {
		t.Errorf("expected window 0 tool 'claude', got %q", cfg.Monitors[0].ToolFor(0))
	}
	if cfg.Monitors[0].ToolFor(1) != "codex" {
		t.Errorf("expected window 1 tool 'codex', got %q", cfg.Monitors[0].ToolFor(1))
	}
}

func TestSaveAndLoadV4(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := &Config{
		Version:        4,
		ProjectsRoot:   "/test/projects",
		DefaultAccount: "claude",
		LastAccount:    "codex",
		Accounts: []Account{
			{ID: "claude", Label: "Claude", Command: "claude", Args: []string{"--skip"}, Enabled: true},
			{ID: "codex", Label: "Codex", Command: "codex", Args: []string{"--auto"}, Enabled: true},
		},
		Monitors: []MonitorConfig{
			{
				Layout: "vertical",
				Windows: []WindowConfig{
					{Tool: "codex"},
					{Tool: "claude"},
				},
			},
		},
	}

	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.Version != 4 {
		t.Errorf("expected version 4, got %d", loaded.Version)
	}
	if loaded.DefaultAccount != "claude" {
		t.Errorf("expected defaultAccount 'claude', got %q", loaded.DefaultAccount)
	}
	if loaded.LastAccount != "codex" {
		t.Errorf("expected lastAccount 'codex', got %q", loaded.LastAccount)
	}
	if loaded.Monitors[0].ToolFor(0) != "codex" {
		t.Errorf("expected window 0 tool 'codex', got %q", loaded.Monitors[0].ToolFor(0))
	}
	if loaded.Monitors[0].ToolFor(1) != "claude" {
		t.Errorf("expected window 1 tool 'claude', got %q", loaded.Monitors[0].ToolFor(1))
	}
}

func TestLoadFallbackToLegacy(t *testing.T) {
	// Create a temp directory structure simulating home
	dir := t.TempDir()
	legacyDir := filepath.Join(dir, ".cc")
	os.MkdirAll(legacyDir, 0755)

	legacyPath := filepath.Join(legacyDir, "config.yaml")
	v3yaml := []byte(`version: 3
projectsRoot: /legacy
monitors:
  - layout: full
    windows:
      - tool: cc
`)

	if err := os.WriteFile(legacyPath, v3yaml, 0644); err != nil {
		t.Fatalf("failed to write legacy config: %v", err)
	}

	// Load with explicit legacy path (since we can't mock home dir easily)
	cfg, err := Load(legacyPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Should have been migrated to v4
	if cfg.Version != 4 {
		t.Errorf("expected version 4 after migration, got %d", cfg.Version)
	}

	// cc -> claude mapping
	if cfg.Monitors[0].Windows[0].Tool != "claude" {
		t.Errorf("expected tool 'claude' after migration, got %q", cfg.Monitors[0].Windows[0].Tool)
	}
}

func TestWindowConfigToolMapping(t *testing.T) {
	// Test the v3 tool mapping function
	tests := []struct {
		input    string
		expected string
	}{
		{"cc", "claude"},
		{"cx", "codex"},
		{"claude", "claude"},
		{"codex", "codex"},
		{"", "claude"},
	}

	for _, tt := range tests {
		got := mapV3Tool(tt.input)
		if got != tt.expected {
			t.Errorf("mapV3Tool(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestDefaultConfigPaths(t *testing.T) {
	// Just verify they don't panic and return non-empty strings
	path := DefaultConfigPath()
	if path == "" {
		t.Error("DefaultConfigPath() returned empty string")
	}

	legacy := LegacyConfigPath()
	if legacy == "" {
		t.Error("LegacyConfigPath() returned empty string")
	}

	root := DefaultProjectsRoot()
	if root == "" {
		t.Error("DefaultProjectsRoot() returned empty string")
	}

	// Verify paths contain expected directory names
	if !containsPath(path, ".qs") {
		t.Errorf("DefaultConfigPath() should contain .qs, got %q", path)
	}
	if !containsPath(legacy, ".cc") {
		t.Errorf("LegacyConfigPath() should contain .cc, got %q", legacy)
	}
}

func TestAccountAuthCommand(t *testing.T) {
	tests := []struct {
		authCmd     string
		wantCmd     string
		wantArgs    []string
		wantHasAuth bool
	}{
		{"claude /login", "claude", []string{"/login"}, true},
		{"codex login", "codex", []string{"login"}, true},
		{"opencode auth login", "opencode", []string{"auth", "login"}, true},
		{"gemini", "gemini", nil, true},
		{"", "", nil, false},
		{"  ", "", nil, false},
	}

	for _, tt := range tests {
		a := Account{AuthCmd: tt.authCmd}

		gotCmd, gotArgs := a.AuthCommand()
		if gotCmd != tt.wantCmd {
			t.Errorf("AuthCommand(%q) cmd = %q, want %q", tt.authCmd, gotCmd, tt.wantCmd)
		}
		if len(gotArgs) == 0 && len(tt.wantArgs) == 0 {
			// both empty, ok
		} else if len(gotArgs) != len(tt.wantArgs) {
			t.Errorf("AuthCommand(%q) args len = %d, want %d", tt.authCmd, len(gotArgs), len(tt.wantArgs))
		} else {
			for i := range gotArgs {
				if gotArgs[i] != tt.wantArgs[i] {
					t.Errorf("AuthCommand(%q) args[%d] = %q, want %q", tt.authCmd, i, gotArgs[i], tt.wantArgs[i])
				}
			}
		}

		if got := a.HasAuth(); got != tt.wantHasAuth {
			t.Errorf("HasAuth(%q) = %v, want %v", tt.authCmd, got, tt.wantHasAuth)
		}
	}
}

func TestAuthCmdRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := &Config{
		Version:        4,
		ProjectsRoot:   "/test",
		DefaultAccount: "claude",
		Accounts: []Account{
			{ID: "claude", Label: "Claude", Command: "claude", AuthCmd: "claude /login", Enabled: true},
			{ID: "aider", Label: "Aider", Command: "aider", Enabled: true},
		},
		Monitors: []MonitorConfig{
			{Layout: "full", Windows: []WindowConfig{{Tool: "claude"}}},
		},
	}

	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify omitempty: aider has no authCmd, so it shouldn't appear in YAML
	data, _ := os.ReadFile(path)
	yaml := string(data)
	// Claude's authCmd should be present
	if !contains(yaml, "authCmd") {
		t.Error("expected authCmd to appear in saved YAML for claude")
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify Claude's auth command survived round-trip
	claude := AccountByID(loaded.Accounts, "claude")
	if claude == nil {
		t.Fatal("expected to find claude account")
	}
	if claude.AuthCmd != "claude /login" {
		t.Errorf("expected AuthCmd 'claude /login', got %q", claude.AuthCmd)
	}
	if !claude.HasAuth() {
		t.Error("expected claude HasAuth() to be true")
	}

	// Verify Aider has no auth command
	aider := AccountByID(loaded.Accounts, "aider")
	if aider == nil {
		t.Fatal("expected to find aider account")
	}
	if aider.AuthCmd != "" {
		t.Errorf("expected empty AuthCmd for aider, got %q", aider.AuthCmd)
	}
	if aider.HasAuth() {
		t.Error("expected aider HasAuth() to be false")
	}
}

func TestEnsureAuthDefaults(t *testing.T) {
	cfg := &Config{
		Accounts: []Account{
			{ID: "claude", Label: "Claude", Command: "claude", Enabled: true},
			{ID: "custom", Label: "Custom", Command: "custom", Enabled: true},
		},
	}
	EnsureDefaults(cfg)

	// claude should get backfilled auth command
	claude := AccountByID(cfg.Accounts, "claude")
	if claude == nil {
		t.Fatal("expected claude account")
	}
	if claude.AuthCmd == "" {
		t.Error("expected claude AuthCmd to be backfilled")
	}

	// custom should remain empty (not a known default)
	custom := AccountByID(cfg.Accounts, "custom")
	if custom == nil {
		t.Fatal("expected custom account")
	}
	if custom.AuthCmd != "" {
		t.Errorf("expected custom AuthCmd to remain empty, got %q", custom.AuthCmd)
	}
}

func TestAccountInstallCommand(t *testing.T) {
	tests := []struct {
		installCmd     string
		wantCmd        string
		wantArgs       []string
		wantHasInstall bool
	}{
		{"npm i -g @anthropic-ai/claude-code", "npm", []string{"i", "-g", "@anthropic-ai/claude-code"}, true},
		{"pip install aider-chat", "pip", []string{"install", "aider-chat"}, true},
		{"npm i -g opencode", "npm", []string{"i", "-g", "opencode"}, true},
		{"", "", nil, false},
		{"  ", "", nil, false},
	}

	for _, tt := range tests {
		a := Account{InstallCmd: tt.installCmd}

		gotCmd, gotArgs := a.InstallCommand()
		if gotCmd != tt.wantCmd {
			t.Errorf("InstallCommand(%q) cmd = %q, want %q", tt.installCmd, gotCmd, tt.wantCmd)
		}
		if len(gotArgs) == 0 && len(tt.wantArgs) == 0 {
			// both empty, ok
		} else if len(gotArgs) != len(tt.wantArgs) {
			t.Errorf("InstallCommand(%q) args len = %d, want %d", tt.installCmd, len(gotArgs), len(tt.wantArgs))
		} else {
			for i := range gotArgs {
				if gotArgs[i] != tt.wantArgs[i] {
					t.Errorf("InstallCommand(%q) args[%d] = %q, want %q", tt.installCmd, i, gotArgs[i], tt.wantArgs[i])
				}
			}
		}

		if got := a.HasInstall(); got != tt.wantHasInstall {
			t.Errorf("HasInstall(%q) = %v, want %v", tt.installCmd, got, tt.wantHasInstall)
		}
	}
}

func TestInstallCmdRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := &Config{
		Version:        4,
		ProjectsRoot:   "/test",
		DefaultAccount: "claude",
		Accounts: []Account{
			{ID: "claude", Label: "Claude", Command: "claude", InstallCmd: "npm i -g @anthropic-ai/claude-code", Enabled: true},
			{ID: "aider", Label: "Aider", Command: "aider", Enabled: true},
		},
		Monitors: []MonitorConfig{
			{Layout: "full", Windows: []WindowConfig{{Tool: "claude"}}},
		},
	}

	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify omitempty: aider has no installCmd, so it shouldn't appear in YAML
	data, _ := os.ReadFile(path)
	yamlStr := string(data)
	if !contains(yamlStr, "installCmd") {
		t.Error("expected installCmd to appear in saved YAML for claude")
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Verify Claude's install command survived round-trip
	claude := AccountByID(loaded.Accounts, "claude")
	if claude == nil {
		t.Fatal("expected to find claude account")
	}
	if claude.InstallCmd != "npm i -g @anthropic-ai/claude-code" {
		t.Errorf("expected InstallCmd 'npm i -g @anthropic-ai/claude-code', got %q", claude.InstallCmd)
	}
	if !claude.HasInstall() {
		t.Error("expected claude HasInstall() to be true")
	}

	// Verify Aider has no install command
	aider := AccountByID(loaded.Accounts, "aider")
	if aider == nil {
		t.Fatal("expected to find aider account")
	}
	if aider.InstallCmd != "" {
		t.Errorf("expected empty InstallCmd for aider, got %q", aider.InstallCmd)
	}
	if aider.HasInstall() {
		t.Error("expected aider HasInstall() to be false")
	}
}

func TestEnsureInstallDefaults(t *testing.T) {
	cfg := &Config{
		Accounts: []Account{
			{ID: "claude", Label: "Claude", Command: "claude", Enabled: true},
			{ID: "gemini", Label: "Gemini", Command: "gemini", Enabled: true},
			{ID: "custom", Label: "Custom", Command: "custom", Enabled: true},
		},
	}
	EnsureDefaults(cfg)

	// claude should get backfilled install command
	claude := AccountByID(cfg.Accounts, "claude")
	if claude == nil {
		t.Fatal("expected claude account")
	}
	if claude.InstallCmd == "" {
		t.Error("expected claude InstallCmd to be backfilled")
	}
	if claude.InstallCmd != "npm i -g @anthropic-ai/claude-code" {
		t.Errorf("expected claude InstallCmd 'npm i -g @anthropic-ai/claude-code', got %q", claude.InstallCmd)
	}

	// gemini should get backfilled install command
	gemini := AccountByID(cfg.Accounts, "gemini")
	if gemini == nil {
		t.Fatal("expected gemini account")
	}
	if gemini.InstallCmd == "" {
		t.Error("expected gemini InstallCmd to be backfilled")
	}

	// custom should remain empty (not a known default)
	custom := AccountByID(cfg.Accounts, "custom")
	if custom == nil {
		t.Fatal("expected custom account")
	}
	if custom.InstallCmd != "" {
		t.Errorf("expected custom InstallCmd to remain empty, got %q", custom.InstallCmd)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsSubstr(s, substr)
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func containsPath(path, segment string) bool {
	for _, p := range filepath.SplitList(path) {
		if p == segment {
			return true
		}
	}
	// Also check with filepath split
	dir := path
	for dir != "." && dir != "/" && dir != "" {
		base := filepath.Base(dir)
		if base == segment {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return false
}
