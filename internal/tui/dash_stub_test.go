package tui

import (
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// fakeEngine is an in-memory sessionEngine for tests. It tracks sessions in a
// map and exposes a channel callers push SessionEvents onto, mirroring A's real
// engine closely enough to exercise the dashboard's Update loop.
type fakeEngine struct {
	mu       sync.Mutex
	sessions map[string]Session
	order    []string
	events   chan SessionEvent

	// call recorders so tests can assert lifecycle wiring.
	started  []SessionSpec
	stopped  []string
	killed   []string
	attached []string
	captures map[string]string

	nextID int
	closed bool
}

func newFakeEngine() *fakeEngine {
	return &fakeEngine{
		sessions: make(map[string]Session),
		events:   make(chan SessionEvent, 16),
		captures: make(map[string]string),
	}
}

func (e *fakeEngine) Start(spec SessionSpec) (Session, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.started = append(e.started, spec)
	e.nextID++
	id := fmt.Sprintf("sess-%d", e.nextID)
	s := Session{
		ID:        id,
		ProjectID: spec.ProjectID,
		AccountID: spec.AccountID,
		State:     SessionStarting,
		StartedAt: time.Now(),
	}
	e.sessions[id] = s
	e.order = append(e.order, id)
	return s, nil
}

func (e *fakeEngine) Stop(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.stopped = append(e.stopped, id)
	if s, ok := e.sessions[id]; ok {
		s.State = SessionExited
		e.sessions[id] = s
	}
	return nil
}

func (e *fakeEngine) Kill(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.killed = append(e.killed, id)
	delete(e.sessions, id)
	for i, oid := range e.order {
		if oid == id {
			e.order = append(e.order[:i], e.order[i+1:]...)
			break
		}
	}
	return nil
}

func (e *fakeEngine) List() []Session {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Session, 0, len(e.order))
	for _, id := range e.order {
		if s, ok := e.sessions[id]; ok {
			out = append(out, s)
		}
	}
	return out
}

func (e *fakeEngine) Attach(id string) (*exec.Cmd, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.attached = append(e.attached, id)
	// A harmless no-op command so tea.ExecProcess has something runnable.
	return exec.Command("cmd", "/c", "exit"), nil
}

func (e *fakeEngine) SendKeys(id, keys string) error { return nil }

func (e *fakeEngine) Capture(id string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if text, ok := e.captures[id]; ok {
		return text, nil
	}
	return "", nil
}

func (e *fakeEngine) Events() <-chan SessionEvent { return e.events }

func (e *fakeEngine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.closed {
		e.closed = true
		close(e.events)
	}
	return nil
}

// push enqueues a SessionEvent for the dashboard's subscription to read.
func (e *fakeEngine) push(ev SessionEvent) {
	e.events <- ev
}

// seed inserts a session directly (bypassing Start) for list-rendering tests.
func (e *fakeEngine) seed(s Session) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.sessions[s.ID]; !ok {
		e.order = append(e.order, s.ID)
	}
	e.sessions[s.ID] = s
}

// fakeServices builds a scripted set of bound services plus their statuses for
// rendering/probe tests.
func fakeServices() ([]Service, map[string]ServiceStatus) {
	svcs := []Service{
		{ID: "github", Label: "GitHub", Category: "vcs", Enabled: true},
		{ID: "railway", Label: "Railway", Category: "deploy", Enabled: true},
	}
	statuses := map[string]ServiceStatus{
		"github":  {ServiceID: "github", State: StatusAuthed, CheckedAt: time.Now()},
		"railway": {ServiceID: "railway", State: StatusConfigured, CheckedAt: time.Now()},
	}
	return svcs, statuses
}
