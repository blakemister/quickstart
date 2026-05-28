package session

import (
	"os/exec"
	"time"
)

// SessionState is the lifecycle state of a managed session.
type SessionState string

const (
	// StateStarting means the session has been requested but its child
	// process has not yet been confirmed running.
	StateStarting SessionState = "starting"
	// StateRunning means the child process is alive and recently active.
	StateRunning SessionState = "running"
	// StateIdle means the child process is alive but has had no terminal
	// activity for an implementation-defined interval.
	StateIdle SessionState = "idle"
	// StateExited means the child process has terminated; see ExitCode.
	StateExited SessionState = "exited"
	// StateError means the session failed to start or crashed unexpectedly.
	StateError SessionState = "error"
)

// Session is the engine's view of a single managed AI-coding session.
type Session struct {
	// ID is the engine-assigned unique identifier for this session.
	ID string
	// ProjectID identifies the project this session is bound to.
	ProjectID string
	// AccountID identifies the account/tool this session runs.
	AccountID string
	// PsmuxSession is the underlying multiplexer session name (empty for
	// engines that do not use psmux, e.g. InProcessEngine).
	PsmuxSession string

	// State is the current lifecycle state.
	State SessionState
	// StartedAt is when the session was started.
	StartedAt time.Time
	// LastActivityAt is the timestamp of the most recent observed terminal
	// activity, used to derive the idle state.
	LastActivityAt time.Time
	// ExitCode is the child process exit code; only meaningful once State is
	// StateExited.
	ExitCode int
}

// SessionSpec describes a session to start. Callers pass only the project and
// account IDs; the engine resolves the child environment, command, args, and
// working directory internally via its LaunchResolver.
type SessionSpec struct {
	ProjectID string
	AccountID string
}

// SessionEventKind discriminates the kinds of events an engine emits.
type SessionEventKind string

const (
	// EventStarted is emitted when a session's child process begins running.
	EventStarted SessionEventKind = "started"
	// EventStateChange is emitted on any lifecycle state transition.
	EventStateChange SessionEventKind = "state_change"
	// EventActivity is emitted when terminal activity is observed.
	EventActivity SessionEventKind = "activity"
	// EventExited is emitted when a session's child process terminates.
	EventExited SessionEventKind = "exited"
	// EventError is emitted when an error occurs for a session; Err is set.
	EventError SessionEventKind = "error"
)

// SessionEvent is a single notification about a session, delivered on the
// channel returned by Events.
type SessionEvent struct {
	// Kind identifies what happened.
	Kind SessionEventKind
	// Session is a snapshot of the session at the time of the event.
	Session Session
	// Err carries the cause for EventError (and is otherwise nil).
	Err error
}

// SessionEngine manages persistent AI-coding sessions.
//
// Rendering ownership: psmux owns rendering. Capture returns a STATIC snapshot
// of a session's screen suitable for previews; it is not a live feed. Attach
// returns a prepared *exec.Cmd that the CALLER runs via tea.ExecProcess to
// interact with the live session. The engine never embeds a live pane and
// never calls tea.ExecProcess itself.
//
// All methods are safe for concurrent use unless an implementation documents
// otherwise.
type SessionEngine interface {
	// Start launches a new session for the given spec and returns its initial
	// Session record. The environment, command, args, and working directory
	// are resolved internally via the engine's LaunchResolver.
	Start(spec SessionSpec) (Session, error)
	// Stop requests a graceful shutdown of the session with the given ID.
	Stop(id string) error
	// Kill forcibly terminates the session with the given ID.
	Kill(id string) error
	// List returns a snapshot of all sessions currently known to the engine.
	List() []Session
	// Attach returns a prepared *exec.Cmd that, when run by the caller via
	// tea.ExecProcess, attaches the terminal to the live session. The engine
	// does not run the returned command.
	Attach(id string) (*exec.Cmd, error)
	// SendKeys sends the given key sequence to the session's input as if typed.
	SendKeys(id string, keys string) error
	// Capture returns a STATIC text snapshot of the session's current screen.
	Capture(id string) (string, error)
	// Events returns a channel on which the engine publishes SessionEvents.
	Events() <-chan SessionEvent
	// Close shuts down the engine, releasing all resources and stopping the
	// Events channel.
	Close() error
}

// LaunchResolver resolves how to launch a session's child process from its
// project and account IDs. It is satisfied (via an adapter in cmd/dash.go) by
// config.ResolveSessionEnv. The engine builds the child environment from THIS
// resolver — never from os.Environ().
//
// It returns the child environment (env), the executable (command), its
// arguments (args), and the working directory (dir).
type LaunchResolver func(projectID, accountID string) (env []string, command string, args []string, dir string, err error)
