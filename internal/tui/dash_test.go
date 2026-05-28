package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bcmister/qs/internal/config"
	"github.com/bcmister/qs/internal/session"
	tea "github.com/charmbracelet/bubbletea"
)

// newTestDashboard builds a dashboard over a temp projects root containing the
// given project folders, wired to a fresh fakeEngine.
func newTestDashboard(t *testing.T, projects ...string) (DashboardModel, *fakeEngine, string) {
	t.Helper()
	root := t.TempDir()
	for _, p := range projects {
		if err := os.MkdirAll(filepath.Join(root, p), 0755); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
	}
	cfg := &config.Config{ProjectsRoot: root, DefaultAccount: "claude"}
	eng := newFakeEngine()
	m := NewDashboard(cfg, eng)
	return m, eng, root
}

// drainBatch flattens a Cmd that may be a tea.Batch into the messages each of
// its sub-Cmds produces. Tick-based Cmds are skipped (they would block).
func drainBatch(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		return []tea.Msg{msg}
	}
	var out []tea.Msg
	for _, c := range batch {
		if c == nil {
			continue
		}
		out = append(out, c())
	}
	return out
}

func keyMsg(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }

func runeKey(r rune) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}} }

func TestDashboardFocusCycling(t *testing.T) {
	m, _, _ := newTestDashboard(t, "alpha", "beta")

	if m.focus != projectsPane {
		t.Fatalf("initial focus = %v, want projectsPane", m.focus)
	}

	steps := []dashFocus{sessionsPane, contextPane, projectsPane}
	model := tea.Model(m)
	for i, want := range steps {
		model, _ = model.Update(keyMsg(tea.KeyTab))
		if got := model.(DashboardModel).focus; got != want {
			t.Fatalf("after %d tabs focus = %v, want %v", i+1, got, want)
		}
	}

	// shift+tab wraps backward to contextPane.
	model, _ = model.Update(keyMsg(tea.KeyShiftTab))
	if got := model.(DashboardModel).focus; got != contextPane {
		t.Fatalf("after shift+tab focus = %v, want contextPane", got)
	}
}

func TestDashboardProjectNavigation(t *testing.T) {
	m, _, _ := newTestDashboard(t, "alpha", "beta", "gamma")

	if got := m.selectedProjectID(); got != "alpha" {
		t.Fatalf("initial project = %q, want alpha", got)
	}

	model, _ := m.Update(keyMsg(tea.KeyDown))
	if got := model.(DashboardModel).selectedProjectID(); got != "beta" {
		t.Fatalf("after down project = %q, want beta", got)
	}

	model, _ = model.Update(keyMsg(tea.KeyDown))
	model, _ = model.Update(keyMsg(tea.KeyDown)) // clamp at gamma
	if got := model.(DashboardModel).selectedProjectID(); got != "gamma" {
		t.Fatalf("after 3 downs project = %q, want gamma", got)
	}

	model, _ = model.Update(keyMsg(tea.KeyUp))
	if got := model.(DashboardModel).selectedProjectID(); got != "beta" {
		t.Fatalf("after up project = %q, want beta", got)
	}
}

func TestDashboardSelectProjectTriggersContextAndProbe(t *testing.T) {
	m, _, root := newTestDashboard(t, "alpha")
	// Give alpha a context doc so the load yields its content.
	if err := os.WriteFile(filepath.Join(root, "alpha", "CLAUDE.md"), []byte("# Alpha"), 0644); err != nil {
		t.Fatal(err)
	}
	// Bind a service so the probe batch has something to probe.
	m.boundServices = []config.Service{{ID: "github", Label: "GitHub", Enabled: true}}

	cmd := m.selectProjectCmd()
	if cmd == nil {
		t.Fatal("selectProjectCmd returned nil")
	}
	msgs := drainBatch(cmd)

	var sawContext, sawProbe bool
	for _, msg := range msgs {
		switch v := msg.(type) {
		case contextLoadedMsg:
			sawContext = true
			if len(v.docs) == 0 || filepath.Base(v.docs[0]) != "CLAUDE.md" {
				t.Errorf("context docs = %v, want CLAUDE.md first", v.docs)
			}
		case probeBatchMsg:
			sawProbe = true
			if v.seq != m.probeSeq {
				t.Errorf("probe seq = %d, want %d", v.seq, m.probeSeq)
			}
		}
	}
	if !sawContext {
		t.Error("expected a contextLoadedMsg from selectProjectCmd")
	}
	if !sawProbe {
		t.Error("expected a debounced probeBatchMsg from selectProjectCmd")
	}
}

