package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

// viewerKind classifies a file for rendering.
type viewerKind int

const (
	viewerKindMarkdown viewerKind = iota
	viewerKindCode
	viewerKindText
	viewerKindBinary
)

// binaryProbeBytes is how many bytes we sniff to decide if a file is binary.
const binaryProbeBytes = 512

// chromaStyleName is the chroma style applied to code highlighting. Fixed by
// spec — no user customization.
const chromaStyleName = "monokai"

// chromaFormatterName selects a broadly compatible 256-color terminal formatter.
const chromaFormatterName = "terminal256"

// viewerFooterRows is the number of rows reserved at the bottom of the
// viewer for the footer (one status line plus the blank separator above it).
// Kept as a fixed budget so SetSize is cheap; the footer's actual height is
// derived from this constant.
const viewerFooterRows = 2

// extToLexer maps file extensions to chroma lexer names. Keys are lowercase
// and include the leading dot.
var extToLexer = map[string]string{
	".go":         "go",
	".ts":         "typescript",
	".tsx":        "tsx",
	".js":         "javascript",
	".jsx":        "jsx",
	".py":         "python",
	".rs":         "rust",
	".java":       "java",
	".c":          "c",
	".cpp":        "cpp",
	".h":          "c",
	".hpp":        "cpp",
	".cs":         "csharp",
	".rb":         "ruby",
	".php":        "php",
	".swift":      "swift",
	".kt":         "kotlin",
	".scala":      "scala",
	".sh":         "bash",
	".ps1":        "powershell",
	".bash":       "bash",
	".zsh":        "bash",
	".json":       "json",
	".yaml":       "yaml",
	".yml":        "yaml",
	".toml":       "toml",
	".xml":        "xml",
	".html":       "html",
	".css":        "css",
	".scss":       "scss",
	".sql":        "sql",
	".vue":        "vue",
	".svelte":     "svelte",
	".dockerfile": "docker",
	".makefile":   "makefile",
}

// nameToLexer maps full filenames (extension-less) to chroma lexer names.
var nameToLexer = map[string]string{
	"dockerfile": "docker",
	"makefile":   "makefile",
}

// viewerModel renders a file in a scrollable viewport.
type viewerModel struct {
	vp       viewport.Model
	path     string
	rawBytes []byte
	kind     viewerKind
	width    int
	height   int
	lang     string // chroma lexer name for code; empty otherwise
	loadErr  error  // error reading the file; nil on success
}

// newViewer reads the file at path and returns a viewer model sized to
// (width, height). It never returns nil — read errors are stored on the
// returned model so the viewer can show them.
func newViewer(path string, width, height int) *viewerModel {
	m := &viewerModel{
		path:   path,
		width:  width,
		height: height,
	}

	data, err := os.ReadFile(path)
	if err != nil {
		m.loadErr = err
		m.kind = viewerKindText
		m.vp = newSizedViewport(width, height)
		return m
	}
	m.rawBytes = data

	if isBinary(data) {
		m.kind = viewerKindBinary
		return m
	}

	m.kind, m.lang = classify(path)
	m.vp = newSizedViewport(width, height)
	m.render()
	return m
}

// newSizedViewport returns a viewport sized for the available screen area
// after reserving footer rows.
func newSizedViewport(width, height int) viewport.Model {
	vpHeight := height - viewerFooterRows
	if vpHeight < 1 {
		vpHeight = 1
	}
	vpWidth := width
	if vpWidth < 1 {
		vpWidth = 1
	}
	return viewport.New(vpWidth, vpHeight)
}

// render produces the styled string and pushes it into the viewport. Called
// on load and on resize — never on every frame.
func (m *viewerModel) render() {
	if m.kind == viewerKindBinary {
		return
	}

	content := string(m.rawBytes)
	switch m.kind {
	case viewerKindMarkdown:
		out, err := renderMarkdown(content, m.vp.Width)
		if err != nil {
			// Fall back to plain text on glamour failure.
			m.vp.SetContent(content)
			return
		}
		m.vp.SetContent(out)
	case viewerKindCode:
		out, err := renderCode(content, m.lang)
		if err != nil {
			m.vp.SetContent(content)
			return
		}
		m.vp.SetContent(out)
	default:
		m.vp.SetContent(content)
	}
	m.vp.GotoTop()
}

