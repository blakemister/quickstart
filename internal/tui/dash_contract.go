package tui

// TEMPORARY local mirror of the frozen cross-domain contract; integration
// replaces these with imports from internal/session and internal/config.
//
// Domain C (the dashboard TUI) is built in an isolated worktree that does not
// yet contain Domain A's internal/session package or Domain B's new config
// types. To keep go build/vet/test green, this file mirrors those frozen
// contracts with identical shapes and names so the integration wave can swap
// them for real imports mechanically (delete this file, add the imports).

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bcmister/qs/internal/config"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// Domain A — session engine contract (mirror of internal/session)
// ---------------------------------------------------------------------------

// SessionState is the lifecycle state of a managed session.
// Mirror of session.SessionState.
type SessionState string

const (
	SessionStarting SessionState = "starting"
	SessionRunning  SessionState = "running"
	SessionIdle     SessionState = "idle"
	SessionExited   SessionState = "exited"
	SessionError    SessionState = "error"
)

// SessionEventKind classifies an event emitted on the engine event channel.
// Mirror of session.SessionEventKind.
type SessionEventKind string

const (
	EventStarted     SessionEventKind = "started"
	EventStateChange SessionEventKind = "state_change"
	EventActivity    SessionEventKind = "activity"
	EventExited      SessionEventKind = "exited"
	EventError       SessionEventKind = "error"
)

// Session is one managed psmux-backed agent session.
// Mirror of session.Session.
type Session struct {
	ID             string
	ProjectID      string
	AccountID      string
	PsmuxSession   string
	State          SessionState
	StartedAt      time.Time
	LastActivityAt time.Time
	ExitCode       int
}

// SessionSpec describes a session to start.
// Mirror of session.SessionSpec.
type SessionSpec struct {
	ProjectID string
	AccountID string
}

// SessionEvent is one event delivered on the engine's Events() channel.
// Mirror of session.SessionEvent.
type SessionEvent struct {
	Kind    SessionEventKind
	Session Session
	Err     error
}

// sessionEngine is the cross-domain interface the dashboard drives.
// Mirror of session.Engine. The dashboard only ever talks to sessions
// through this interface so the integration swap is a single field-type
// change plus an import.
type sessionEngine interface {
	Start(spec SessionSpec) (Session, error)
	Stop(id string) error
	Kill(id string) error
	List() []Session
	// Attach returns a configured *exec.Cmd that the caller runs via
	// tea.ExecProcess to hand the terminal over to psmux attach.
	Attach(id string) (*exec.Cmd, error)
	SendKeys(id, keys string) error
	Capture(id string) (string, error)
	Events() <-chan SessionEvent
	Close() error
}

// ---------------------------------------------------------------------------
// Domain B — config/profile/service contract (mirror of internal/config)
// ---------------------------------------------------------------------------

// StatusState is the health of a bound service for the selected project.
// Mirror of config.StatusState (iota order is part of the contract).
type StatusState int

const (
	StatusUnknown StatusState = iota
	StatusConfigured
	StatusAuthed
	StatusReachable
	StatusError
)

// Category groups services in the dashboard's context column.
// Mirror of config.Category.
type Category string

// GitIdentity is the git author identity for a profile.
// Mirror of config.GitIdentity.
type GitIdentity struct {
	Name  string
	Email string
}

// Profile is a named identity/context applied to a project's sessions.
// Mirror of config.Profile.
type Profile struct {
	ID                string
	Label             string
	Icon              string
	Color             string
	GitIdentity       GitIdentity
	AccountConfigDirs map[string]string
	ExpectedEnv       []string
}

// Service is an external dependency whose status the dashboard probes.
// Mirror of config.Service.
type Service struct {
	ID          string
	Label       string
	Category    Category
	Icon        string
	StatusCmd   string
	RequiresEnv []string
	Enabled     bool
}

// ServiceStatus is the probed result for one Service + project.
// Mirror of config.ServiceStatus.
type ServiceStatus struct {
	ServiceID string
	State     StatusState
	Detail    string
	CheckedAt time.Time
}

