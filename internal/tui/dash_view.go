package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Layout constants for the dashboard view.
const (
	// viewerChrome is the number of body rows the context column spends on its
	// own header/footer chrome (title line + snapshot strip region), leaving the
	// remainder for the embedded viewer body.
	viewerChrome = 4

	// dashHeaderRows / dashFooterRows reserve vertical space outside the three
	// column bodies.
	dashHeaderRows = 2
	dashFooterRows = 2

	// minColWidth keeps each column legible on narrow terminals.
	minColWidth = 16
)

// columnWidths splits the available width across the three columns.
// Projects and sessions get a quarter each; context takes the remainder.
func (m DashboardModel) columnWidths() (projW, sessW, ctxW int) {
	w := m.width
	if w <= 0 {
		w = 80
	}
	projW = w / 4
	sessW = w / 4
	if projW < minColWidth {
		projW = minColWidth
	}
	if sessW < minColWidth {
		sessW = minColWidth
	}
	ctxW = w - projW - sessW
	if ctxW < minColWidth {
		ctxW = minColWidth
	}
	return projW, sessW, ctxW
}

// bodyHeight is the vertical space available for column bodies.
func (m DashboardModel) bodyHeight() int {
	h := m.height
	if h <= 0 {
		h = 24
	}
	body := h - dashHeaderRows - dashFooterRows
	if body < 1 {
		body = 1
	}
	return body
}

// contextColumnSize reports the context column's inner width and body height
// (used to size the embedded viewer).
func (m DashboardModel) contextColumnSize() (width, height int) {
	_, _, ctxW := m.columnWidths()
	return ctxW, m.bodyHeight()
}

func (m DashboardModel) View() string {
	if m.quitting {
		return ""
	}

	projW, sessW, ctxW := m.columnWidths()
	bodyH := m.bodyHeight()

	header := m.renderHeader()
	projCol := m.renderProjectsColumn(projW, bodyH)
	sessCol := m.renderSessionsColumn(sessW, bodyH)
	ctxCol := m.renderContextColumn(ctxW, bodyH)

	columns := lipgloss.JoinHorizontal(lipgloss.Top, projCol, sessCol, ctxCol)
	footer := m.renderFooter()

	return lipgloss.JoinVertical(lipgloss.Left, header, columns, footer)
}

func (m DashboardModel) renderHeader() string {
	title := TitleStyle.Render("qs dashboard")
	hint := DimStyle.Render("tab: focus  enter: attach  q: quit")
	return fmt.Sprintf(" %s   %s", title, hint)
}

func (m DashboardModel) renderProjectsColumn(w, h int) string {
	focused := m.focus == projectsPane
	var b strings.Builder
	b.WriteString(columnTitle("Projects", focused))
	b.WriteString("\n")

	rows := h - 1
	if rows < 1 {
		rows = 1
	}
	for i := 0; i < rows && i < len(m.projects); i++ {
		name := m.projects[i].Name
		if i == m.projectCur {
			b.WriteString(selectedRow(name, focused))
		} else {
			b.WriteString("  " + NormalStyle.Render(name))
		}
		b.WriteString("\n")
	}
	if len(m.projects) == 0 {
		b.WriteString("  " + DimStyle.Render("no projects"))
	}
	return columnBox(b.String(), w, h, focused)
}

func (m DashboardModel) renderSessionsColumn(w, h int) string {
	focused := m.focus == sessionsPane
	var b strings.Builder
	b.WriteString(columnTitle("Sessions", focused))
	b.WriteString("\n")

	rows := h - 1
	if rows < 1 {
		rows = 1
	}
	for i := 0; i < rows && i < len(m.sessions); i++ {
		s := m.sessions[i]
		line := fmt.Sprintf("%s %s", sessionStateDot(s.State), s.AccountID)
		if i == m.sessionCur {
			b.WriteString(selectedRow(line, focused))
		} else {
			b.WriteString("  " + NormalStyle.Render(line))
		}
		b.WriteString("\n")
	}
	if len(m.sessions) == 0 {
		b.WriteString("  " + DimStyle.Render("no sessions"))
	}
	return columnBox(b.String(), w, h, focused)
}

