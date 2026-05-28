package session

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// psmuxSocket is the dedicated psmux server socket name qs uses. All psmux
// invocations pass "-L qs" before the subcommand so qs sessions live on their
// own server, isolated from any other tmux/psmux the user is running.
const psmuxSocket = "qs"

// psmuxBinary is the executable name. psmux is expected on PATH.
const psmuxBinary = "psmux"

// swarmEnvVar enables Claude Code's experimental agent-teams feature inside a
// launched session. It is added to every launched command's environment.
const swarmEnvVar = "CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS"

// runner is the seam between the engine and the real psmux CLI. The engine
// only ever talks to psmux through a runner, which makes it unit-testable with
// a fake. All args are passed AFTER the "-L qs" socket flag, which the runner
// supplies itself.
type runner interface {
	// run executes "psmux -L qs <args...>" and returns its combined behavior:
	// stdout on success, or a wrapped error on failure. ctx bounds the call.
	run(ctx context.Context, args ...string) (stdout string, err error)
	// command prepares (but does not run) an *exec.Cmd for
	// "psmux -L qs <args...>". Used for Attach, where the CALLER runs the
	// command via tea.ExecProcess.
	command(args ...string) *exec.Cmd
}

// psmuxRunner is the real runner that execs the psmux binary.
type psmuxRunner struct{}

func (psmuxRunner) run(ctx context.Context, args ...string) (string, error) {
	full := append([]string{"-L", psmuxSocket}, args...)
	cmd := exec.CommandContext(ctx, psmuxBinary, full...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("psmux %s: %w: %s",
			strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

func (psmuxRunner) command(args ...string) *exec.Cmd {
	full := append([]string{"-L", psmuxSocket}, args...)
	return exec.Command(psmuxBinary, full...)
}

// PsmuxEngine is a SessionEngine backed by the external psmux multiplexer. It
// owns psmux session naming, builds child environments exclusively from its
// LaunchResolver (never os.Environ()), and runs a single background poller that
// reconciles cached state with psmux and emits SessionEvents.
type PsmuxEngine struct {
	resolver LaunchResolver
	runner   runner

	mu       sync.Mutex
	sessions map[string]*Session // keyed by Session.ID

	events chan SessionEvent

	// poller lifecycle.
	pollerStart   sync.Once
	pollerRunning bool // guarded by mu; true once the poller goroutine exists
	stop          chan struct{}
	done          chan struct{}

	closeOnce sync.Once

	// pollInterval controls the poller ticker; overridable in tests.
	pollInterval time.Duration
	// cmdTimeout bounds each individual psmux invocation.
	cmdTimeout time.Duration
}

// compile-time assertion that PsmuxEngine satisfies the frozen contract.
var _ SessionEngine = (*PsmuxEngine)(nil)

// NewPsmuxEngine constructs a PsmuxEngine with the real psmux runner. The
// resolver supplies the clean child environment, command, args, and working
// directory for each session. It panics if resolver is nil — a misconfigured
// engine that silently launched children with no resolved environment would be
// a security hazard.
func NewPsmuxEngine(resolver LaunchResolver) *PsmuxEngine {
	return newPsmuxEngine(resolver, psmuxRunner{})
}

// newPsmuxEngine is the internal constructor that accepts an injected runner so
// tests can supply a fake.
func newPsmuxEngine(resolver LaunchResolver, r runner) *PsmuxEngine {
	if resolver == nil {
		panic("session: NewPsmuxEngine requires a non-nil LaunchResolver")
	}
	return &PsmuxEngine{
		resolver:     resolver,
		runner:       r,
		sessions:     make(map[string]*Session),
		events:       make(chan SessionEvent, 64),
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
		pollInterval: 1500 * time.Millisecond,
		cmdTimeout:   10 * time.Second,
	}
}

// ctx returns a context bounded by cmdTimeout for a single psmux call.
func (e *PsmuxEngine) ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), e.cmdTimeout)
}