// ---------------------------------------------------------------------------
// Local mirrors of picker/viewer helpers (Domain B's picker-file-viewer work,
// not present in this worktree). Integration replaces these with the real
// scanEntries/entry and viewerModel.
// ---------------------------------------------------------------------------

// entry mirrors the picker's directory entry (scanEntries result element).
type entry struct {
	Name  string
	Path  string
	IsDir bool
}

// scanEntries lists immediate subdirectories of root as project entries,
// skipping dotfiles. Mirror of the picker's scanEntries.
func scanEntries(root string) []entry {
	items, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []entry
	for _, it := range items {
		if !it.IsDir() || strings.HasPrefix(it.Name(), ".") {
			continue
		}
		out = append(out, entry{
			Name:  it.Name(),
			Path:  filepath.Join(root, it.Name()),
			IsDir: true,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// viewerModel is a minimal read-only document viewer backed by a bubbles
// viewport. Mirror of internal/tui/viewer.go's viewerModel; the integration
// wave swaps this for the real one (which adds syntax highlighting, etc.).
type viewerModel struct {
	vp    viewport.Model
	title string
	ready bool
}

// newViewerModel builds a viewer for the given title + content.
func newViewerModel(title, content string) *viewerModel {
	vp := viewport.New(0, 0)
	vp.SetContent(content)
	return &viewerModel{vp: vp, title: title, ready: true}
}

// SetSize resizes the underlying viewport.
func (v *viewerModel) SetSize(width, height int) {
	if width < 0 {
		width = 0
	}
	if height < 0 {
		height = 0
	}
	v.vp.Width = width
	v.vp.Height = height
}

// Update delegates scroll handling to the viewport.
func (v *viewerModel) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	v.vp, cmd = v.vp.Update(msg)
	return cmd
}

// View renders the viewer body.
func (v *viewerModel) View() string {
	if !v.ready {
		return ""
	}
	return v.vp.View()
}

// Title returns the viewer's title (the resolved document name).
func (v *viewerModel) Title() string {
	return v.title
}

// ---------------------------------------------------------------------------
// Local mirrors of Domain B's resolution/probe functions. Integration replaces
// these with config.ServicesForProject / config.ProbeServiceStatus and the
// real context-doc resolver. Signatures are chosen so the swap is mechanical.
// ---------------------------------------------------------------------------

// servicesForProject returns the services bound to a project. Mirror of
// config.ServicesForProject(cfg, projectID).
func servicesForProject(cfg *config.Config, projectID string) []Service {
	_ = cfg
	_ = projectID
	return nil
}

// probeServiceStatus probes one service for a project. Mirror of
// config.ProbeServiceStatus(svc, projectID). The local mirror reports Unknown
// so the dashboard renders a stable state until the real prober is wired in.
func probeServiceStatus(svc Service, projectID string) ServiceStatus {
	_ = projectID
	return ServiceStatus{
		ServiceID: svc.ID,
		State:     StatusUnknown,
		Detail:    "",
		CheckedAt: time.Now(),
	}
}

// contextCandidates is the ordered list of context-doc filenames the dashboard
// resolves for a project, mirroring the integration resolver's preference.
var contextCandidates = []string{
	"CLAUDE.md",
	"README.md",
	"AGENTS.md",
	filepath.Join(".claude", "CLAUDE.md"),
	filepath.Join(".claude", "README.md"),
}

// resolveContextDoc resolves a project's primary context document and the list
// of available candidate docs. It returns the title (filename), the file
// contents, and the candidate paths the user can cycle through with 'c'.
// Mirror of the integration context resolver.
func resolveContextDoc(root string) (title, content string, docs []string, err error) {
	if root == "" {
		return "", "", nil, nil
	}
	var found []string
	for _, name := range contextCandidates {
		p := filepath.Join(root, name)
		if info, statErr := os.Stat(p); statErr == nil && !info.IsDir() {
			found = append(found, p)
		}
	}
	if len(found) == 0 {
		return "no context", "No context document found for this project.", nil, nil
	}
	data, readErr := os.ReadFile(found[0])
	if readErr != nil {
		return "", "", found, readErr
	}
	return filepath.Base(found[0]), string(data), found, nil
}
