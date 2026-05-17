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
	input := []byte(`{"loggedIn":true,"authMethod":"claude.ai","email":"dev@example.com","orgName":"ExampleCorp","subscriptionType":"team"}`)
	email, org, err := ParseAuthStatus(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if email != "dev@example.com" {
		t.Errorf("expected email 'dev@example.com', got %q", email)
	}
	if org != "ExampleCorp" {
		t.Errorf("expected org 'ExampleCorp', got %q", org)
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

func TestDefaultAccounts_ClaudeHasMaxEffort(t *testing.T) {
	for _, id := range []string{"claude", "ama-claude"} {
		a := AccountByID(DefaultAccounts, id)
		if a == nil {
			t.Fatalf("expected DefaultAccounts to contain %q", id)
		}
		found := false
		for i, arg := range a.Args {
			if arg == "--effort" && i+1 < len(a.Args) && a.Args[i+1] == "max" {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected %q to have '--effort max' in Args, got %v", id, a.Args)
		}
	}
}

func TestResolvedArgs_AppendsEffortForClaudeCommand(t *testing.T) {
	a := Account{Command: "claude", Args: []string{"--dangerously-skip-permissions"}}
	got := a.ResolvedArgs()
	want := []string{"--dangerously-skip-permissions", "--effort", "max"}
	if !stringSliceEqual(got, want) {
		t.Errorf("ResolvedArgs = %v, want %v", got, want)
	}
	// Original Args must be unchanged
	if len(a.Args) != 1 {
		t.Errorf("ResolvedArgs mutated receiver Args: %v", a.Args)
	}
}

func TestResolvedArgs_PreservesUserEffortOverride(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"space-separated", []string{"--dangerously-skip-permissions", "--effort", "high"}},
		{"equals-form", []string{"--effort=low", "--dangerously-skip-permissions"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := Account{Command: "claude", Args: tc.args}
			got := a.ResolvedArgs()
			if !stringSliceEqual(got, tc.args) {
				t.Errorf("ResolvedArgs = %v, want unchanged %v", got, tc.args)
			}
		})
	}
}

func TestResolvedArgs_IgnoresNonClaudeCommands(t *testing.T) {
	cases := []Account{
		{Command: "codex", Args: []string{"--dangerously-bypass-approvals-and-sandbox"}},
		{Command: "gemini", Args: []string{"--yolo"}},
		{Command: "agent", Args: []string{}},
	}
	for _, a := range cases {
		got := a.ResolvedArgs()
		if !stringSliceEqual(got, a.Args) {
			t.Errorf("ResolvedArgs for %q = %v, want unchanged %v", a.Command, got, a.Args)
		}
	}
}

func TestResolvedArgs_ReturnsCopyNotReference(t *testing.T) {
	a := Account{Command: "codex", Args: []string{"--yolo"}}
	got := a.ResolvedArgs()
	got[0] = "mutated"
	if a.Args[0] == "mutated" {
		t.Error("ResolvedArgs returned a slice that aliases receiver Args; caller mutation leaked back")
	}
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
