package tui

import (
	"time"

	"github.com/bcmister/qs/internal/config"
	tea "github.com/charmbracelet/bubbletea"
)

// dashFocus identifies which of the three dashboard columns has focus.
type dashFocus int

const (
	projectsPane dashFocus = iota
	sessionsPane
	contextPane
)

// snapshotInterval is the slow tick cadence for refreshing the read-only
// snapshot strip.
const snapshotInterval = 1800 * time.Millisecond

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

// sessionEventMsg wraps one value read from the engine's Events() channel.
type sessionEventMsg struct {
	event SessionEvent
}

// serviceStatusMsg carries the probed status for one bound service.
type serviceStatusMsg struct {
	status ServiceStatus
}

// contextLoadedMsg carries the resolved context document for a project.
type contextLoadedMsg struct {
	projectID string
	title     string
	content   string
	docs      []string // candidate doc paths the user can cycle through
	err       error
}

// snapshotMsg carries a fresh read-only capture of the focused session.
type snapshotMsg struct {
	sessionID string
	text      string
	err       error
}

// attachDoneMsg is sent after a suspended attach (tea.ExecProcess) resumes.
type attachDoneMsg struct {
	sessionID string
	err       error
}

// dashTickMsg drives the slow snapshot refresh loop.
type dashTickMsg struct {
	at time.Time
}

// probeBatchMsg fires after the debounce window to trigger service probes.
type probeBatchMsg struct {
	projectID string
	seq       int
}

// ---------------------------------------------------------------------------
// Model
// ---------------------------------------------------------------------------

// DashboardModel is the three-column session dashboard: projects | sessions |
// context. It drives sessions through the sessionEngine interface and renders
// bound-service status plus a read-only snapshot of the focused session.
type DashboardModel struct {
	cfg    *config.Config
	keys   config.AccountKeys
	engine sessionEngine

	width  int
	height int
	focus  dashFocus

	// Projects pane
	projects   []entry
	projectCur int
	filter     string

	// Bound services for the selected project
	boundServices []Service
	serviceStatus map[string]ServiceStatus

	// Sessions pane
	sessions   []Session
	sessionCur int

	// Context pane
	viewer      *viewerModel
	contextDocs []string
	contextIdx  int

	// Read-only snapshot strip
	snapshot string

	// Status line
	statusMsg string
	statusErr bool

	// probeSeq disambiguates debounced probe batches so a stale batch from a
	// previous selection is ignored when it finally fires.
	probeSeq int

	quitting bool
	err      error
}

// NewDashboard builds a dashboard model bound to the given engine and config.
func NewDashboard(cfg *config.Config, engine sessionEngine) DashboardModel {
	keys, _ := config.LoadKeys()
	projects := scanEntries(cfg.ProjectsRoot)

	m := DashboardModel{
		cfg:           cfg,
		keys:          keys,
		engine:        engine,
		focus:         projectsPane,
		projects:      projects,
		serviceStatus: make(map[string]ServiceStatus),
	}
	if engine != nil {
		m.sessions = engine.List()
	}
	return m
}

// Init starts the long-lived event subscription and the snapshot tick, and
// loads context for the initially selected project.
func (m DashboardModel) Init() tea.Cmd {
	cmds := []tea.Cmd{m.subscribeEvents(), m.tickSnapshot()}
	if cmd := m.selectProjectCmd(); cmd != nil {
		cmds = append(cmds, cmd)
	}
	return tea.Batch(cmds...)
}

// subscribeEvents reads exactly one value from the engine's Events() channel,
// wraps it as a sessionEventMsg, and re-issues itself from Update so the loop
// is long-lived without ever blocking the UI thread.
func (m DashboardModel) subscribeEvents() tea.Cmd {
	if m.engine == nil {
		return nil
	}
	ch := m.engine.Events()
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return sessionEventMsg{event: ev}
	}
}

// tickSnapshot schedules the next slow snapshot refresh.
func (m DashboardModel) tickSnapshot() tea.Cmd {
	return tea.Tick(snapshotInterval, func(t time.Time) tea.Msg {
		return dashTickMsg{at: t}
	})
}

func (m DashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resizeViewer()
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)

	case sessionEventMsg:
		m.applySessionEvent(msg.event)
		// Re-issue the subscription so the loop continues, and refresh the
		// snapshot for the affected session.
		return m, tea.Batch(m.subscribeEvents(), m.captureCmd(msg.event.Session.ID))

	case serviceStatusMsg:
		m.serviceStatus[msg.status.ServiceID] = msg.status
		return m, nil

	case contextLoadedMsg:
		m.applyContextLoaded(msg)
		return m, nil

	case snapshotMsg:
		if msg.err == nil {
			m.snapshot = msg.text
		}
		return m, nil

	case probeBatchMsg:
		if msg.seq != m.probeSeq {
			return m, nil // stale batch from a previous selection
		}
		return m, m.probeServicesCmd(msg.projectID)

	case dashTickMsg:
		return m, tea.Batch(m.captureCmd(m.focusedSessionID()), m.tickSnapshot())

	case attachDoneMsg:
		m.err = msg.err
		if m.engine != nil {
			m.sessions = m.engine.List()
		}
		// After resuming, refresh the snapshot of the session we detached from.
		return m, m.captureCmd(msg.sessionID)
	}

	return m, nil
}

// handleKey routes key input. Wave C1 supports only quit; Wave C2 adds focus
// cycling, pane navigation, and the session lifecycle/attach keys.
func (m DashboardModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		m.quitting = true
		return m, tea.Quit
	default:
		if msg.String() == "q" {
			m.quitting = true
			return m, tea.Quit
		}
	}
	return m, nil
}

