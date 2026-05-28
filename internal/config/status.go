package config

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// statusProbeTimeout bounds how long a StatusCmd may run before it is treated
// as an error. A hung probe must never wedge the dashboard. It is a var (not a
// const) so tests can shorten it.
var statusProbeTimeout = 5 * time.Second

// envHasKey reports whether the given env (a []string of KEY=VALUE) contains the
// named variable, matched case-insensitively (Windows semantics) and treated as
// present only when non-empty.
func envHasKey(env []string, name string) bool {
	target := strings.ToUpper(name)
	for _, kv := range env {
		eq := strings.IndexByte(kv, '=')
		if eq <= 0 {
			continue
		}
		if strings.ToUpper(kv[:eq]) == target && kv[eq+1:] != "" {
			return true
		}
	}
	return false
}

// ExpectedKeysPresent reports how many of the expected env var names are present
// (non-empty) in env, for "N of M connected" reporting. total is len(expected).
func ExpectedKeysPresent(env []string, expected []string) (have int, total int) {
	total = len(expected)
	for _, name := range expected {
		if envHasKey(env, name) {
			have++
		}
	}
	return have, total
}

// ProbeServiceStatus determines a service's runtime status using cheap checks
// only. It runs, in order:
//
//   - If any of svc.RequiresEnv is missing from env, return StatusUnknown
//     (not configured enough to probe). If all are present and there is no
//     StatusCmd, return StatusConfigured.
//   - If svc.StatusCmd is set, run it with c.Env = env (the firewall env, NEVER
//     os.Environ()): exit 0 => StatusReachable, non-zero => StatusError.
//   - If nothing can be probed at all, return StatusUnknown.
//
// The env passed in MUST be a resolved session/profile env so a probe command
// never sees the host shell's secrets.
func ProbeServiceStatus(svc Service, env []string) ServiceStatus {
	status := ServiceStatus{
		ServiceID: svc.ID,
		State:     StatusUnknown,
		CheckedAt: time.Now(),
	}

	// Required env must all be present before we consider the service usable.
	if len(svc.RequiresEnv) > 0 {
		have, total := ExpectedKeysPresent(env, svc.RequiresEnv)
		if have < total {
			status.State = StatusUnknown
			status.Detail = "missing required env"
			return status
		}
		status.State = StatusConfigured
	}

	statusCmd := strings.TrimSpace(svc.StatusCmd)
	if statusCmd == "" {
		// No command to run: configured (if it had required env) or unknown.
		return status
	}

	parts := splitCommand(statusCmd)
	if len(parts) == 0 {
		return status
	}

	ctx, cancel := context.WithTimeout(context.Background(), statusProbeTimeout)
	defer cancel()

	c := exec.CommandContext(ctx, parts[0], parts[1:]...)
	c.Env = env // firewall env only — never os.Environ()
	err := c.Run()
	if ctx.Err() == context.DeadlineExceeded {
		status.State = StatusError
		status.Detail = "probe timed out"
		return status
	}
	if err != nil {
		status.State = StatusError
		status.Detail = err.Error()
		return status
	}

	status.State = StatusReachable
	status.Detail = ""
	return status
}

// splitCommand tokenizes a command string, treating a double-quoted run as a
// single token so paths/args with spaces survive (e.g.
// `"C:/Program Files/x/p.exe" --flag "a b"` => three tokens). It is NOT a full
// shell parser: it only honors double quotes and splits on unquoted whitespace.
func splitCommand(s string) []string {
	var tokens []string
	var cur strings.Builder
	inQuote := false
	hasToken := false

	flush := func() {
		if hasToken {
			tokens = append(tokens, cur.String())
			cur.Reset()
			hasToken = false
		}
	}

	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			hasToken = true // an empty "" is still a (empty) token
		case !inQuote && (r == ' ' || r == '\t' || r == '\n' || r == '\r'):
			flush()
		default:
			cur.WriteRune(r)
			hasToken = true
		}
	}
	flush()
	return tokens
}
