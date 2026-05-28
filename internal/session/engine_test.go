package session

import (
	"fmt"
	"os/exec"
	"sync"
	"testing"
	"time"
)

// stubEngine is an in-memory reference implementation of SessionEngine used to
// prove the interface is satisfiable and to give downstream consumers a fake
// shape to test against. It performs no real process management.
type stubEngine struct {
	mu       sync.Mutex
	sessions map[string]Session
	nextID   int
	events   chan SessionEvent
	closed   bool
}

// compile-time assertion that stubEngine satisfies the frozen contract.
var _ SessionEngine = (*stubEngine)(nil)

func newStubEngine() *stubEngine {
	return &stubEngine{
		sessions: make(map[string]Session),
		events:   make(chan SessionEvent, 16),
	}
}

func (e *stubEngine) Start(spec SessionSpec) (Session, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return Session{}, fmt.Errorf("start %s/%s: engine closed", spec.ProjectID, spec.AccountID)
	}
	e.nextID++
	now := time.Now()
	s := Session{
		ID:             fmt.Sprintf("sess-%d", e.nextID),
		ProjectID:      spec.ProjectID,
		AccountID:      spec.AccountID,
		PsmuxSession:   fmt.Sprintf("psmux-%d", e.nextID),
		State:          StateRunning,
		StartedAt:      now,
		LastActivityAt: now,
	}
	e.sessions[s.ID] = s
	e.emit(SessionEvent{Kind: EventStarted, Session: s})
	return s, nil
}

func (e *stubEngine) Stop(id string) error {
	return e.terminate(id, StateExited)
}

func (e *stubEngine) Kill(id string) error {
	return e.terminate(id, StateExited)
}

func (e *stubEngine) terminate(id string, state SessionState) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	s, ok := e.sessions[id]
	if !ok {
		return fmt.Errorf("terminate %s: unknown session", id)
	}
	s.State = state
	e.sessions[id] = s
	e.emit(SessionEvent{Kind: EventExited, Session: s})
	return nil
}

func (e *stubEngine) List() []Session {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]Session, 0, len(e.sessions))
	for _, s := range e.sessions {
		out = append(out, s)
	}
	return out
}

func (e *stubEngine) Attach(id string) (*exec.Cmd, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.sessions[id]; !ok {
		return nil, fmt.Errorf("attach %s: unknown session", id)
	}
	// The caller is responsible for running this command via tea.ExecProcess.
	return exec.Command("psmux", "attach", id), nil
}

func (e *stubEngine) SendKeys(id string, keys string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.sessions[id]; !ok {
		return fmt.Errorf("send-keys %s: unknown session", id)
	}
	return nil
}

func (e *stubEngine) Capture(id string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.sessions[id]; !ok {
		return "", fmt.Errorf("capture %s: unknown session", id)
	}
	return fmt.Sprintf("snapshot of %s", id), nil
}

func (e *stubEngine) Events() <-chan SessionEvent {
	return e.events
}

func (e *stubEngine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	e.closed = true
	close(e.events)
	return nil
}

// emit publishes an event without blocking; callers must hold e.mu.
func (e *stubEngine) emit(ev SessionEvent) {
	select {
	case e.events <- ev:
	default:
	}
}

func TestStubStartListRoundTrip(t *testing.T) {
	e := newStubEngine()
	defer e.Close()

	s, err := e.Start(SessionSpec{ProjectID: "proj", AccountID: "claude"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if s.ID == "" {
		t.Fatal("Start returned empty ID")
	}
	if s.ProjectID != "proj" || s.AccountID != "claude" {
		t.Fatalf("Start did not preserve spec: %+v", s)
	}
	if s.State != StateRunning {
		t.Fatalf("expected StateRunning, got %q", s.State)
	}

	list := e.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 session, got %d", len(list))
	}
	if list[0].ID != s.ID {
		t.Fatalf("List returned wrong session: got %q want %q", list[0].ID, s.ID)
	}
}

func TestStubStopRoundTrip(t *testing.T) {
	e := newStubEngine()
	defer e.Close()

	s, err := e.Start(SessionSpec{ProjectID: "p", AccountID: "a"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := e.Stop(s.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	for _, got := range e.List() {
		if got.ID == s.ID && got.State != StateExited {
			t.Fatalf("expected StateExited after Stop, got %q", got.State)
		}
	}

	if err := e.Stop("nope"); err == nil {
		t.Fatal("expected error stopping unknown session")
	}
}

func TestStubAttachAndCapture(t *testing.T) {
	e := newStubEngine()
	defer e.Close()

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

	snap, err := e.Capture(s.ID)
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if snap == "" {
		t.Fatal("Capture returned empty snapshot")
	}

	if err := e.SendKeys(s.ID, "ls\n"); err != nil {
		t.Fatalf("SendKeys: %v", err)
	}
}

func TestStubEventsRoundTrip(t *testing.T) {
	e := newStubEngine()

	s, err := e.Start(SessionSpec{ProjectID: "p", AccountID: "a"})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}

	select {
	case ev := <-e.Events():
		if ev.Kind != EventStarted {
			t.Fatalf("expected EventStarted, got %q", ev.Kind)
		}
		if ev.Session.ID != s.ID {
			t.Fatalf("event for wrong session: %q", ev.Session.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for started event")
	}

	if err := e.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// After Close the events channel must be drained and closed.
	for range e.Events() {
	}
}