func TestDashboardProbeBatchEmitsServiceStatus(t *testing.T) {
	m, _, _ := newTestDashboard(t, "alpha")
	svcs, _ := fakeServices()
	m.boundServices = svcs

	// A matching seq should fan out probe Cmds.
	model, cmd := m.Update(probeBatchMsg{projectID: "alpha", seq: m.probeSeq})
	_ = model
	msgs := drainBatch(cmd)
	if len(msgs) != len(svcs) {
		t.Fatalf("got %d status msgs, want %d", len(msgs), len(svcs))
	}
	for _, msg := range msgs {
		if _, ok := msg.(serviceStatusMsg); !ok {
			t.Fatalf("expected serviceStatusMsg, got %T", msg)
		}
	}

	// A stale seq should be ignored.
	_, cmd = m.Update(probeBatchMsg{projectID: "alpha", seq: m.probeSeq + 99})
	if cmd != nil {
		t.Fatal("stale probe batch should produce no Cmd")
	}
}

func TestDashboardServiceStatusMsgUpdatesMap(t *testing.T) {
	m, _, _ := newTestDashboard(t, "alpha")
	st := config.ServiceStatus{ServiceID: "github", State: config.StatusAuthed, CheckedAt: time.Now()}
	model, _ := m.Update(serviceStatusMsg{status: st})
	got := model.(DashboardModel).serviceStatus["github"]
	if got.State != config.StatusAuthed {
		t.Fatalf("status state = %v, want StatusAuthed", got.State)
	}
}

func TestDashboardSessionEventUpdatesList(t *testing.T) {
	m, eng, _ := newTestDashboard(t, "alpha")
	if len(m.sessions) != 0 {
		t.Fatalf("expected 0 sessions initially, got %d", len(m.sessions))
	}

	// Seed the engine so List() reflects the new session, then push the event.
	s := session.Session{ID: "sess-1", ProjectID: "alpha", AccountID: "claude", State: session.StateRunning}
	eng.seed(s)

	model, _ := m.Update(sessionEventMsg{event: session.SessionEvent{Kind: session.EventStarted, Session: s}})
	sessions := model.(DashboardModel).sessions
	if len(sessions) != 1 || sessions[0].ID != "sess-1" {
		t.Fatalf("expected session sess-1 in list, got %+v", sessions)
	}
}

func TestDashboardSessionEventReissuesSubscription(t *testing.T) {
	m, eng, _ := newTestDashboard(t, "alpha")
	s := session.Session{ID: "sess-1", AccountID: "claude", State: session.StateRunning}
	eng.seed(s)

	_, cmd := m.Update(sessionEventMsg{event: session.SessionEvent{Kind: session.EventStarted, Session: s}})
	if cmd == nil {
		t.Fatal("expected a Cmd (re-subscription + capture) after a session event")
	}
	// Pushing another event should let the re-issued subscription resolve.
	eng.push(session.SessionEvent{Kind: session.EventActivity, Session: s})
	msgs := drainBatch(cmd)
	var sawEvent bool
	for _, msg := range msgs {
		if _, ok := msg.(sessionEventMsg); ok {
			sawEvent = true
		}
	}
	if !sawEvent {
		t.Error("re-issued subscription did not deliver the next sessionEventMsg")
	}
}