// Start launches a new session for the given spec. It resolves the clean child
// environment via the LaunchResolver (never os.Environ()), ensures psmux uses
// Git Bash, creates a detached session, injects the resolved env via
// set-environment (so secrets never pass through send-keys), and launches the
// resolved command (with the swarm env var added) inside the session.
func (e *PsmuxEngine) Start(spec SessionSpec) (Session, error) {
	if e.isClosed() {
		return Session{}, fmt.Errorf("start %s/%s: engine closed", spec.ProjectID, spec.AccountID)
	}

	// Ensure psmux will use Git Bash before we spawn anything; dead panes from
	// a PowerShell fallback are worse than a clear up-front error.
	if err := ensureSwarmShell(); err != nil {
		return Session{}, fmt.Errorf("start %s/%s: %w", spec.ProjectID, spec.AccountID, err)
	}

	env, command, args, dir, err := e.resolver(spec.ProjectID, spec.AccountID)
	if err != nil {
		return Session{}, fmt.Errorf("start %s/%s: resolve launch: %w", spec.ProjectID, spec.AccountID, err)
	}
	// For an unbound folder the resolver supplies no dir; fall back to the
	// caller-provided working dir so the session still launches in the folder.
	if dir == "" {
		dir = spec.WorkingDir
	}
	if command == "" {
		return Session{}, fmt.Errorf("start %s/%s: resolver returned empty command", spec.ProjectID, spec.AccountID)
	}

	// The child env is built ONLY from the resolver output plus the swarm var.
	// We deliberately do NOT consult os.Environ() here.
	childEnv := withSwarmEnv(env)

	name := newSessionName(spec.ProjectID, spec.AccountID)

	ctx, cancel := e.ctx()
	defer cancel()

	// 1. Create the detached session with an idle shell. We launch the actual
	//    tool via send-keys afterward so that the working dir + env are fully in
	//    place first and so the pane survives the tool exiting (for capture).
	createArgs := []string{"new-session", "-d", "-s", name}
	if dir != "" {
		createArgs = append(createArgs, "-c", dir)
	}
	if _, err := e.runner.run(ctx, createArgs...); err != nil {
		return Session{}, fmt.Errorf("start %s/%s: create session: %w", spec.ProjectID, spec.AccountID, err)
	}

	// 2. Inject env via set-environment (session-scoped). This keeps secrets
	//    OUT of the command line and shell history — unlike send-keys, which
	//    would echo them. Failures here tear the session back down.
	for _, kv := range childEnv {
		key, val, ok := splitEnv(kv)
		if !ok {
			continue
		}
		if _, err := e.runner.run(ctx, "set-environment", "-t", name, key, val); err != nil {
			// Best-effort cleanup; report the original failure.
			cleanupCtx, cleanupCancel := e.ctx()
			_, _ = e.runner.run(cleanupCtx, "kill-session", "-t", name)
			cleanupCancel()
			return Session{}, fmt.Errorf("start %s/%s: set-environment %s: %w", spec.ProjectID, spec.AccountID, key, err)
		}
	}

	// 3. Launch the resolved command inside the session via send-keys. Only the
	//    non-secret command + args are typed; the env was already injected
	//    above. The session env (set-environment) is inherited by the shell, so
	//    the launched tool sees every resolved var including the swarm flag.
	launchLine := buildLaunchLine(dir, command, args)
	if _, err := e.runner.run(ctx, "send-keys", "-t", name, launchLine, "Enter"); err != nil {
		cleanupCtx, cleanupCancel := e.ctx()
		_, _ = e.runner.run(cleanupCtx, "kill-session", "-t", name)
		cleanupCancel()
		return Session{}, fmt.Errorf("start %s/%s: launch command: %w", spec.ProjectID, spec.AccountID, err)
	}

	now := time.Now()
	s := &Session{
		ID:             name,
		ProjectID:      spec.ProjectID,
		AccountID:      spec.AccountID,
		PsmuxSession:   name,
		State:          StateRunning,
		StartedAt:      now,
		LastActivityAt: now,
	}

	e.mu.Lock()
	e.sessions[s.ID] = s
	snapshot := *s
	e.mu.Unlock()

	e.emit(SessionEvent{Kind: EventStarted, Session: snapshot})

	// Begin polling once a session exists.
	e.startPoller()

	return snapshot, nil
}

