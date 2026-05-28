// Package session defines the abstraction for managing persistent
// AI-coding sessions in qs.
//
// A "session" is one long-running AI coding tool (Claude Code, Codex, etc.)
// bound to a single project + account. Unlike the legacy launcher — which
// spawns a Windows Terminal window and forgets about it — a SessionEngine
// owns the child process lifecycle: it can start, stop, kill, enumerate,
// attach to, drive (SendKeys), and snapshot (Capture) sessions, and it emits
// a stream of SessionEvents describing state transitions.
//
// # Rendering ownership
//
// The terminal multiplexer (psmux) owns rendering. This package deliberately
// does NOT embed a live terminal pane and NEVER calls tea.ExecProcess itself:
//
//   - Capture returns a STATIC, point-in-time text snapshot of a session's
//     screen. It is suitable for previews/thumbnails in the dashboard TUI but
//     is not a live feed.
//   - Attach returns a prepared *exec.Cmd that the CALLER runs (via
//     tea.ExecProcess) to take over the terminal and interact with the live
//     session. The engine constructs the command but never runs it; control
//     of stdin/stdout/stderr and the foreground terminal belongs to the
//     caller, not the engine.
//
// Keeping rendering out of the engine lets the dashboard remain a normal Bubble
// Tea program: it polls Capture for previews and hands the user off to a live
// pane via Attach when they select a session.
//
// # Implementations
//
// v1 ships PsmuxEngine, which wraps the external psmux multiplexer via its CLI
// (start/stop/list/send-keys/capture-pane/attach). InProcessEngine is the
// fallback: it manages child processes directly within the qs process when
// psmux is unavailable. ConPtyEngine is a reserved future v2 slot that would
// drive child processes through the Windows ConPTY API for richer in-process
// rendering. All implementations satisfy the frozen SessionEngine interface
// declared in engine.go.
//
// Child process environment is always resolved through a LaunchResolver
// (backed by config.ResolveSessionEnv via an adapter in cmd/dash.go); engines
// never read os.Environ() directly.
package session
