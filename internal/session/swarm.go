package session

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// gitBashDefaultPath is the canonical install location of Git Bash on Windows.
const gitBashDefaultPath = `C:\Program Files\Git\bin\bash.exe`

// psmuxDefaultShellLine is the psmux config directive that forces psmux to use
// Git Bash as the shell for new windows/panes. Claude Code's agent-teams spawn
// teammates with bash syntax (cd '...' && env ... claude ...); psmux's default
// fallback shell on this box is PowerShell 5.1, which rejects that syntax,
// producing panes that spawn but never execute. Forcing Git Bash fixes it.
//
// The value uses forward slashes because that is what psmux expects in its
// config, matching the verified spike configuration.
const psmuxDefaultShellLine = `set -g default-shell "C:/Program Files/Git/bin/bash.exe"`

// psmuxDefaultShellKey is the prefix used to detect an existing default-shell
// directive so we never duplicate or clobber a user's own setting.
const psmuxDefaultShellKey = "set -g default-shell"

// ensureSwarmShell is the indirection through which the engine invokes
// EnsureSwarmShell. Tests override it (via stubEnsureSwarmShell) so unit tests
// never touch the real filesystem or require Git Bash to be installed.
var ensureSwarmShell = EnsureSwarmShell

// EnsureSwarmShell makes psmux use Git Bash as its default shell so that
// Claude Code agent-teams (which spawn teammates via bash split-window syntax)
// actually execute instead of producing dead panes.
//
// It is idempotent: it locates Git Bash, and if "~/.psmux.conf" does not
// already contain a default-shell directive it APPENDS the Git Bash directive.
// It never clobbers an existing user .psmux.conf — other lines are preserved and
// an existing default-shell line (even a different one) is left untouched.
//
// If Git Bash cannot be located it returns a clear error rather than silently
// allowing dead panes to be created later.
func EnsureSwarmShell() error {
	bashPath, err := findGitBash()
	if err != nil {
		return err
	}
	_ = bashPath // located only to fail fast; the config line is canonical.

	confPath, err := psmuxConfPath()
	if err != nil {
		return fmt.Errorf("ensure swarm shell: locate config: %w", err)
	}

	hasDirective, err := confHasDefaultShell(confPath)
	if err != nil {
		return fmt.Errorf("ensure swarm shell: read %s: %w", confPath, err)
	}
	if hasDirective {
		return nil
	}

	if err := appendDefaultShell(confPath); err != nil {
		return fmt.Errorf("ensure swarm shell: update %s: %w", confPath, err)
	}
	return nil
}

// findGitBash locates the Git Bash executable. It checks the canonical install
// path first, then falls back to a PATH lookup. The WSL bash.exe shim (which
// lives in %WINDIR%\System32 and cannot run the spawned tools) is explicitly
// rejected, so when the only bash on PATH is the WSL shim this returns the same
// "not found" error as having no bash at all. It returns a clear, actionable
// error if Git Bash is not found.
func findGitBash() (string, error) {
	if fileExists(gitBashDefaultPath) {
		return gitBashDefaultPath, nil
	}

	// Fall back to PATH lookup. exec.LookPath honors PATHEXT and PATH.
	if p, err := exec.LookPath("bash"); err == nil && !isWSLBashShim(p) {
		return p, nil
	}

	return "", fmt.Errorf(
		"ensure swarm shell: Git Bash not found at %q and 'bash' is not on PATH; "+
			"install Git for Windows (https://git-scm.com/download/win) so psmux can spawn agent-team panes",
		gitBashDefaultPath)
}

// isWSLBashShim reports whether the resolved bash path is the WSL launcher shim
// rather than real Git Bash. The shim is %WINDIR%\System32\bash.exe; we match it
// case-insensitively, either by its System32 directory or by a System32-rooted
// bash.exe basename, so it is never selected as the swarm shell.
func isWSLBashShim(path string) bool {
	lower := strings.ToLower(filepath.ToSlash(path))
	if strings.Contains(lower, "/windows/system32/") {
		return true
	}
	// Defensive: a bare System32 reference even without the windows/ prefix.
	return filepath.Base(lower) == "bash.exe" && strings.Contains(lower, "system32/")
}

// psmuxConfPath returns the path to the user's psmux config file (~/.psmux.conf).
func psmuxConfPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".psmux.conf"), nil
}

// confHasDefaultShell reports whether the config file already declares a
// default-shell directive. A missing file reports false (and is not an error).
func confHasDefaultShell(path string) (bool, error) {
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, psmuxDefaultShellKey) {
			return true, nil
		}
	}
	return false, scanner.Err()
}

// appendDefaultShell appends the Git Bash default-shell directive to the config
// file, creating it if necessary. Existing content is preserved (append mode);
// a leading newline is added if the file does not already end in one.
func appendDefaultShell(path string) error {
	needsLeadingNewline, err := fileNeedsLeadingNewline(path)
	if err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	var b strings.Builder
	if needsLeadingNewline {
		b.WriteString("\n")
	}
	b.WriteString("# Added by qs: use Git Bash so Claude Code agent-teams panes execute.\n")
	b.WriteString(psmuxDefaultShellLine)
	b.WriteString("\n")

	if _, err := f.WriteString(b.String()); err != nil {
		return err
	}
	return nil
}

// fileNeedsLeadingNewline reports whether an existing, non-empty file lacks a
// trailing newline (so an appended block would otherwise join the last line).
func fileNeedsLeadingNewline(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if info.Size() == 0 {
		return false, nil
	}

	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()

	buf := make([]byte, 1)
	if _, err := f.ReadAt(buf, info.Size()-1); err != nil {
		return false, err
	}
	return buf[0] != '\n', nil
}

// fileExists reports whether path exists and is a regular file (not a dir).
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// withSwarmEnv returns a copy of env with CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1
// added. If the variable is already present (case-insensitively, per Windows env
// semantics) the existing entry is left untouched and no duplicate is added.
//
// The input env is the CLEAN, resolver-built environment; this function NEVER
// reads os.Environ(). Only the single swarm var is layered on.
func withSwarmEnv(env []string) []string {
	out := make([]string, len(env), len(env)+1)
	copy(out, env)

	target := strings.ToUpper(swarmEnvVar) + "="
	for _, kv := range out {
		eq := strings.IndexByte(kv, '=')
		if eq <= 0 {
			continue
		}
		if strings.ToUpper(kv[:eq])+"=" == target {
			return out // already set; do not duplicate.
		}
	}

	return append(out, swarmEnvVar+"=1")
}
