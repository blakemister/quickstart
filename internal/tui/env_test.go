package tui

import (
	"testing"

	"github.com/bcmister/qs/internal/config"
)

func TestAccountEnvSlice(t *testing.T) {
	keys := config.AccountKeys{
		"claude": {"ANTHROPIC_API_KEY": "sk-test-123"},
		"codex":  {"OPENAI_API_KEY": "sk-openai-456"},
	}

	// Known account with keys
	env := accountEnvSlice(keys, "claude")
	if len(env) != 1 {
		t.Fatalf("expected 1 env var for claude, got %d", len(env))
	}
	if env[0] != "ANTHROPIC_API_KEY=sk-test-123" {
		t.Errorf("expected ANTHROPIC_API_KEY=sk-test-123, got %s", env[0])
	}

	// Account with no keys
	env = accountEnvSlice(keys, "gemini")
	if len(env) != 0 {
		t.Errorf("expected 0 env vars for gemini, got %d", len(env))
	}

	// Empty keys map
	env = accountEnvSlice(nil, "claude")
	if len(env) != 0 {
		t.Errorf("expected 0 env vars with nil keys, got %d", len(env))
	}
}
