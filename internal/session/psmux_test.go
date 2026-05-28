package session

import (
	"context"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// compile-time assertion that PsmuxEngine satisfies the frozen contract. This
// mirrors the assertion in psmux.go but lives in the test package per the task
// requirement that the unit suite assert it explicitly.
var _ SessionEngine = (*PsmuxEngine)(nil)

// fakeRunner records every psmux invocation and returns scripted output. It is
// the seam that lets us assert exact arg sequences without touching real psmux.
type fakeRunner struct {
	mu sync.Mutex
	// calls is the ordered list of arg slices passed to run (NOT including the
	// "-L qs" socket flag, which the runner contract strips/owns).
	calls [][]string
	// commands records arg slices passed to command (Attach path).
	commands [][]string

	// responder, if set, returns (stdout, err) for a given call's args. If nil,
	// run returns ("", nil) for every call.
	responder func(args []string) (string, error)
}

func (f *fakeRunner) run(_ context.Context, args ...string) (string, error) {
	f.mu.Lock()
	cp := append([]string(nil), args...)
	f.calls = append(f.calls, cp)
	resp := f.responder
	f.mu.Unlock()
	if resp != nil {
		return resp(cp)
	}
	return "", nil
}

func (f *fakeRunner) command(args ...string) *exec.Cmd {
	f.mu.Lock()
	cp := append([]string(nil), args...)
	f.commands = append(f.commands, cp)
	f.mu.Unlock()
	// Mirror the real runner: prepend the socket flag so callers/tests see the
	// full argv the user would run.
	full := append([]string{"-L", psmuxSocket}, args...)
	return exec.Command(psmuxBinary, full...)
}

func (f *fakeRunner) snapshotCalls() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]string, len(f.calls))
	copy(out, f.calls)
	return out
}

// findCall returns the first recorded call whose first arg equals subcommand,
// or nil if none.
func (f *fakeRunner) findCall(subcommand string) []string {
	for _, c := range f.snapshotCalls() {
		if len(c) > 0 && c[0] == subcommand {
			return c
		}
	}
	return nil
}

// findAllCalls returns every recorded call whose first arg equals subcommand.
func (f *fakeRunner) findAllCalls(subcommand string) [][]string {
	var out [][]string
	for _, c := range f.snapshotCalls() {
		if len(c) > 0 && c[0] == subcommand {
			out = append(out, c)
		}
	}
	return out
}

// fixedResolver returns a LaunchResolver that always yields the given values.
// The env here stands in for the CLEAN firewall env from config.ResolveSessionEnv.
func fixedResolver(env []string, command string, args []string, dir string) LaunchResolver {
	return func(projectID, accountID string) ([]string, string, []string, string, error) {
		return env, command, args, dir, nil
	}
}

// newTestEngine builds a PsmuxEngine wired to a fake runner with a fast poll
// interval and short command timeout suitable for tests. It also stubs out the
// swarm-shell check via the package-level test hook so Start does not touch the
// real filesystem / Git Bash.
func newTestEngine(t *testing.T, r runner, resolver LaunchResolver) *PsmuxEngine {
	t.Helper()
	restore := stubEnsureSwarmShell()
	t.Cleanup(restore)
	e := newPsmuxEngine(resolver, r)
	e.pollInterval = 10 * time.Millisecond
	e.cmdTimeout = 2 * time.Second
	t.Cleanup(func() { _ = e.Close() })
	return e
}