// SetSize updates dimensions and re-renders. Called from the picker on
// tea.WindowSizeMsg.
func (m *viewerModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	if m.kind == viewerKindBinary {
		return
	}
	m.vp.Width = width
	vpHeight := height - viewerFooterRows
	if vpHeight < 1 {
		vpHeight = 1
	}
	m.vp.Height = vpHeight
	m.render()
}

// Update handles key and mouse messages, delegating most scroll keys to the
// viewport. Esc / Left / q are handled by the picker, not here.
func (m *viewerModel) Update(msg tea.Msg) tea.Cmd {
	if m.kind == viewerKindBinary {
		return nil
	}

	// Handle g / G (top / bottom) before delegating — the bubbles viewport
	// does not bind these by default.
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "g", "home":
			m.vp.GotoTop()
			return nil
		case "G", "end":
			m.vp.GotoBottom()
			return nil
		}
	}

	var cmd tea.Cmd
	m.vp, cmd = m.vp.Update(msg)
	return cmd
}

// View renders the viewer including its footer.
func (m *viewerModel) View() string {
	if m.kind == viewerKindBinary {
		return m.viewBinary()
	}

	body := m.vp.View()
	if m.loadErr != nil {
		body = ErrorStyle.Render(fmt.Sprintf("failed to read file: %v", m.loadErr))
	}
	return body + "\n" + m.footer()
}

// viewBinary centers the unsupported-message body and shows the footer.
func (m *viewerModel) viewBinary() string {
	w := m.width
	h := m.height - viewerFooterRows
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	msg := DimStyle.Render("Binary file — preview not supported")
	box := lipgloss.NewStyle().
		Width(w).
		Height(h).
		Align(lipgloss.Center, lipgloss.Center).
		Render(msg)
	return box + "\n" + m.footer()
}

// footer renders the bottom status line.
func (m *viewerModel) footer() string {
	dim := DimStyle
	white := WhiteStyle
	sep := dim.Render("  •  ")

	pathStr := white.Render(m.path)
	var scrollStr string
	if m.kind == viewerKindBinary {
		scrollStr = dim.Render("binary")
	} else {
		scrollStr = dim.Render(fmt.Sprintf("%3d%%", int(m.vp.ScrollPercent()*100)))
	}
	help := dim.Render("esc back  q quit")

	return " " + pathStr + sep + scrollStr + sep + help
}

// renderMarkdown runs the content through glamour with the auto style. Word
// wrap is set to the viewport width to keep code blocks inside the visible
// area.
func renderMarkdown(content string, width int) (string, error) {
	wrap := width
	if wrap < 1 {
		wrap = 80
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(wrap),
	)
	if err != nil {
		return "", err
	}
	out, err := r.Render(content)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(out, "\n"), nil
}

// renderCode highlights content with chroma. If lang is empty, chroma's
// content analyser is used; falls back to plain text on failure.
func renderCode(content, lang string) (string, error) {
	lexer := lexers.Get(lang)
	if lexer == nil {
		lexer = lexers.Analyse(content)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}

	style := styles.Get(chromaStyleName)
	if style == nil {
		style = styles.Fallback
	}

	formatter := formatters.Get(chromaFormatterName)
	if formatter == nil {
		formatter = formatters.Fallback
	}

	it, err := lexer.Tokenise(nil, content)
	if err != nil {
		return "", err
	}

	var buf strings.Builder
	if err := formatter.Format(&buf, style, it); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// isBinary returns true when the first probe bytes contain a null byte or are
// not valid UTF-8. Empty content is treated as text.
func isBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	probe := data
	if len(probe) > binaryProbeBytes {
		probe = probe[:binaryProbeBytes]
	}
	for _, b := range probe {
		if b == 0 {
			return true
		}
	}
	return !utf8.Valid(probe)
}

// classify inspects a file path and returns the viewer kind plus the chroma
// lexer name for code files (empty for non-code).
func classify(path string) (viewerKind, string) {
	base := strings.ToLower(filepath.Base(path))
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".md", ".markdown":
		return viewerKindMarkdown, ""
	}

	if lang, ok := extToLexer[ext]; ok {
		return viewerKindCode, lang
	}

	// Extension-less filename matches (e.g. Dockerfile, Makefile).
	if ext == "" {
		if lang, ok := nameToLexer[base]; ok {
			return viewerKindCode, lang
		}
	}

	return viewerKindText, ""
}

// detectLanguage exposes classify's lexer-name result for testing.
func detectLanguage(path string) string {
	_, lang := classify(path)
	return lang
}
