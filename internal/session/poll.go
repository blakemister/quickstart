package session

import (
	"strings"
	"time"
)

// idleThreshold is how long a running session may go without any observed
// terminal-output change before it is reclassified from running to idle.
const idleThreshold = 30 * time.Second

// startPoller launches the single background poller goroutine exactly once. It
// is safe to call from every Start; only the first call has any effect.
func (e *PsmuxEngine) startPoller() {
	e.pollerStart.Do(func() {
		// If Close already ran, do not spawn a goroutine that would block on a
		// closed events channel.
		if e.isClosed() {
			return
		}
		e.mu.Lock()
		e.pollerRunning = true
		e.mu.Unlock()
		go e.pollLoop()
	})
}

// pollLoop is the one and only poller goroutine. On each tick it reconciles
// cached session state against psmux (existence + screen contents) and emits
// the appropriate SessionEvents on the shared channel. It exits when stop is
// closed (by Close), signaling completion via done.
func (e *PsmuxEngine) pollLoop() {
	defer close(e.done)

	ticker := time.NewTicker(e.pollInterval)
	defer ticker.Stop()

	// captures caches the last-seen screen snapshot per session ID, used to
	// detect activity (a changed screen) vs idleness (an unchanged screen).
	captures := make(map[string]string)

	for {
		select {
		case <-e.stop:
			return
		case <-ticker.C:
			e.pollOnce(captures)
		}
	}
}

// pollOnce performs a single reconciliation pass. It is a method (not the
// sync.Once field) used by pollLoop.
func (e *PsmuxEngine) pollOnce(captures map[string]string) {
	// Snapshot the set of sessions we care about under the lock, then do the
	// (slow) psmux calls without holding it.
	e.mu.Lock()
	type pending struct {
		id    string
		name  string
		state SessionState
	}
	work := make([]pending, 0, len(e.sessions))
	for id, s := range e.sessions {
		work = append(work, pending{id: id, name: s.PsmuxSession, state: s.State})
	}
	e.mu.Unlock()

	if len(work) == 0 {
		return
	}

	// One ls call tells us which sessions still exist on the server.
	live := e.liveSessions()

	now := time.Now()
	for _, w := range work {
		// Sessions already marked exited need no further polling; drop their
		// cached capture so a future reused name starts fresh.
		if w.state == StateExited || w.state == StateError {
			delete(captures, w.id)
			continue
		}

		if !live[w.name] {
			e.markExited(w.id, now)
			delete(captures, w.id)
			continue
		}

		// Session is alive: capture its screen to detect activity vs idle.
		snap, err := e.captureByName(w.name)
		if err != nil {
			// Transient capture failure: leave state as-is, try again next tick.
			continue
		}

		prev, seen := captures[w.id]
		captures[w.id] = snap

		if !seen {
			// First observation: record baseline, ensure running.
			e.applyActivity(w.id, now, false)
			continue
		}

		if snap != prev {
			e.applyActivity(w.id, now, true)
		} else {
			e.applyIdleIfStale(w.id, now)
		}
	}
}

// liveSessions returns the set of psmux session names currently on the server.
// On error it returns an empty set, which is treated as "indeterminate" by the
// caller's logic only when combined with the cache — but to avoid spuriously
// declaring everything exited on a transient ls failure, we return nil and the
// caller skips the exited check when the map is nil.
func (e *PsmuxEngine) liveSessions() map[string]bool {
	ctx, cancel := e.ctx()
	defer cancel()

	out, err := e.runner.run(ctx, "ls", "-F", "#{session_name}")
	if err != nil {
		// Indeterminate: report every known session as still live this tick so a
		// transient ls failure does not mass-emit false EventExited.
		e.mu.Lock()
		live := make(map[string]bool, len(e.sessions))
		for _, s := range e.sessions {
			live[s.PsmuxSession] = true
		}
		e.mu.Unlock()
		return live
	}

	live := make(map[string]bool)
	for _, line := range strings.Split(out, "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			live[name] = true
		}
	}
	return live
}

// captureByName returns the static screen snapshot for a psmux session name.
func (e *PsmuxEngine) captureByName(name string) (string, error) {
	ctx, cancel := e.ctx()
	defer cancel()
	return e.runner.run(ctx, "capture-pane", "-p", "-t", name)
}

// markExited transitions a session to exited and emits EventExited (once).
func (e *PsmuxEngine) markExited(id string, now time.Time) {
	e.mu.Lock()
	s, ok := e.sessions[id]
	if !ok || s.State == StateExited {
		e.mu.Unlock()
		return
	}
	s.State = StateExited
	s.LastActivityAt = now
	snapshot := *s
	e.mu.Unlock()

	e.emit(SessionEvent{Kind: EventExited, Session: snapshot})
	e.emit(SessionEvent{Kind: EventStateChange, Session: snapshot})
}

// applyActivity bumps LastActivityAt and ensures the session is running. If
// changed=true it emits EventActivity; any state transition emits
// EventStateChange.
func (e *PsmuxEngine) applyActivity(id string, now time.Time, changed bool) {
	e.mu.Lock()
	s, ok := e.sessions[id]
	if !ok || s.State == StateExited || s.State == StateError {
		e.mu.Unlock()
		return
	}
	prevState := s.State
	if changed {
		s.LastActivityAt = now
	}
	s.State = StateRunning
	snapshot := *s
	transitioned := prevState != s.State
	e.mu.Unlock()

	if changed {
		e.emit(SessionEvent{Kind: EventActivity, Session: snapshot})
	}
	if transitioned {
		e.emit(SessionEvent{Kind: EventStateChange, Session: snapshot})
	}
}

// applyIdleIfStale transitions a running session to idle once it has been
// inactive for longer than idleThreshold, emitting EventStateChange on the
// transition.
func (e *PsmuxEngine) applyIdleIfStale(id string, now time.Time) {
	e.mu.Lock()
	s, ok := e.sessions[id]
	if !ok || s.State != StateRunning {
		e.mu.Unlock()
		return
	}
	if now.Sub(s.LastActivityAt) < idleThreshold {
		e.mu.Unlock()
		return
	}
	s.State = StateIdle
	snapshot := *s
	e.mu.Unlock()

	e.emit(SessionEvent{Kind: EventStateChange, Session: snapshot})
}
