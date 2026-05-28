package config

import (
	"sort"
	"strings"
	"testing"
)

// envMap parses a []string of KEY=VALUE into a map keyed by upper-cased name.
func envMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, kv := range env {
		eq := strings.IndexByte(kv, '=')
		if eq <= 0 {
			continue
		}
		m[strings.ToUpper(kv[:eq])] = kv[eq+1:]
	}
	return m
}

// TestResolveSessionEnvProfileIsolation is the headline security property: a
// session resolved for an AMA-bound project must NOT contain any secret from a
// different ("backlit") profile, and must NOT contain a secret leaked into the
// launching shell's environment.
func TestResolveSessionEnvProfileIsolation(t *testing.T) {
	// A secret in the launching shell that must never reach the tool.
	t.Setenv("LEAKED_SECRET", "should-not-appear")

	cfg := &Config{
		Version: CurrentVersion,
		Accounts: []Account{
			{ID: "claude", Command: "claude", Args: []string{"--dangerously-skip-permissions"}, Enabled: true},
		},
		Profiles: []Profile{
			{ID: "backlit", Label: "Backlit"},
			{ID: "ama", Label: "AMA", GitIdentity: GitIdentity{Name: "AMA Dev", Email: "dev@ama.example"}},
		},
		Projects: []Project{
			{ID: "ama-proj", Label: "AMA Project", Path: "C:/dev/ama", Profile: "ama", Accounts: []string{"claude"}},
		},
	}

	keys := AccountKeys{
		"profile:backlit": {
			"BACKLIT_API_KEY": "backlit-secret-value",
			"GH_TOKEN":        "ghp_backlit_token",
		},
		"profile:ama": {
			"AMA_API_KEY": "ama-secret-value",
		},
	}

	env, spec, err := ResolveSessionEnv(cfg, keys, "ama-proj", "claude")
	if err != nil {
		t.Fatalf("ResolveSessionEnv failed: %v", err)
	}

	m := envMap(env)

	// No backlit-namespaced keys or values may appear.
	for _, name := range []string{"BACKLIT_API_KEY", "GH_TOKEN"} {
		if _, ok := m[name]; ok {
			t.Errorf("SECURITY: backlit key %q leaked into AMA session env", name)
		}
	}
	for _, kv := range env {
		if strings.Contains(kv, "backlit-secret-value") || strings.Contains(kv, "ghp_backlit_token") {
			t.Errorf("SECURITY: backlit secret value leaked into AMA session env: %q", kv)
		}
	}

	// The shell-leaked secret must not appear.
	if _, ok := m["LEAKED_SECRET"]; ok {
		t.Error("SECURITY: LEAKED_SECRET from host shell leaked into session env")
	}
	for _, kv := range env {
		if strings.Contains(kv, "should-not-appear") {
			t.Errorf("SECURITY: host-leaked value reached session env: %q", kv)
		}
	}

	// The AMA profile's own secret SHOULD be present.
	if m["AMA_API_KEY"] != "ama-secret-value" {
		t.Errorf("expected AMA_API_KEY present, got %q", m["AMA_API_KEY"])
	}
	// Git identity from AMA profile should be present.
	if m["GIT_AUTHOR_EMAIL"] != "dev@ama.example" || m["GIT_COMMITTER_EMAIL"] != "dev@ama.example" {
		t.Errorf("expected AMA git email, got author=%q committer=%q", m["GIT_AUTHOR_EMAIL"], m["GIT_COMMITTER_EMAIL"])
	}

	// Launch spec sanity.
	if spec.Command != "claude" {
		t.Errorf("expected command 'claude', got %q", spec.Command)
	}
	if spec.Dir != "C:/dev/ama" {
		t.Errorf("expected dir 'C:/dev/ama', got %q", spec.Dir)
	}
}

// TestResolveSessionEnvNeverIncludesFullHostEnv verifies a non-allowlisted,
// non-secret host var is also dropped (the allowlist is exhaustive).
func TestResolveSessionEnvNeverIncludesFullHostEnv(t *testing.T) {
	t.Setenv("SOME_RANDOM_HOST_VAR", "random-value")

	cfg := &Config{
		Accounts: []Account{{ID: "claude", Command: "claude", Enabled: true}},
	}
	env, _, err := ResolveSessionEnv(cfg, nil, "", "claude")
	if err != nil {
		t.Fatalf("ResolveSessionEnv failed: %v", err)
	}
	if _, ok := envMap(env)["SOME_RANDOM_HOST_VAR"]; ok {
		t.Error("non-allowlisted host var leaked into clean base env")
	}
}