// Stop requests a graceful shutdown via kill-session and synchronously emits
// EventExited (it does not wait for the next poll tick).
func (e *PsmuxEngine) Stop(id string) error {
	return e.teardown(id, "stop", false)
}

// Kill forcibly terminates the session. For psmux, kill-session is already a
// hard teardown of the session's processes, so Kill and Stop use the same psmux
// verb; Kill additionally ignores a "session not found" condition as success
// since the goal is for the session to be gone.
func (e *PsmuxEngine) Kill(id string) error {
	return e.teardown(id, "kill", true)
}

// teardown kills the psmux session and updates cached state. force=true treats
// an already-missing session as success.
func (e *PsmuxEngine) teardown(id, op string, force bool) error {
	e.mu.Lock()
	s, ok := e.sessions[id]
	if !ok {
		e.mu.Unlock()
		return fmt.Errorf("%s %s: unknown session", op, id)
	}
	name := s.PsmuxSession
	e.mu.Unlock()

	ctx, cancel := e.ctx()
	defer cancel()

	if _, err := e.runner.run(ctx, "kill-session", "-t", name); err != nil {
		if !force {
			return fmt.Errorf("%s %s: %w", op, id, err)
		}
		// force: fall through and mark exited regardless.
	}

	e.mu.Lock()
	s, ok = e.sessions[id]
	if !ok {
		e.mu.Unlock()
		return nil
	}
	s.State = StateExited
	s.LastActivityAt = time.Now()
	snapshot := *s
	e.mu.Unlock()

	e.emit(SessionEvent{Kind: EventExited, Session: snapshot})
	return nil
}

// List returns a snapshot copy of all cached sessions.
func (e *PsmuxEngine) List() []Session {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Session, 0, len(e.sessions))
	for _, s := range e.sessions {
		out = append(out, *s)
	}
	return out
}

// Attach returns a prepared (but NOT run) *exec.Cmd for
// "psmux -L qs attach -t <name>". The caller runs it via tea.ExecProcess.
func (e *PsmuxEngine) Attach(id string) (*exec.Cmd, error) {
	e.mu.Lock()
	s, ok := e.sessions[id]
	if !ok {
		e.mu.Unlock()
		return nil, fmt.Errorf("attach %s: unknown session", id)
	}
	name := s.PsmuxSession
	e.mu.Unlock()

	return e.runner.command("attach", "-t", name), nil
}

// SendKeys types keys into the session followed by Enter. This is for
// non-secret input only — secrets are injected via set-environment in Start.
func (e *PsmuxEngine) SendKeys(id string, keys string) error {
	e.mu.Lock()
	s, ok := e.sessions[id]
	if !ok {
		e.mu.Unlock()
		return fmt.Errorf("send-keys %s: unknown session", id)
	}
	name := s.PsmuxSession
	e.mu.Unlock()

	ctx, cancel := e.ctx()
	defer cancel()

	if _, err := e.runner.run(ctx, "send-keys", "-t", name, keys, "Enter"); err != nil {
		return fmt.Errorf("send-keys %s: %w", id, err)
	}

	e.mu.Lock()
	if s, ok := e.sessions[id]; ok {
		s.LastActivityAt = time.Now()
	}
	e.mu.Unlock()
	return nil
}

// Capture returns a STATIC text snapshot of the session's screen via
// "capture-pane -p".
func (e *PsmuxEngine) Capture(id string) (string, error) {
	e.mu.Lock()
	s, ok := e.sessions[id]
	if !ok {
		e.mu.Unlock()
		return "", fmt.Errorf("capture %s: unknown session", id)
	}
	name := s.PsmuxSession
	e.mu.Unlock()

	ctx, cancel := e.ctx()
	defer cancel()

	out, err := e.runner.run(ctx, "capture-pane", "-p", "-t", name)
	if err != nil {
		return "", fmt.Errorf("capture %s: %w", id, err)
	}
	return out, nil
}

