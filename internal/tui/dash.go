package tui

import (
	"os"
	"path/filepath"
	"time"

	"github.com/bcmister/qs/internal/config"
	"github.com/bcmister/qs/internal/session"
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

// contextCandidates is the ordered list of context-doc filenames the dashboard
// resolves for a project. The first that exists becomes the primary document;
// the rest are cyclable with 'c'.
var contextCandidates = []string{
	"CLAUDE.md",
	"README.md",
	"AGENTS.md",
	filepath.Join(".claude", "CLAUDE.md"),
	filepath.Join(".claude", "README.md"),
}

// resolveContextDocs returns the candidate context-document PATHS that exist for
// a project root, in preference order. The viewer reads the file itself, so this
// only resolves paths (never contents). Returns nil when none exist.
func resolveContextDocs(root string) []string {
	if root == "" {
		return nil
	}
	var found []string
	for _, name := range contextCandidates {
		p := filepath.Join(root, name)
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			found = append(found, p)
		}
	}
	return found
}

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

// sessionEventMsg wraps one value read from the engine's Events() channel.
type sessionEventMsg struct {
	event session.SessionEvent
}

// serviceStatusMsg carries the probed status for one bound service.
type serviceStatusMsg struct {
	status config.ServiceStatus
}

// contextLoadedMsg carries the resolved context-document paths for a project.
type contextLoadedMsg struct {
	projectID string
	docs      []string // candidate doc paths the user can cycle through
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
// context. It drives sessions through the session.SessionEngine interface and
// renders bound-service status plus a read-only snapshot of the focused session.
type DashboardModel struct {
	cfg    *config.Config
	keys   config.AccountKeys
	engine session.SessionEngine

	width  int
	height int
	focus  dashFocus

	// Projects pane
	projects   []entry
	projectCur int
	filter     string

	// Bound services for the selected project
	boundServices []config.Service
	serviceStatus map[string]config.ServiceStatus

	// Sessions pane
	sessions   []session.Session
	sessionCur int

	// Context pane
	viewer       *viewerModel
	contextDocs  []string
	contextIdx   int
	contextTitle string // filepath.Base of the current doc (viewer has no Title())

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
func NewDashboard(cfg *config.Config, engine session.SessionEngine) DashboardModel {
	keys, _ := config.LoadKeys()
	projects := projectEntries(cfg.ProjectsRoot)

	m := DashboardModel{
		cfg:           cfg,
		keys:          keys,
		engine:        engine,
		focus:         projectsPane,
		projects:      projects,
		serviceStatus: make(map[string]config.ServiceStatus),
	}
	if engine != nil {
		m.sessions = engine.List()
	}
	return m
}

// projectEntries lists only the directory entries under root (project folders),
// dropping files the shared scanEntries also returns.
func projectEntries(root string) []entry {
	all := scanEntries(root)
	out := all[:0]
	for _, e := range all {
		if e.isDir {
			out = append(out, e)
		}
	}
	return out
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

// handleKey routes key input: focus cycling, pane navigation, the session
// lifecycle keys (s/x/k), Enter-attach, refresh, context cycling, and quit.
func (m DashboardModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyCtrlC:
		m.quitting = true
		return m, tea.Quit
	case tea.KeyTab:
		m.cycleFocus(1)
		return m, nil
	case tea.KeyShiftTab:
		m.cycleFocus(-1)
		return m, nil
	case tea.KeyUp:
		return m.navUp()
	case tea.KeyDown:
		return m.navDown()
	case tea.KeyEnter:
		return m.attachSelected()
	}

	switch msg.String() {
	case "q":
		m.quitting = true
		return m, tea.Quit
	case "s":
		return m, m.startSelected()
	case "x":
		return m, m.stopSelected()
	case "k":
		return m, m.killSelected()
	case "r":
		return m.refresh()
	case "c":
		m.cycleContext()
		return m, nil
	}

	// When the context pane is focused, delegate other keys (page up/down,
	// home/end, etc.) to the embedded viewer for scrolling.
	if m.focus == contextPane && m.viewer != nil {
		cmd := m.viewer.Update(msg)
		return m, cmd
	}
	return m, nil
}

// cycleFocus moves focus forward (dir=1) or backward (dir=-1) across the three
// panes, wrapping around.
func (m *DashboardModel) cycleFocus(dir int) {
	const paneCount = 3
	m.focus = dashFocus((int(m.focus) + dir + paneCount) % paneCount)
}

// navUp moves the cursor up in the focused pane (or scrolls the viewer up).
func (m DashboardModel) navUp() (tea.Model, tea.Cmd) {
	switch m.focus {
	case projectsPane:
		if m.projectCur > 0 {
			m.projectCur--
			return m, m.selectProjectCmd()
		}
	case sessionsPane:
		if m.sessionCur > 0 {
			m.sessionCur--
		}
	case contextPane:
		if m.viewer != nil {
			return m, m.viewer.Update(tea.KeyMsg{Type: tea.KeyUp})
		}
	}
	return m, nil
}

// navDown moves the cursor down in the focused pane (or scrolls the viewer).
func (m DashboardModel) navDown() (tea.Model, tea.Cmd) {
	switch m.focus {
	case projectsPane:
		if m.projectCur < len(m.projects)-1 {
			m.projectCur++
			return m, m.selectProjectCmd()
		}
	case sessionsPane:
		if m.sessionCur < len(m.sessions)-1 {
			m.sessionCur++
		}
	case contextPane:
		if m.viewer != nil {
			return m, m.viewer.Update(tea.KeyMsg{Type: tea.KeyDown})
		}
	}
	return m, nil
}

// startSelected starts a session for the selected project using the configured
// default account.
func (m *DashboardModel) startSelected() tea.Cmd {
	if m.engine == nil {
		return nil
	}
	projectID := m.selectedProjectID()
	if projectID == "" {
		return nil
	}
	accountID := m.cfg.DefaultAccount
	if accountID == "" {
		accountID = m.cfg.LastAccount
	}
	engine := m.engine
	spec := session.SessionSpec{ProjectID: projectID, AccountID: accountID}
	return func() tea.Msg {
		s, err := engine.Start(spec)
		if err != nil {
			return sessionEventMsg{event: session.SessionEvent{Kind: session.EventError, Err: err}}
		}
		return sessionEventMsg{event: session.SessionEvent{Kind: session.EventStarted, Session: s}}
	}
}

// stopSelected stops the focused session.
func (m *DashboardModel) stopSelected() tea.Cmd {
	return m.lifecycleCmd(func(id string) error { return m.engine.Stop(id) }, session.EventStateChange)
}

// killSelected kills the focused session.
func (m *DashboardModel) killSelected() tea.Cmd {
	return m.lifecycleCmd(func(id string) error { return m.engine.Kill(id) }, session.EventExited)
}

// lifecycleCmd runs a stop/kill action on the focused session off-thread and
// reports the result as a sessionEventMsg so the list reconciles.
func (m *DashboardModel) lifecycleCmd(action func(string) error, kind session.SessionEventKind) tea.Cmd {
	if m.engine == nil {
		return nil
	}
	id := m.focusedSessionID()
	if id == "" {
		return nil
	}
	return func() tea.Msg {
		if err := action(id); err != nil {
			return sessionEventMsg{event: session.SessionEvent{Kind: session.EventError, Session: session.Session{ID: id}, Err: err}}
		}
		return sessionEventMsg{event: session.SessionEvent{Kind: kind, Session: session.Session{ID: id}}}
	}
}

// attachSelected hands the terminal over to the focused session via psmux
// attach. The dashboard SUSPENDS (tea.ExecProcess) and RESUMES afterward — it
// does NOT quit.
func (m DashboardModel) attachSelected() (tea.Model, tea.Cmd) {
	if m.engine == nil {
		return m, nil
	}
	id := m.focusedSessionID()
	if id == "" {
		return m, nil
	}
	cmd, err := m.engine.Attach(id)
	if err != nil {
		m.statusMsg = err.Error()
		m.statusErr = true
		return m, nil
	}
	return m, tea.ExecProcess(cmd, func(execErr error) tea.Msg {
		return attachDoneMsg{sessionID: id, err: execErr}
	})
}

// refresh re-scans projects and re-reads the session list from the engine.
func (m DashboardModel) refresh() (tea.Model, tea.Cmd) {
	m.projects = projectEntries(m.cfg.ProjectsRoot)
	if m.projectCur >= len(m.projects) {
		m.projectCur = len(m.projects) - 1
	}
	if m.projectCur < 0 {
		m.projectCur = 0
	}
	if m.engine != nil {
		m.sessions = m.engine.List()
	}
	m.clampSessionCursor()
	return m, m.selectProjectCmd()
}

// cycleContext advances to the next candidate context document and loads it.
func (m *DashboardModel) cycleContext() {
	if len(m.contextDocs) < 2 {
		return
	}
	m.contextIdx = (m.contextIdx + 1) % len(m.contextDocs)
	m.loadCurrentDoc()
}

// loadCurrentDoc builds the viewer for the doc at contextIdx and tracks its
// title. The viewer reads the file itself.
func (m *DashboardModel) loadCurrentDoc() {
	if m.contextIdx < 0 || m.contextIdx >= len(m.contextDocs) {
		m.viewer = nil
		m.contextTitle = ""
		return
	}
	path := m.contextDocs[m.contextIdx]
	colW, bodyH := m.contextColumnSize()
	m.viewer = newViewer(path, colW, bodyH-viewerChrome)
	m.contextTitle = filepath.Base(path)
}

// applySessionEvent reconciles the session list with one engine event.
func (m *DashboardModel) applySessionEvent(ev session.SessionEvent) {
	if m.engine != nil {
		m.sessions = m.engine.List()
	} else {
		m.upsertSession(ev.Session)
	}
	if ev.Kind == session.EventError && ev.Err != nil {
		m.statusMsg = ev.Err.Error()
		m.statusErr = true
	}
	m.clampSessionCursor()
}

// upsertSession inserts or updates a session by ID (used when there is no
// engine List() to reconcile against, e.g. in tests).
func (m *DashboardModel) upsertSession(s session.Session) {
	for i := range m.sessions {
		if m.sessions[i].ID == s.ID {
			m.sessions[i] = s
			return
		}
	}
	m.sessions = append(m.sessions, s)
}

// applyContextLoaded installs the freshly resolved context-document paths and
// loads the primary one into the viewer.
func (m *DashboardModel) applyContextLoaded(msg contextLoadedMsg) {
	m.contextDocs = msg.docs
	m.contextIdx = 0
	if len(m.contextDocs) == 0 {
		m.viewer = nil
		m.contextTitle = ""
		return
	}
	m.loadCurrentDoc()
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
		return m.projects[m.projectCur].name
	}
	return ""
}

// selectedProjectPath returns the filesystem path of the highlighted project,
// joining the project name onto the configured projects root.
func (m DashboardModel) selectedProjectPath() string {
	if m.projectCur >= 0 && m.projectCur < len(m.projects) {
		return filepath.Join(m.cfg.ProjectsRoot, m.projects[m.projectCur].name)
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
// context-document resolution and (2) a debounced service-probe trigger. It also
// resets boundServices/serviceStatus for the new project. Mutating state here
// is intentional: callers invoke it on a *DashboardModel.
func (m *DashboardModel) selectProjectCmd() tea.Cmd {
	projectID := m.selectedProjectID()
	if projectID == "" {
		return nil
	}
	root := m.selectedProjectPath()

	m.boundServices = config.ServicesForProject(m.cfg, projectID)
	m.serviceStatus = make(map[string]config.ServiceStatus)
	m.probeSeq++
	seq := m.probeSeq

	return tea.Batch(
		loadContextCmd(projectID, root),
		debounceProbeCmd(projectID, seq),
	)
}

// loadContextCmd resolves a project's candidate context documents (CLAUDE.md →
// README.md → .claude/* candidates) off-thread and emits a contextLoadedMsg.
func loadContextCmd(projectID, root string) tea.Cmd {
	return func() tea.Msg {
		return contextLoadedMsg{
			projectID: projectID,
			docs:      resolveContextDocs(root),
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
		cmds = append(cmds, probeServiceCmd(m.cfg, m.keys, svc, projectID))
	}
	return tea.Batch(cmds...)
}

// probeServiceCmd probes a single service's status off-thread. It resolves the
// project's firewall env first and probes the service against it (never
// os.Environ()); an env-resolution failure yields a StatusError result.
func probeServiceCmd(cfg *config.Config, keys config.AccountKeys, svc config.Service, projectID string) tea.Cmd {
	return func() tea.Msg {
		env, err := config.ResolveProfileEnv(cfg, keys, projectID)
		if err != nil {
			return serviceStatusMsg{status: config.ServiceStatus{
				ServiceID: svc.ID,
				State:     config.StatusError,
				Detail:    err.Error(),
				CheckedAt: time.Now(),
			}}
		}
		return serviceStatusMsg{status: config.ProbeServiceStatus(svc, env)}
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