func TestStartArgSequenceAndEnvInjection(t *testing.T) {
	// A clean resolver env plus a value that must NOT come from os.Environ().
	resolverEnv := []string{
		"PATH=C:/clean/path",
		"SECRET_TOKEN=swordfish",
	}
	r := &fakeRunner{}
	e := newTestEngine(t, r, fixedResolver(resolverEnv, "claude", []string{"--effort", "max"}, "C:/proj"))

	s, err := e.Start(SessionSpec{ProjectID: "myproj", AccountID: "claude"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Session name is engine-owned: qs-<project>-<account>-<short>.
	if !strings.HasPrefix(s.PsmuxSession, "qs-myproj-claude-") {
		t.Fatalf("unexpected session name %q", s.PsmuxSession)
	}
	if s.ID != s.PsmuxSession {
		t.Fatalf("ID (%q) should equal PsmuxSession (%q)", s.ID, s.PsmuxSession)
	}
	if s.State != StateRunning {
		t.Fatalf("expected StateRunning, got %q", s.State)
	}

	name := s.PsmuxSession

	// new-session must be detached, named, and carry the working dir.
	create := r.findCall("new-session")
	if create == nil {
		t.Fatal("no new-session call recorded")
	}
	wantCreate := []string{"new-session", "-d", "-s", name, "-c", "C:/proj"}
	if !equalArgs(create, wantCreate) {
		t.Fatalf("new-session args:\n got %v\nwant %v", create, wantCreate)
	}

	// set-environment must be used for EVERY resolved var (secrets included),
	// keeping them off the command line / shell history.
	setEnv := r.findAllCalls("set-environment")
	gotEnv := map[string]string{}
	for _, c := range setEnv {
		// set-environment -t <name> KEY VALUE
		if len(c) != 5 || c[1] != "-t" || c[2] != name {
			t.Fatalf("malformed set-environment call: %v", c)
		}
		gotEnv[c[3]] = c[4]
	}
	if gotEnv["SECRET_TOKEN"] != "swordfish" {
		t.Fatalf("SECRET_TOKEN not injected via set-environment: %v", gotEnv)
	}
	if gotEnv["PATH"] != "C:/clean/path" {
		t.Fatalf("PATH not injected via set-environment: %v", gotEnv)
	}
	// The swarm var must be injected too.
	if gotEnv[swarmEnvVar] != "1" {
		t.Fatalf("%s not injected: %v", swarmEnvVar, gotEnv)
	}

	// Secrets must NOT be typed via send-keys (which would leak to history).
	for _, c := range r.findAllCalls("send-keys") {
		joined := strings.Join(c, " ")
		if strings.Contains(joined, "swordfish") {
			t.Fatalf("secret leaked into send-keys call: %v", c)
		}
	}

	// The launch send-keys must reference the resolved command + args and end
	// with a literal Enter as a SEPARATE arg.
	launch := r.findCall("send-keys")
	if launch == nil {
		t.Fatal("no send-keys (launch) call recorded")
	}
	if launch[len(launch)-1] != "Enter" {
		t.Fatalf("send-keys must end with Enter arg, got %v", launch)
	}
	line := launch[len(launch)-2]
	if !strings.Contains(line, "claude") || !strings.Contains(line, "--effort") || !strings.Contains(line, "max") {
		t.Fatalf("launch line missing resolved command/args: %q", line)
	}
}

func TestStartDoesNotReadOSEnviron(t *testing.T) {
	// Set a sentinel in the REAL process env. If the engine ever did
	// append(os.Environ(), ...), this would show up in the injected env.
	t.Setenv("QS_OS_ENVIRON_SENTINEL", "leaked")

	resolverEnv := []string{"PATH=C:/clean"}
	r := &fakeRunner{}
	e := newTestEngine(t, r, fixedResolver(resolverEnv, "claude", nil, "C:/proj"))

	if _, err := e.Start(SessionSpec{ProjectID: "p", AccountID: "claude"}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	for _, c := range r.snapshotCalls() {
		if strings.Contains(strings.Join(c, "\x00"), "QS_OS_ENVIRON_SENTINEL") {
			t.Fatalf("os.Environ() leaked into psmux call: %v", c)
		}
	}
}

// TestStartUsesSpecWorkingDirWhenResolverHasNoDir verifies the unbound-folder
// path: when the resolver returns an empty dir (unbound project), Start falls
// back to spec.WorkingDir for both new-session -c and the launch cd.
func TestStartUsesSpecWorkingDirWhenResolverHasNoDir(t *testing.T) {
	r := &fakeRunner{}
	// Resolver returns NO dir (empty), mimicking an unbound project.
	e := newTestEngine(t, r, fixedResolver([]string{"PATH=C:/clean"}, "claude", nil, ""))

	s, err := e.Start(SessionSpec{ProjectID: "", AccountID: "claude", WorkingDir: "C:/folder"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	create := r.findCall("new-session")
	wantCreate := []string{"new-session", "-d", "-s", s.PsmuxSession, "-c", "C:/folder"}
	if !equalArgs(create, wantCreate) {
		t.Fatalf("new-session args:\n got %v\nwant %v", create, wantCreate)
	}

	launch := r.findCall("send-keys")
	if launch == nil {
		t.Fatal("no send-keys (launch) call recorded")
	}
	line := launch[len(launch)-2]
	if !strings.Contains(line, "C:/folder") {
		t.Fatalf("launch line did not cd into spec.WorkingDir: %q", line)
	}
}

// TestStartResolverDirWinsOverSpecWorkingDir verifies a bound project's resolved
// dir takes precedence over any spec.WorkingDir fallback.
func TestStartResolverDirWinsOverSpecWorkingDir(t *testing.T) {
	r := &fakeRunner{}
	e := newTestEngine(t, r, fixedResolver([]string{"PATH=C:/clean"}, "claude", nil, "C:/resolved"))

	s, err := e.Start(SessionSpec{ProjectID: "bound", AccountID: "claude", WorkingDir: "C:/folder"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	create := r.findCall("new-session")
	wantCreate := []string{"new-session", "-d", "-s", s.PsmuxSession, "-c", "C:/resolved"}
	if !equalArgs(create, wantCreate) {
		t.Fatalf("new-session args:\n got %v\nwant %v", create, wantCreate)
	}
}

func TestStopArgsAndSyncExitEvent(t *testing.T) {
	r := &fakeRunner{}
	e := newTestEngine(t, r, fixedResolver([]string{"PATH=x"}, "claude", nil, ""))

	s, err := e.Start(SessionSpec{ProjectID: "p", AccountID: "a"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	drainStartedEvent(t, e)

	if err := e.Stop(s.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	kill := r.findCall("kill-session")
	if kill == nil {
		t.Fatal("no kill-session call recorded")
	}
	want := []string{"kill-session", "-t", s.PsmuxSession}
	if !equalArgs(kill, want) {
		t.Fatalf("kill-session args:\n got %v\nwant %v", kill, want)
	}

	// EventExited must be emitted SYNCHRONOUSLY (not on a later poll tick).
	select {
	case ev := <-e.Events():
		if ev.Kind != EventExited {
			t.Fatalf("expected EventExited, got %q", ev.Kind)
		}
		if ev.Session.State != StateExited {
			t.Fatalf("exited event state = %q", ev.Session.State)
		}
	case <-time.After(time.Second):
		t.Fatal("Stop did not emit EventExited synchronously")
	}

	// Cached state reflects the exit.
	for _, got := range e.List() {
		if got.ID == s.ID && got.State != StateExited {
			t.Fatalf("expected StateExited in List, got %q", got.State)
		}
	}

	if err := e.Stop("nope"); err == nil {
		t.Fatal("expected error stopping unknown session")
	}
}

func TestKillForceTreatsMissingAsSuccess(t *testing.T) {
	r := &fakeRunner{
		responder: func(args []string) (string, error) {
			if len(args) > 0 && args[0] == "kill-session" {
				return "can't find session", &exec.ExitError{}
			}
			return "", nil
		},
	}
	e := newTestEngine(t, r, fixedResolver([]string{"PATH=x"}, "claude", nil, ""))

	s, err := e.Start(SessionSpec{ProjectID: "p", AccountID: "a"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	drainStartedEvent(t, e)

	// Kill must succeed even though kill-session errored (force semantics).
	if err := e.Kill(s.ID); err != nil {
		t.Fatalf("Kill should succeed on force despite psmux error: %v", err)
	}

	select {
	case ev := <-e.Events():
		if ev.Kind != EventExited {
			t.Fatalf("expected EventExited from Kill, got %q", ev.Kind)
		}
	case <-time.After(time.Second):
		t.Fatal("Kill did not emit EventExited synchronously")
	}
}

func TestSendKeysArgs(t *testing.T) {
	r := &fakeRunner{}
	e := newTestEngine(t, r, fixedResolver([]string{"PATH=x"}, "claude", nil, ""))

	s, err := e.Start(SessionSpec{ProjectID: "p", AccountID: "a"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := e.SendKeys(s.ID, "/help"); err != nil {
		t.Fatalf("SendKeys: %v", err)
	}

	// The most recent send-keys must be ours: -t <name> "/help" Enter.
	all := r.findAllCalls("send-keys")
	last := all[len(all)-1]
	want := []string{"send-keys", "-t", s.PsmuxSession, "/help", "Enter"}
	if !equalArgs(last, want) {
		t.Fatalf("send-keys args:\n got %v\nwant %v", last, want)
	}

	if err := e.SendKeys("nope", "x"); err == nil {
		t.Fatal("expected error sending keys to unknown session")
	}
}

func TestCaptureArgsAndOutput(t *testing.T) {
	r := &fakeRunner{
		responder: func(args []string) (string, error) {
			if len(args) > 0 && args[0] == "capture-pane" {
				return "SCREEN CONTENTS\n", nil
			}
			return "", nil
		},
	}
	e := newTestEngine(t, r, fixedResolver([]string{"PATH=x"}, "claude", nil, ""))

	s, err := e.Start(SessionSpec{ProjectID: "p", AccountID: "a"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	out, err := e.Capture(s.ID)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if out != "SCREEN CONTENTS\n" {
		t.Fatalf("Capture output = %q", out)
	}

	cap := r.findCall("capture-pane")
	want := []string{"capture-pane", "-p", "-t", s.PsmuxSession}
	if !equalArgs(cap, want) {
		t.Fatalf("capture-pane args:\n got %v\nwant %v", cap, want)
	}

	if _, err := e.Capture("nope"); err == nil {
		t.Fatal("expected error capturing unknown session")
	}
}

func TestAttachReturnsPreparedCmd(t *testing.T) {
	r := &fakeRunner{}
	e := newTestEngine(t, r, fixedResolver([]string{"PATH=x"}, "claude", nil, ""))

	s, err := e.Start(SessionSpec{ProjectID: "p", AccountID: "a"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	cmd, err := e.Attach(s.ID)
	if err != nil {
		t.Fatalf("Attach: %v", err)
	}
	if cmd == nil {
		t.Fatal("Attach returned nil *exec.Cmd")
	}

	// The prepared argv must be: psmux -L qs attach -t <name>.
	wantArgs := []string{psmuxBinary, "-L", psmuxSocket, "attach", "-t", s.PsmuxSession}
	if !equalArgs(cmd.Args, wantArgs) {
		t.Fatalf("Attach cmd.Args:\n got %v\nwant %v", cmd.Args, wantArgs)
	}

	// Attach must NOT have been run: ProcessState is nil until Run/Start.
	if cmd.ProcessState != nil {
		t.Fatal("Attach must not run the command")
	}

	if _, err := e.Attach("nope"); err == nil {
		t.Fatal("expected error attaching to unknown session")
	}
}

func TestListReturnsSnapshotCopies(t *testing.T) {
	r := &fakeRunner{}
	e := newTestEngine(t, r, fixedResolver([]string{"PATH=x"}, "claude", nil, ""))

	s1, err := e.Start(SessionSpec{ProjectID: "p1", AccountID: "a"})
	if err != nil {
		t.Fatalf("Start1: %v", err)
	}
	s2, err := e.Start(SessionSpec{ProjectID: "p2", AccountID: "a"})
	if err != nil {
		t.Fatalf("Start2: %v", err)
	}

	list := e.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(list))
	}

	// Mutating a returned snapshot must not affect engine state.
	for i := range list {
		list[i].State = StateError
	}
	for _, got := range e.List() {
		if got.State == StateError {
			t.Fatal("List returned a reference, not a copy")
		}
	}

	ids := map[string]bool{s1.ID: false, s2.ID: false}
	for _, got := range e.List() {
		if _, ok := ids[got.ID]; ok {
			ids[got.ID] = true
		}
	}
	for id, seen := range ids {
		if !seen {
			t.Fatalf("session %q missing from List", id)
		}
	}
}

func TestStartFailsWhenEngineClosed(t *testing.T) {
	r := &fakeRunner{}
	e := newTestEngine(t, r, fixedResolver([]string{"PATH=x"}, "claude", nil, ""))
	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := e.Start(SessionSpec{ProjectID: "p", AccountID: "a"}); err == nil {
		t.Fatal("expected Start to fail on a closed engine")
	}
}

func TestCloseIsIdempotentAndClosesEvents(t *testing.T) {
	r := &fakeRunner{}
	e := newTestEngine(t, r, fixedResolver([]string{"PATH=x"}, "claude", nil, ""))

	if _, err := e.Start(SessionSpec{ProjectID: "p", AccountID: "a"}); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Second Close must not panic (no double close of channels).
	if err := e.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}

	// Events channel must be closed and drainable.
	done := make(chan struct{})
	go func() {
		for range e.Events() {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Events channel was not closed by Close")
	}
}

func TestResolverErrorPropagates(t *testing.T) {
	r := &fakeRunner{}
	restore := stubEnsureSwarmShell()
	defer restore()
	bad := func(projectID, accountID string) ([]string, string, []string, string, error) {
		return nil, "", nil, "", &exec.ExitError{}
	}
	e := newPsmuxEngine(bad, r)
	e.pollInterval = 10 * time.Millisecond
	defer e.Close()

	if _, err := e.Start(SessionSpec{ProjectID: "p", AccountID: "a"}); err == nil {
		t.Fatal("expected Start to propagate resolver error")
	}
	// No psmux calls should have been made on resolver failure.
	if len(r.snapshotCalls()) != 0 {
		t.Fatalf("expected no psmux calls on resolver error, got %v", r.snapshotCalls())
	}
}

func TestWithSwarmEnvAddsAndDeduplicates(t *testing.T) {
	// Adds when absent.
	got := withSwarmEnv([]string{"PATH=x"})
	if !containsKV(got, swarmEnvVar+"=1") {
		t.Fatalf("withSwarmEnv did not add swarm var: %v", got)
	}

	// Does not duplicate when already present (any value, any case).
	pre := []string{"PATH=x", swarmEnvVar + "=already"}
	got = withSwarmEnv(pre)
	count := 0
	for _, kv := range got {
		if strings.HasPrefix(strings.ToUpper(kv), strings.ToUpper(swarmEnvVar)+"=") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("withSwarmEnv duplicated swarm var: %v", got)
	}
	if !containsKV(got, swarmEnvVar+"=already") {
		t.Fatalf("withSwarmEnv clobbered existing swarm value: %v", got)
	}

	// Must not mutate input slice's contents in place beyond appending a copy.
	orig := []string{"PATH=x"}
	_ = withSwarmEnv(orig)
	if len(orig) != 1 || orig[0] != "PATH=x" {
		t.Fatalf("withSwarmEnv mutated input: %v", orig)
	}
}

func TestSessionNameSanitizes(t *testing.T) {
	name := newSessionName("my.proj:1", "ac count")
	if strings.ContainsAny(name, ".: ") {
		t.Fatalf("session name not sanitized: %q", name)
	}
	if !strings.HasPrefix(name, "qs-my_proj_1-ac_count-") {
		t.Fatalf("unexpected sanitized name: %q", name)
	}
}

// --- helpers ---

func equalArgs(a, b []string) bool {
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

func containsKV(env []string, kv string) bool {
	for _, e := range env {
		if e == kv {
			return true
		}
	}
	return false
}

// drainStartedEvent consumes the EventStarted emitted by Start so a later
// assertion on the events channel sees the event it actually cares about.
func drainStartedEvent(t *testing.T, e *PsmuxEngine) {
	t.Helper()
	select {
	case ev := <-e.Events():
		if ev.Kind != EventStarted {
			t.Fatalf("expected EventStarted first, got %q", ev.Kind)
		}
	case <-time.After(time.Second):
		t.Fatal("no EventStarted emitted by Start")
	}
}