// Events returns the shared channel on which all session events are published.
func (e *PsmuxEngine) Events() <-chan SessionEvent {
	return e.events
}

// Close stops the poller and closes the Events channel exactly once. It does
// not kill running sessions (psmux sessions are daemon-held and survive client
// exit by design).
func (e *PsmuxEngine) Close() error {
	e.closeOnce.Do(func() {
		// Signal the poller (if started) to stop. Closing stop also makes emit()
		// drop further events so the poller cannot block on a send after we close
		// the events channel.
		close(e.stop)

		e.mu.Lock()
		running := e.pollerRunning
		e.mu.Unlock()

		if running {
			// Wait for the poller to actually exit so we never close the events
			// channel out from under a sending goroutine.
			select {
			case <-e.done:
			case <-time.After(2 * e.cmdTimeout):
				// Poller failed to exit in time; proceed anyway to avoid hanging
				// shutdown.
			}
		}
		close(e.events)
	})
	return nil
}

// isClosed reports whether Close has been initiated.
func (e *PsmuxEngine) isClosed() bool {
	select {
	case <-e.stop:
		return true
	default:
		return false
	}
}

// emit publishes an event, dropping it if the buffered channel is full or the
// engine is closing rather than blocking the caller. Events are advisory; the
// authoritative state is always List().
func (e *PsmuxEngine) emit(ev SessionEvent) {
	select {
	case <-e.stop:
		return
	default:
	}
	select {
	case e.events <- ev:
	default:
	}
}

// newSessionName builds the psmux session name. Format:
// "qs-<projectID>-<accountID>-<short>" with a random short suffix so repeated
// starts of the same project/account never collide. C and B never construct
// these names — the engine owns naming.
func newSessionName(projectID, accountID string) string {
	return fmt.Sprintf("qs-%s-%s-%s", sanitize(projectID), sanitize(accountID), shortID())
}

// sanitize maps characters that psmux treats specially in session names (".",
// ":", whitespace) to "_" so names round-trip through list/has-session cleanly.
func sanitize(s string) string {
	if s == "" {
		return "none"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// shortID returns a short random hex string used to disambiguate session names.
func shortID() string {
	var buf [4]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// Fall back to a time-based suffix; uniqueness is best-effort here and a
		// collision only means a second Start would fail loudly on new-session.
		return fmt.Sprintf("%x", time.Now().UnixNano()&0xffffffff)
	}
	return hex.EncodeToString(buf[:])
}

// splitEnv splits a "KEY=VALUE" entry. It returns ok=false for malformed
// entries (no '=' or empty key) so they are skipped rather than mis-injected.
func splitEnv(kv string) (key, value string, ok bool) {
	eq := strings.IndexByte(kv, '=')
	if eq <= 0 {
		return "", "", false
	}
	return kv[:eq], kv[eq+1:], true
}

// buildLaunchLine builds the shell line typed into the session to launch the
// tool. Because the engine ensures Git Bash is psmux's default shell, this uses
// bash syntax. The working dir was already set at new-session time via -c, so we
// only cd if a dir is present (defensive against shells that ignore -c). The env
// was injected via set-environment, so nothing secret appears on this line.
func buildLaunchLine(dir, command string, args []string) string {
	var b strings.Builder
	if dir != "" {
		b.WriteString("cd ")
		b.WriteString(shellQuote(dir))
		b.WriteString(" && ")
	}
	// exec replaces the shell with the tool so the pane's lifetime tracks the
	// tool process, giving accurate exited detection.
	b.WriteString("exec ")
	b.WriteString(shellQuote(command))
	for _, a := range args {
		b.WriteByte(' ')
		b.WriteString(shellQuote(a))
	}
	return b.String()
}

// shellQuote single-quotes a string for bash, escaping embedded single quotes.
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	// Replace ' with '\'' (close quote, escaped quote, reopen quote).
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