func TestDashboardStartIssuesEngineStart(t *testing.T) {
	m, eng, _ := newTestDashboard(t, "alpha")
	m.focus = projectsPane

	_, cmd := m.Update(runeKey('s'))
	if cmd == nil {
		t.Fatal("'s' produced no Cmd")
	}
	msg := cmd()
	ev, ok := msg.(sessionEventMsg)
	if !ok {
		t.Fatalf("expected sessionEventMsg, got %T", msg)
	}
	if ev.event.Kind != session.EventStarted {
		t.Fatalf("event kind = %v, want started", ev.event.Kind)
	}
	if len(eng.started) != 1 || eng.started[0].ProjectID != "alpha" {
		t.Fatalf("engine.Start not called for alpha: %+v", eng.started)
	}
}

func TestDashboardStopAndKill(t *testing.T) {
	m, eng, _ := newTestDashboard(t, "alpha")
	s := session.Session{ID: "sess-1", AccountID: "claude", State: session.StateRunning}
	eng.seed(s)
	m.sessions = eng.List()
	m.focus = sessionsPane
	m.sessionCur = 0

	if _, cmd := m.Update(runeKey('x')); cmd != nil {
		cmd()
	}
	if len(eng.stopped) != 1 || eng.stopped[0] != "sess-1" {
		t.Fatalf("engine.Stop not called: %+v", eng.stopped)
	}

	if _, cmd := m.Update(runeKey('k')); cmd != nil {
		cmd()
	}
	if len(eng.killed) != 1 || eng.killed[0] != "sess-1" {
		t.Fatalf("engine.Kill not called: %+v", eng.killed)
	}
}

func TestDashboardEnterIssuesAttachCmd(t *testing.T) {
	m, eng, _ := newTestDashboard(t, "alpha")
	s := session.Session{ID: "sess-1", AccountID: "claude", State: session.StateRunning}
	eng.seed(s)
	m.sessions = eng.List()
	m.focus = sessionsPane
	m.sessionCur = 0

	_, cmd := m.Update(keyMsg(tea.KeyEnter))
	if cmd == nil {
		t.Fatal("Enter produced no attach Cmd")
	}
	if len(eng.attached) != 1 || eng.attached[0] != "sess-1" {
		t.Fatalf("engine.Attach not called for sess-1: %+v", eng.attached)
	}
}

func TestDashboardContextCycle(t *testing.T) {
	m, _, root := newTestDashboard(t, "alpha")
	a := filepath.Join(root, "alpha")
	if err := os.WriteFile(filepath.Join(a, "CLAUDE.md"), []byte("# claude"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(a, "README.md"), []byte("# readme"), 0644); err != nil {
		t.Fatal(err)
	}

	// Resolve alpha's candidate docs synchronously.
	docs := resolveContextDocs(a)
	if len(docs) < 2 {
		t.Fatalf("expected at least 2 candidate docs, got %v", docs)
	}
	model, _ := m.Update(contextLoadedMsg{projectID: "alpha", docs: docs})
	dm := model.(DashboardModel)
	if dm.viewer == nil || dm.contextTitle != "CLAUDE.md" {
		t.Fatalf("expected context titled CLAUDE.md, got %q (viewer=%v)", dm.contextTitle, dm.viewer)
	}

	// 'c' cycles to the next candidate (README.md).
	model, _ = dm.Update(runeKey('c'))
	dm = model.(DashboardModel)
	if dm.contextTitle != "README.md" {
		t.Fatalf("after cycle context title = %q, want README.md", dm.contextTitle)
	}
}

func TestDashboardQuit(t *testing.T) {
	m, _, _ := newTestDashboard(t, "alpha")
	model, cmd := m.Update(runeKey('q'))
	if !model.(DashboardModel).quitting {
		t.Fatal("'q' did not set quitting")
	}
	if cmd == nil {
		t.Fatal("'q' did not return tea.Quit")
	}
}

func TestDashboardViewRendersColumns(t *testing.T) {
	m, _, _ := newTestDashboard(t, "alpha", "beta")
	m.width = 120
	m.height = 30
	out := m.View()
	for _, want := range []string{"Projects", "Sessions", "Context", "alpha"} {
		if !strings.Contains(out, want) {
			t.Errorf("view missing %q", want)
		}
	}
}
