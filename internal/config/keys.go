package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// AccountKeys maps account ID → env var name → value
type AccountKeys map[string]map[string]string

// KeysPath returns the path to the keys file (~/.qs/keys.yaml)
func KeysPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".qs", "keys.yaml")
}

// LoadKeys reads the keys file. Returns an empty map if the file doesn't exist.
func LoadKeys() (AccountKeys, error) {
	data, err := os.ReadFile(KeysPath())
	if err != nil {
		if os.IsNotExist(err) {
			return make(AccountKeys), nil
		}
		return nil, fmt.Errorf("failed to read keys file: %w", err)
	}

	var keys AccountKeys
	if err := yaml.Unmarshal(data, &keys); err != nil {
		return nil, fmt.Errorf("failed to parse keys file: %w", err)
	}

	if keys == nil {
		keys = make(AccountKeys)
	}
	EnsureDefaultKeys(keys)
	return keys, nil
}

// SaveKeys writes the keys file with restrictive permissions (0600).
func SaveKeys(keys AccountKeys) error {
	path := KeysPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create keys directory: %w", err)
	}

	data, err := yaml.Marshal(keys)
	if err != nil {
		return fmt.Errorf("failed to marshal keys: %w", err)
	}

	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("failed to write keys file: %w", err)
	}

	return nil
}

// DefaultAccountKeys returns built-in env vars for default accounts.
// These are merged into loaded keys without overwriting user-set values.
func DefaultAccountKeys() AccountKeys {
	homeDir, _ := os.UserHomeDir()
	return AccountKeys{
		"ama-claude": {
			"CLAUDE_CONFIG_DIR": filepath.Join(homeDir, ".claude-ama"),
		},
	}
}

// EnsureDefaultKeys merges DefaultAccountKeys into the given keys map
// without overwriting any existing user-set values.
func EnsureDefaultKeys(keys AccountKeys) {
	for accountID, defaults := range DefaultAccountKeys() {
		for k, v := range defaults {
			if keys[accountID] == nil {
				keys[accountID] = make(map[string]string)
			}
			if _, exists := keys[accountID][k]; !exists {
				keys[accountID][k] = v
			}
		}
	}
}

// KeysForAccount returns the env var map for a specific account.
// Returns nil if the account has no keys.
func KeysForAccount(keys AccountKeys, accountID string) map[string]string {
	if keys == nil {
		return nil
	}
	return keys[accountID]
}

// UserAPIKeys returns only user-provided API keys for an account,
// excluding internal env vars from DefaultAccountKeys (like CLAUDE_CONFIG_DIR).
func UserAPIKeys(keys AccountKeys, accountID string) map[string]string {
	ak := KeysForAccount(keys, accountID)
	if len(ak) == 0 {
		return nil
	}
	defaults := DefaultAccountKeys()[accountID]
	result := make(map[string]string)
	for k, v := range ak {
		if _, isDefault := defaults[k]; !isDefault {
			result[k] = v
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// SetAccountKey sets a single key for an account.
func SetAccountKey(keys AccountKeys, accountID, name, value string) {
	if keys[accountID] == nil {
		keys[accountID] = make(map[string]string)
	}
	keys[accountID][name] = value
}

// DeleteAccountKey removes a single key from an account.
func DeleteAccountKey(keys AccountKeys, accountID, name string) {
	if keys[accountID] == nil {
		return
	}
	delete(keys[accountID], name)
	if len(keys[accountID]) == 0 {
		delete(keys, accountID)
	}
}

// MaskValue returns a masked display string for a secret value.
// Shows the first 4 characters followed by asterisks, or just asterisks if too short.
func MaskValue(val string) string {
	if len(val) <= 4 {
		return strings.Repeat("*", len(val))
	}
	return val[:4] + strings.Repeat("*", 8)
}

// ValidateEnvVarName checks that a string is a valid environment variable name.
func ValidateEnvVarName(name string) error {
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if strings.Contains(name, "=") {
		return fmt.Errorf("name cannot contain '='")
	}
	for _, r := range name {
		if unicode.IsSpace(r) {
			return fmt.Errorf("name cannot contain whitespace")
		}
	}
	return nil
}
