//go:build psmux_integration

// Package session integration test. Drives REAL psmux with a benign sleeper
// command (never claude, never any secret). Run with:
//
//	go test -tags psmux_integration ./internal/session/ -run TestPsmuxIntegration -v
//
// It is excluded from normal CI by the build tag above.
package session

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// benignResolver returns a launch spec that runs a harmless sleeper via bash so
// the session stays alive long enough to observe, then exits on its own. NO
// secrets are used; the only "secret-shaped" var is a benign sentinel we verify
// propagates via set-environment.
func benignResolver(env []string) LaunchResolver {
	return func(projectID, accountID string) ([]string, string, []string, string, error) {
		return env, "bash", []string{"-c", "sleep 30"}, "", nil
	}
}

func requirePsmux(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath(psmuxBinary); err != nil {
		t.Skipf("psmux not on PATH: %v", err)
	}
}

func TestPsmuxIntegrationLifecycle(t *testing.T) {
	requirePsmux(t)

	// Clean slate and guaranteed teardown of the qs server.
	killServer := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = psmuxRunner{}.run(ctx, "kill-server")
	}
	killServer()
	t.Cleanup(killServer)

	env := []string{
		"PATH=" + getPath(),
		"QS_BENIGN_SENTINEL=hello-from-resolver",
	}
	e := NewPsmuxEngine(benignResolver(env))
	e.pollInterval = 500 * time.Millisecond
	defer e.Close()

	s, err := e.Start(SessionSpec{ProjectID: "itest", AccountID: "sleeper"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// ls should show the session.
	if !sessionExists(t, s.PsmuxSession) {
		t.Fatalf("session %q not found via ls", s.PsmuxSession)
	}

	// capture should be non-empty within a short window.
	waitFor(t, 5*time.Second, func() bool {
		out, err := e.Capture(s.ID)
		return err == nil && strings.TrimSpace(out) != ""
	}, "capture never returned non-empty output")

	// set-environment propagation: the benign sentinel injected via Start must be
	// visible in the session environment.
	if got := showEnv(t, s.PsmuxSession, "QS_BENIGN_SENTINEL"); !strings.Contains(got, "hello-from-resolver") {
		t.Fatalf("sentinel env did not propagate via set-environment: %q", got)
	}
	// The swarm var must also have propagated.
	if got := showEnv(t, s.PsmuxSession, swarmEnvVar); !strings.Contains(got, "1") {
		t.Fatalf("%s did not propagate: %q", swarmEnvVar, got)
	}

	// Kill and confirm the poller emits EventExited.
	if err := e.Kill(s.ID); err != nil {
		t.Fatalf("Kill: %v", err)
	}

	sawExit := false
	deadline := time.After(10 * time.Second)
	for !sawExit {
		select {
		case ev := <-e.Events():
			if ev.Kind == EventExited && ev.Session.ID == s.ID {
				sawExit = true
			}
		case <-deadline:
			t.Fatal("never observed EventExited after Kill")
		}
	}

	if sessionExists(t, s.PsmuxSession) {
		t.Fatalf("session %q still alive after Kill", s.PsmuxSession)
	}
}

// --- helpers (integration only) ---

func getPath() string {
	for _, kv := range os.Environ() {
		if strings.HasPrefix(strings.ToUpper(kv), "PATH=") {
			return kv[5:]
		}
	}
	return ""
}

func sessionExists(t *testing.T, name string) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := psmuxRunner{}.run(ctx, "ls", "-F", "#{session_name}")
	if err != nil {
		return false
	}
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == name {
			return true
		}
	}
	return false
}

func showEnv(t *testing.T, name, key string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, _ := psmuxRunner{}.run(ctx, "show-environment", "-t", name, key)
	return out
}

func waitFor(t *testing.T, d time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal(msg)
}