func (m DashboardModel) renderContextColumn(w, h int) string {
	focused := m.focus == contextPane
	var b strings.Builder

	docTitle := "Context"
	if m.viewer != nil && m.viewer.Title() != "" {
		docTitle = "Context: " + m.viewer.Title()
	}
	b.WriteString(columnTitle(docTitle, focused))
	b.WriteString("\n")

	// Bound-service status dots.
	if len(m.boundServices) > 0 {
		b.WriteString(m.renderServiceDots())
		b.WriteString("\n")
	}

	// Embedded viewer body.
	if m.viewer != nil {
		b.WriteString(m.viewer.View())
	} else {
		b.WriteString(DimStyle.Render("select a project"))
	}
	b.WriteString("\n")

	// Read-only snapshot strip.
	b.WriteString(m.renderSnapshotStrip())

	return columnBox(b.String(), w, h, focused)
}

// renderServiceDots renders one status dot per bound service.
func (m DashboardModel) renderServiceDots() string {
	parts := make([]string, 0, len(m.boundServices))
	for _, svc := range m.boundServices {
		state := StatusUnknown
		if st, ok := m.serviceStatus[svc.ID]; ok {
			state = st.State
		}
		label := svc.Label
		if label == "" {
			label = svc.ID
		}
		parts = append(parts, fmt.Sprintf("%s %s", statusStateDot(state), DimStyle.Render(label)))
	}
	return strings.Join(parts, "  ")
}

// renderSnapshotStrip renders the read-only capture of the focused session.
func (m DashboardModel) renderSnapshotStrip() string {
	label := DimStyle.Render("snapshot (read-only)")
	if strings.TrimSpace(m.snapshot) == "" {
		return label + "\n" + DimStyle.Render("—")
	}
	// Show only the last line as a compact strip.
	lines := strings.Split(strings.TrimRight(m.snapshot, "\n"), "\n")
	last := lines[len(lines)-1]
	return label + "\n" + WhiteStyle.Render(last)
}

func (m DashboardModel) renderFooter() string {
	if m.statusMsg != "" {
		if m.statusErr {
			return " " + ErrorStyle.Render(m.statusMsg)
		}
		return " " + DimStyle.Render(m.statusMsg)
	}
	keys := "s start  x stop  k kill  enter attach  r refresh  c context  tab focus"
	return " " + DimStyle.Render(keys)
}

// ---------------------------------------------------------------------------
// Rendering helpers
// ---------------------------------------------------------------------------

func columnTitle(text string, focused bool) string {
	if focused {
		return SelectedStyle.Render(text)
	}
	return SubtitleStyle.Render(text)
}

func selectedRow(text string, focused bool) string {
	marker := DimStyle.Render(">")
	if focused {
		marker = SelectedStyle.Render(">")
		return marker + " " + SelectedStyle.Render(text)
	}
	return marker + " " + WhiteStyle.Render(text)
}

func columnBox(content string, w, h int, focused bool) string {
	style := lipgloss.NewStyle().
		Width(w).
		Height(h).
		Padding(0, 1).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorDimGray)
	if focused {
		style = style.BorderForeground(ColorBrCyan)
	}
	return style.Render(content)
}

// sessionStateDot maps a SessionState to a colored status glyph.
func sessionStateDot(state SessionState) string {
	switch state {
	case SessionRunning:
		return SuccessStyle.Render("●")
	case SessionStarting:
		return WarningStyle.Render("◐")
	case SessionIdle:
		return DimStyle.Render("○")
	case SessionError:
		return ErrorStyle.Render("✕")
	case SessionExited:
		return DimStyle.Render("✓")
	default:
		return DimStyle.Render("○")
	}
}

// statusStateDot maps a StatusState to a colored status glyph per the locked UX:
// Authed/Reachable green ●, Configured yellow ◐, Unknown dim ○, Error red ✕.
func statusStateDot(state StatusState) string {
	switch state {
	case StatusAuthed, StatusReachable:
		return SuccessStyle.Render("●")
	case StatusConfigured:
		return WarningStyle.Render("◐")
	case StatusError:
		return ErrorStyle.Render("✕")
	default: // StatusUnknown
		return DimStyle.Render("○")
	}
}