// TestResolveSessionEnvLayerPrecedence verifies the documented low->high
// override order across account keys, profile secrets, config dirs, git identity.
func TestResolveSessionEnvLayerPrecedence(t *testing.T) {
	cfg := &Config{
		Accounts: []Account{{ID: "claude", Command: "claude", Enabled: true}},
		Profiles: []Profile{
			{
				ID:                "work",
				GitIdentity:       GitIdentity{Name: "Worker", Email: "w@x.io"},
				AccountConfigDirs: map[string]string{"claude": "C:/work/.claude"},
			},
		},
		Projects: []Project{
			{ID: "p", Path: "C:/dev/p", Profile: "work", Accounts: []string{"claude"}},
		},
	}
	keys := AccountKeys{
		// Account key sets CLAUDE_CONFIG_DIR low; profile config dir must win.
		"claude":       {"CLAUDE_CONFIG_DIR": "C:/account/.claude", "SHARED": "from-account"},
		"profile:work": {"SHARED": "from-profile", "CLAUDE_CONFIG_DIR": "C:/profile-secret/.claude"},
	}

	env, _, err := ResolveSessionEnv(cfg, keys, "p", "claude")
	if err != nil {
		t.Fatalf("ResolveSessionEnv failed: %v", err)
	}
	m := envMap(env)

	// Profile secret beats account key.
	if m["SHARED"] != "from-profile" {
		t.Errorf("expected profile to override account for SHARED, got %q", m["SHARED"])
	}
	// AccountConfigDirs beats profile secret which beats account key.
	if m["CLAUDE_CONFIG_DIR"] != "C:/work/.claude" {
		t.Errorf("expected AccountConfigDirs to win CLAUDE_CONFIG_DIR, got %q", m["CLAUDE_CONFIG_DIR"])
	}
}

// TestResolveSessionEnvCaseInsensitiveDedup verifies keys differing only in
// case collapse to a single canonical entry.
func TestResolveSessionEnvCaseInsensitiveDedup(t *testing.T) {
	cfg := &Config{
		Accounts: []Account{{ID: "claude", Command: "claude", Enabled: true}},
		Profiles: []Profile{{ID: "work"}},
		Projects: []Project{{ID: "p", Path: "C:/dev/p", Profile: "work"}},
	}
	keys := AccountKeys{
		"claude":       {"my_var": "lower"},
		"profile:work": {"MY_VAR": "upper"},
	}

	env, _, err := ResolveSessionEnv(cfg, keys, "p", "claude")
	if err != nil {
		t.Fatalf("ResolveSessionEnv failed: %v", err)
	}

	count := 0
	for _, kv := range env {
		if strings.HasPrefix(strings.ToUpper(kv), "MY_VAR=") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly one MY_VAR entry, got %d (env: %v)", count, env)
	}
	if envMap(env)["MY_VAR"] != "upper" {
		t.Errorf("expected case-insensitive override to 'upper', got %q", envMap(env)["MY_VAR"])
	}
}

// TestResolveSessionEnvSorted verifies output is sorted.
func TestResolveSessionEnvSorted(t *testing.T) {
	cfg := &Config{Accounts: []Account{{ID: "claude", Command: "claude", Enabled: true}}}
	env, _, err := ResolveSessionEnv(cfg, nil, "", "claude")
	if err != nil {
		t.Fatalf("ResolveSessionEnv failed: %v", err)
	}
	if !sort.StringsAreSorted(env) {
		t.Errorf("expected sorted env, got %v", env)
	}
}

// TestResolveSessionEnvUnknownAccount errors rather than silently passing.
func TestResolveSessionEnvUnknownAccount(t *testing.T) {
	cfg := &Config{Accounts: []Account{{ID: "claude", Command: "claude", Enabled: true}}}
	if _, _, err := ResolveSessionEnv(cfg, nil, "", "ghost"); err == nil {
		t.Error("expected error for unknown account")
	}
}

// TestResolveProfileEnvNoCommand verifies the probing env has profile layers but
// no account command, and stays isolated from other profiles.
func TestResolveProfileEnvNoCommand(t *testing.T) {
	t.Setenv("LEAKED_SECRET", "should-not-appear")

	cfg := &Config{
		Profiles: []Profile{
			{ID: "backlit"},
			{ID: "ama", GitIdentity: GitIdentity{Email: "dev@ama.example"}},
		},
		Projects: []Project{{ID: "ama-proj", Path: "C:/dev/ama", Profile: "ama"}},
	}
	keys := AccountKeys{
		"profile:backlit": {"BACKLIT_API_KEY": "backlit-secret-value"},
		"profile:ama":     {"AMA_API_KEY": "ama-secret-value"},
	}

	env, err := ResolveProfileEnv(cfg, keys, "ama-proj")
	if err != nil {
		t.Fatalf("ResolveProfileEnv failed: %v", err)
	}
	m := envMap(env)

	if m["AMA_API_KEY"] != "ama-secret-value" {
		t.Errorf("expected AMA_API_KEY present, got %q", m["AMA_API_KEY"])
	}
	if _, ok := m["BACKLIT_API_KEY"]; ok {
		t.Error("SECURITY: backlit key leaked into AMA profile env")
	}
	if _, ok := m["LEAKED_SECRET"]; ok {
		t.Error("SECURITY: host-leaked secret reached profile env")
	}
}