// applySessionEvent reconciles the session list with one engine event.
func (m *DashboardModel) applySessionEvent(ev SessionEvent) {
	if m.engine != nil {
		m.sessions = m.engine.List()
	} else {
		m.upsertSession(ev.Session)
	}
	if ev.Kind == EventError && ev.Err != nil {
		m.statusMsg = ev.Err.Error()
		m.statusErr = true
	}
	m.clampSessionCursor()
}

// upsertSession inserts or updates a session by ID (used when there is no
// engine List() to reconcile against, e.g. in tests).
func (m *DashboardModel) upsertSession(s Session) {
	for i := range m.sessions {
		if m.sessions[i].ID == s.ID {
			m.sessions[i] = s
			return
		}
	}
	m.sessions = append(m.sessions, s)
}

// applyContextLoaded installs a freshly loaded context document.
func (m *DashboardModel) applyContextLoaded(msg contextLoadedMsg) {
	if msg.err != nil {
		m.statusMsg = msg.err.Error()
		m.statusErr = true
		return
	}
	m.viewer = newViewerModel(msg.title, msg.content)
	m.contextDocs = msg.docs
	m.contextIdx = 0
	m.resizeViewer()
}

// clampSessionCursor keeps the session cursor within bounds.
func (m *DashboardModel) clampSessionCursor() {
	if m.sessionCur >= len(m.sessions) {
		m.sessionCur = len(m.sessions) - 1
	}
	if m.sessionCur < 0 {
		m.sessionCur = 0
	}
}

// focusedSessionID returns the ID of the currently highlighted session, or "".
func (m DashboardModel) focusedSessionID() string {
	if m.sessionCur >= 0 && m.sessionCur < len(m.sessions) {
		return m.sessions[m.sessionCur].ID
	}
	return ""
}

// selectedProjectID returns the highlighted project's name, or "".
func (m DashboardModel) selectedProjectID() string {
	if m.projectCur >= 0 && m.projectCur < len(m.projects) {
		return m.projects[m.projectCur].Name
	}
	return ""
}

// captureCmd returns a Cmd that captures a read-only snapshot of a session.
func (m DashboardModel) captureCmd(id string) tea.Cmd {
	if m.engine == nil || id == "" {
		return nil
	}
	engine := m.engine
	return func() tea.Msg {
		text, err := engine.Capture(id)
		return snapshotMsg{sessionID: id, text: text, err: err}
	}
}

// Err returns the last error from an attach handoff, if any.
func (m DashboardModel) Err() error {
	return m.err
}

// debounceInterval is the wait after a project selection before service probes
// fire, so rapid arrow-key navigation does not spawn a probe per project.
const debounceInterval = 250 * time.Millisecond

// selectProjectCmd is fired when the selected project changes. It batches (1) a
// context-document load and (2) a debounced service-probe trigger. It also
// resets boundServices/serviceStatus for the new project. Mutating state here
// is intentional: callers invoke it on a *DashboardModel.
func (m *DashboardModel) selectProjectCmd() tea.Cmd {
	projectID := m.selectedProjectID()
	if projectID == "" {
		return nil
	}
	root := ""
	if m.projectCur >= 0 && m.projectCur < len(m.projects) {
		root = m.projects[m.projectCur].Path
	}

	m.boundServices = servicesForProject(m.cfg, projectID)
	m.serviceStatus = make(map[string]ServiceStatus)
	m.probeSeq++
	seq := m.probeSeq

	return tea.Batch(
		loadContextCmd(projectID, root),
		debounceProbeCmd(projectID, seq),
	)
}

// loadContextCmd resolves a project's context document (CLAUDE.md → README.md →
// .claude/* candidates) off-thread and emits a contextLoadedMsg.
func loadContextCmd(projectID, root string) tea.Cmd {
	return func() tea.Msg {
		title, content, docs, err := resolveContextDoc(root)
		return contextLoadedMsg{
			projectID: projectID,
			title:     title,
			content:   content,
			docs:      docs,
			err:       err,
		}
	}
}

// debounceProbeCmd waits the debounce window then emits a probeBatchMsg tagged
// with seq; Update ignores it if seq is stale.
func debounceProbeCmd(projectID string, seq int) tea.Cmd {
	return tea.Tick(debounceInterval, func(time.Time) tea.Msg {
		return probeBatchMsg{projectID: projectID, seq: seq}
	})
}

// probeServicesCmd fans out one probe Cmd per bound service. Each emits a
// serviceStatusMsg as it completes.
func (m DashboardModel) probeServicesCmd(projectID string) tea.Cmd {
	if len(m.boundServices) == 0 {
		return nil
	}
	cmds := make([]tea.Cmd, 0, len(m.boundServices))
	for _, svc := range m.boundServices {
		cmds = append(cmds, probeServiceCmd(svc, projectID))
	}
	return tea.Batch(cmds...)
}

// probeServiceCmd probes a single service's status off-thread.
func probeServiceCmd(svc Service, projectID string) tea.Cmd {
	return func() tea.Msg {
		return serviceStatusMsg{status: probeServiceStatus(svc, projectID)}
	}
}

// resizeViewer sizes the embedded context viewer to the context column.
func (m *DashboardModel) resizeViewer() {
	if m.viewer == nil {
		return
	}
	colW, bodyH := m.contextColumnSize()
	m.viewer.SetSize(colW, bodyH-viewerChrome)
}
