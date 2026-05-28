package config

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestKeysPath(t *testing.T) {
	path := KeysPath()
	if path == "" {
		t.Error("KeysPath() returned empty string")
	}
	if !containsPath(path, ".qs") {
		t.Errorf("KeysPath() should contain .qs, got %q", path)
	}
}

func TestLoadKeysMissingFile(t *testing.T) {
	keys, err := LoadKeys()
	if err != nil {
		t.Logf("LoadKeys returned error (may be expected if keys.yaml exists with bad data): %v", err)
	}
	if keys == nil {
		t.Fatal("expected non-nil map from LoadKeys")
	}
}

func TestSaveAndLoadKeysRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.yaml")

	keys := AccountKeys{
		"opencode": {
			"PROVIDER_API_KEY": "sk-test-123",
			"OTHER_API_KEY":    "sk-test-456",
		},
		"claude": {
			"ANTHROPIC_API_KEY": "sk-test-789",
		},
	}

	// Marshal and write
	data, err := yaml.Marshal(keys)
	if err != nil {
		t.Fatalf("failed to marshal keys: %v", err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("failed to write keys file: %v", err)
	}

	// Verify file exists
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("failed to stat keys file: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("keys file is empty")
	}

	// Read back
	readData, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read keys file: %v", err)
	}

	var loaded AccountKeys
	if err := yaml.Unmarshal(readData, &loaded); err != nil {
		t.Fatalf("failed to unmarshal keys: %v", err)
	}

	// Verify opencode keys
	ocKeys := KeysForAccount(loaded, "opencode")
	if ocKeys == nil {
		t.Fatal("expected opencode keys")
	}
	if ocKeys["PROVIDER_API_KEY"] != "sk-test-123" {
		t.Errorf("expected PROVIDER_API_KEY 'sk-test-123', got %q", ocKeys["PROVIDER_API_KEY"])
	}
	if ocKeys["OTHER_API_KEY"] != "sk-test-456" {
		t.Errorf("expected OTHER_API_KEY 'sk-test-456', got %q", ocKeys["OTHER_API_KEY"])
	}

	// Verify claude keys
	clKeys := KeysForAccount(loaded, "claude")
	if clKeys == nil {
		t.Fatal("expected claude keys")
	}
	if clKeys["ANTHROPIC_API_KEY"] != "sk-test-789" {
		t.Errorf("expected ANTHROPIC_API_KEY 'sk-test-789', got %q", clKeys["ANTHROPIC_API_KEY"])
	}

	// Verify unknown account returns nil
	if KeysForAccount(loaded, "unknown") != nil {
		t.Error("expected nil for unknown account")
	}
}

func TestSetAndDeleteAccountKey(t *testing.T) {
	keys := make(AccountKeys)

	// Set a key
	SetAccountKey(keys, "opencode", "MY_KEY", "my-value")
	if keys["opencode"]["MY_KEY"] != "my-value" {
		t.Errorf("expected MY_KEY 'my-value', got %q", keys["opencode"]["MY_KEY"])
	}

	// Set another key for same account
	SetAccountKey(keys, "opencode", "OTHER_KEY", "other-value")
	if len(keys["opencode"]) != 2 {
		t.Errorf("expected 2 keys, got %d", len(keys["opencode"]))
	}

	// Delete first key
	DeleteAccountKey(keys, "opencode", "MY_KEY")
	if _, ok := keys["opencode"]["MY_KEY"]; ok {
		t.Error("expected MY_KEY to be deleted")
	}
	if len(keys["opencode"]) != 1 {
		t.Errorf("expected 1 key remaining, got %d", len(keys["opencode"]))
	}

	// Delete last key — should remove account entry entirely
	DeleteAccountKey(keys, "opencode", "OTHER_KEY")
	if _, ok := keys["opencode"]; ok {
		t.Error("expected opencode entry to be removed when empty")
	}

	// Delete from non-existent account — should not panic
	DeleteAccountKey(keys, "nonexistent", "KEY")
}

