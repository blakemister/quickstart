package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthStatusCmds_HasClaude(t *testing.T) {
	cmd, ok := AuthStatusCmds["claude"]
	if !ok {
		t.Fatal("expected AuthStatusCmds to have entry for 'claude'")
	}
	if cmd != "claude auth status" {
		t.Errorf("expected 'claude auth status', got %q", cmd)
	}
}

func TestParseAuthStatus_Valid(t *testing.T) {
	input := []byte(`{"loggedIn":true,"authMethod":"claude.ai","email":"tech@eckmedia.com","orgName":"EckMedia","subscriptionType":"team"}`)
	email, org, err := ParseAuthStatus(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if email != "tech@eckmedia.com" {
		t.Errorf("expected email 'tech@eckmedia.com', got %q", email)
	}
	if org != "EckMedia" {
		t.Errorf("expected org 'EckMedia', got %q", org)
	}
}

func TestParseAuthStatus_Minimal(t *testing.T) {
	input := []byte(`{"email":"a@b.com"}`)
	email, org, err := ParseAuthStatus(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if email != "a@b.com" {
		t.Errorf("expected 'a@b.com', got %q", email)
	}
	if org != "" {
		t.Errorf("expected empty org, got %q", org)
	}
}

func TestParseAuthStatus_InvalidJSON(t *testing.T) {
	_, _, err := ParseAuthStatus([]byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestAccountAuthUser_OmitEmpty(t *testing.T) {
	a := Account{ID: "test", Label: "Test", Command: "test", Enabled: true}
	if a.AuthUser != "" {
		t.Errorf("expected empty AuthUser, got %q", a.AuthUser)
	}
}

func TestUniqueAccountID_NoCollision(t *testing.T) {
	existing := []Account{
		{ID: "claude"},
		{ID: "codex"},
	}
	got := UniqueAccountID("My Tool", existing)
	if got != "my-tool" {
		t.Errorf("expected 'my-tool', got %q", got)
	}
}

func TestUniqueAccountID_Collision(t *testing.T) {
	existing := []Account{
		{ID: "claude"},
		{ID: "claude-2"},
	}
	got := UniqueAccountID("Claude", existing)
	if got != "claude-3" {
		t.Errorf("expected 'claude-3', got %q", got)
	}
}

func TestUniqueAccountID_EmptyLabel(t *testing.T) {
	got := UniqueAccountID("", nil)
	if got != "account" {
		t.Errorf("expected 'account', got %q", got)
	}
}

func TestUniqueAccountID_SpecialChars(t *testing.T) {
	got := UniqueAccountID("My Tool (Work)", nil)
	if got != "my-tool-work" {
		t.Errorf("expected 'my-tool-work', got %q", got)
	}
}

func TestCloneAccount(t *testing.T) {
	src := Account{
		ID:         "claude",
		Label:      "Claude Code",
		Command:    "claude",
		Args:       []string{"--dangerously-skip-permissions"},
		AuthCmd:    "claude /login",
		InstallCmd: "npm i -g @anthropic-ai/claude-code",
		Icon:       "\U0001F7E0",
		Enabled:    true,
	}
	existing := []Account{src}

	clone := CloneAccount(src, "Claude Code (Work)", existing)

	if clone.ID != "claude-code-work" {
		t.Errorf("expected ID 'claude-code-work', got %q", clone.ID)
	}
	if clone.Label != "Claude Code (Work)" {
		t.Errorf("expected label 'Claude Code (Work)', got %q", clone.Label)
	}
	if clone.Command != "claude" {
		t.Errorf("expected command 'claude', got %q", clone.Command)
	}
	if len(clone.Args) != 1 || clone.Args[0] != "--dangerously-skip-permissions" {
		t.Errorf("expected args to match source, got %v", clone.Args)
	}
	if clone.AuthCmd != "claude /login" {
		t.Errorf("expected AuthCmd to match source, got %q", clone.AuthCmd)
	}
	if clone.InstallCmd != "npm i -g @anthropic-ai/claude-code" {
		t.Errorf("expected InstallCmd to match source, got %q", clone.InstallCmd)
	}
	if !clone.Enabled {
		t.Error("expected clone to be enabled")
	}

	// Verify deep copy of args - modifying clone shouldn't affect source
	clone.Args[0] = "modified"
	if src.Args[0] == "modified" {
		t.Error("clone args should be a deep copy, but modifying clone affected source")
	}
}

func TestCloneAccount_IDCollision(t *testing.T) {
	existing := []Account{
		{ID: "claude"},
		{ID: "claude-work"},
	}
	src := existing[0]
	clone := CloneAccount(src, "Claude Work", existing)

	if clone.ID != "claude-work-2" {
		t.Errorf("expected 'claude-work-2' on collision, got %q", clone.ID)
	}
}

func TestConfigDirEnvVars(t *testing.T) {
	envVar, ok := ConfigDirEnvVars["claude"]
	if !ok {
		t.Fatal("expected ConfigDirEnvVars to have entry for 'claude'")
	}
	if envVar != "CLAUDE_CONFIG_DIR" {
		t.Errorf("expected CLAUDE_CONFIG_DIR, got %q", envVar)
	}
}

func TestAccountConfigDir(t *testing.T) {
	dir := AccountConfigDir("claude-work")
	if dir == "" {
		t.Fatal("expected non-empty path")
	}
	if !filepath.IsAbs(dir) {
		t.Errorf("expected absolute path, got %q", dir)
	}
	// Should contain .qs/auth/<id>
	if !strings.Contains(dir, ".qs") || !strings.Contains(dir, "auth") || !strings.Contains(dir, "claude-work") {
		t.Errorf("expected path to contain .qs/auth/claude-work, got %q", dir)
	}
}

func TestSuggestedEnvVars(t *testing.T) {
	// Known tools should have entries
	for _, cmd := range []string{"claude", "codex", "gemini", "agent", "opencode"} {
		vars, ok := SuggestedEnvVars[cmd]
		if !ok {
			t.Errorf("expected SuggestedEnvVars to have entry for %q", cmd)
		}
		if len(vars) == 0 {
			t.Errorf("expected non-empty env vars for %q", cmd)
		}
	}

	// Claude should suggest ANTHROPIC_API_KEY
	if SuggestedEnvVars["claude"][0] != "ANTHROPIC_API_KEY" {
		t.Errorf("expected claude to suggest ANTHROPIC_API_KEY, got %q", SuggestedEnvVars["claude"][0])
	}
}
