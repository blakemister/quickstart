package session

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubEnsureSwarmShell replaces the engine's swarm-shell check with a no-op and
// returns a function that restores the original. Used by the psmux unit tests so
// Start does not touch the real filesystem or require Git Bash.
func stubEnsureSwarmShell() (restore func()) {
	prev := ensureSwarmShell
	ensureSwarmShell = func() error { return nil }
	return func() { ensureSwarmShell = prev }
}

// readConf reads a .psmux.conf for assertions.
func readConf(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read conf: %v", err)
	}
	return string(b)
}

func countDefaultShellLines(conf string) int {
	n := 0
	for _, line := range strings.Split(conf, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), psmuxDefaultShellKey) {
			n++
		}
	}
	return n
}

func TestEnsureSwarmShellCreatesConf(t *testing.T) {
	if !fileExists(gitBashDefaultPath) {
		t.Skip("Git Bash not installed at default path; skipping real swarm-shell test")
	}
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)

	if err := EnsureSwarmShell(); err != nil {
		t.Fatalf("EnsureSwarmShell: %v", err)
	}

	conf := readConf(t, filepath.Join(home, ".psmux.conf"))
	if !strings.Contains(conf, psmuxDefaultShellLine) {
		t.Fatalf("conf missing default-shell directive:\n%s", conf)
	}
	if countDefaultShellLines(conf) != 1 {
		t.Fatalf("expected exactly one default-shell line:\n%s", conf)
	}
}

func TestEnsureSwarmShellIsIdempotent(t *testing.T) {
	if !fileExists(gitBashDefaultPath) {
		t.Skip("Git Bash not installed at default path; skipping real swarm-shell test")
	}
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)

	for i := 0; i < 3; i++ {
		if err := EnsureSwarmShell(); err != nil {
			t.Fatalf("EnsureSwarmShell run %d: %v", i, err)
		}
	}

	conf := readConf(t, filepath.Join(home, ".psmux.conf"))
	if countDefaultShellLines(conf) != 1 {
		t.Fatalf("idempotency broken — multiple default-shell lines:\n%s", conf)
	}
}

func TestEnsureSwarmShellPreservesExistingConf(t *testing.T) {
	if !fileExists(gitBashDefaultPath) {
		t.Skip("Git Bash not installed at default path; skipping real swarm-shell test")
	}
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)

	confPath := filepath.Join(home, ".psmux.conf")
	existing := "set -g mouse on\nset -g history-limit 50000"
	if err := os.WriteFile(confPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("seed conf: %v", err)
	}

	if err := EnsureSwarmShell(); err != nil {
		t.Fatalf("EnsureSwarmShell: %v", err)
	}

	conf := readConf(t, confPath)
	if !strings.Contains(conf, "set -g mouse on") || !strings.Contains(conf, "set -g history-limit 50000") {
		t.Fatalf("EnsureSwarmShell clobbered existing user config:\n%s", conf)
	}
	if !strings.Contains(conf, psmuxDefaultShellLine) {
		t.Fatalf("conf missing default-shell directive:\n%s", conf)
	}
	// The seeded file had no trailing newline; the appended block must not have
	// joined the last existing line.
	if strings.Contains(conf, "50000set") || strings.Contains(conf, "50000#") {
		t.Fatalf("appended block joined the last existing line:\n%s", conf)
	}
}

func TestEnsureSwarmShellDoesNotDuplicateExistingDirective(t *testing.T) {
	if !fileExists(gitBashDefaultPath) {
		t.Skip("Git Bash not installed at default path; skipping real swarm-shell test")
	}
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)

	confPath := filepath.Join(home, ".psmux.conf")
	// A user who already set a (different) default-shell must be left untouched.
	existing := `set -g default-shell "/usr/bin/zsh"` + "\n"
	if err := os.WriteFile(confPath, []byte(existing), 0o644); err != nil {
		t.Fatalf("seed conf: %v", err)
	}

	if err := EnsureSwarmShell(); err != nil {
		t.Fatalf("EnsureSwarmShell: %v", err)
	}

	conf := readConf(t, confPath)
	if countDefaultShellLines(conf) != 1 {
		t.Fatalf("EnsureSwarmShell added a duplicate default-shell line:\n%s", conf)
	}
	if !strings.Contains(conf, "/usr/bin/zsh") {
		t.Fatalf("EnsureSwarmShell clobbered the user's existing default-shell:\n%s", conf)
	}
}