func TestMaskValue(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", ""},
		{"ab", "**"},
		{"abcd", "****"},
		{"sk-test-12345678", "sk-t********"},
		{"short", "shor********"},
	}

	for _, tt := range tests {
		got := MaskValue(tt.input)
		if got != tt.expected {
			t.Errorf("MaskValue(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestValidateEnvVarName(t *testing.T) {
	// Valid names
	validNames := []string{
		"MY_KEY",
		"PROVIDER_API_KEY",
		"key123",
		"A",
	}
	for _, name := range validNames {
		if err := ValidateEnvVarName(name); err != nil {
			t.Errorf("ValidateEnvVarName(%q) returned unexpected error: %v", name, err)
		}
	}

	// Invalid names
	invalidNames := []struct {
		name string
		desc string
	}{
		{"", "empty"},
		{"KEY=VALUE", "contains equals"},
		{"MY KEY", "contains space"},
		{"MY\tKEY", "contains tab"},
	}
	for _, tt := range invalidNames {
		if err := ValidateEnvVarName(tt.name); err == nil {
			t.Errorf("ValidateEnvVarName(%q) [%s] should have returned error", tt.name, tt.desc)
		}
	}
}

func TestKeysForAccountNilMap(t *testing.T) {
	if KeysForAccount(nil, "anything") != nil {
		t.Error("expected nil for nil keys map")
	}
}

func TestDefaultAccountKeysHasNoEffortLevel(t *testing.T) {
	defaults := DefaultAccountKeys()

	// claude must not inject any forced env vars (Claude Code owns effort now)
	if _, ok := defaults["claude"]; ok {
		t.Errorf("expected no 'claude' defaults, got %v", defaults["claude"])
	}

	// ama-claude keeps CLAUDE_CONFIG_DIR but no longer forces effort
	amaKeys, ok := defaults["ama-claude"]
	if !ok {
		t.Fatal("expected 'ama-claude' entry in DefaultAccountKeys")
	}
	if _, hasEffort := amaKeys["CLAUDE_CODE_EFFORT_LEVEL"]; hasEffort {
		t.Error("expected ama-claude to NOT have CLAUDE_CODE_EFFORT_LEVEL")
	}
	if amaKeys["CLAUDE_CONFIG_DIR"] == "" {
		t.Error("expected ama-claude to still have CLAUDE_CONFIG_DIR")
	}
}

func TestEnsureDefaultKeysAddsNoEffortLevel(t *testing.T) {
	keys := make(AccountKeys)
	EnsureDefaultKeys(keys)

	if _, hasEffort := keys["claude"]["CLAUDE_CODE_EFFORT_LEVEL"]; hasEffort {
		t.Error("expected claude to NOT get CLAUDE_CODE_EFFORT_LEVEL")
	}
	if _, hasEffort := keys["ama-claude"]["CLAUDE_CODE_EFFORT_LEVEL"]; hasEffort {
		t.Error("expected ama-claude to NOT get CLAUDE_CODE_EFFORT_LEVEL")
	}
	if keys["ama-claude"]["CLAUDE_CONFIG_DIR"] == "" {
		t.Error("expected ama-claude to get CLAUDE_CONFIG_DIR")
	}
}

func TestEnsureDefaultKeysPreservesUserEffortLevel(t *testing.T) {
	keys := AccountKeys{
		"claude": {
			"CLAUDE_CODE_EFFORT_LEVEL": "normal",
		},
	}
	EnsureDefaultKeys(keys)

	if keys["claude"]["CLAUDE_CODE_EFFORT_LEVEL"] != "normal" {
		t.Errorf("expected claude CLAUDE_CODE_EFFORT_LEVEL 'normal' (user-set), got %q", keys["claude"]["CLAUDE_CODE_EFFORT_LEVEL"])
	}
}

func TestUserAPIKeysExcludesDefaultEffortLevel(t *testing.T) {
	keys := make(AccountKeys)
	EnsureDefaultKeys(keys)

	// With only defaults, UserAPIKeys should return nil
	userKeys := UserAPIKeys(keys, "claude")
	if userKeys != nil {
		t.Errorf("expected nil UserAPIKeys for claude with only defaults, got %v", userKeys)
	}

	// Add a user API key
	SetAccountKey(keys, "claude", "ANTHROPIC_API_KEY", "sk-test-999")

	userKeys = UserAPIKeys(keys, "claude")
	if userKeys == nil {
		t.Fatal("expected non-nil UserAPIKeys after adding ANTHROPIC_API_KEY")
	}
	if userKeys["ANTHROPIC_API_KEY"] != "sk-test-999" {
		t.Errorf("expected ANTHROPIC_API_KEY 'sk-test-999', got %q", userKeys["ANTHROPIC_API_KEY"])
	}
	if _, hasEffort := userKeys["CLAUDE_CODE_EFFORT_LEVEL"]; hasEffort {
		t.Error("UserAPIKeys should not include default CLAUDE_CODE_EFFORT_LEVEL")
	}
}
