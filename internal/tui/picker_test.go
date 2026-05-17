package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bcmister/qs/internal/config"
	tea "github.com/charmbracelet/bubbletea"
)

func TestSanitizeProjectName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "simple", input: "my-project", want: "my-project"},
		{name: "spaces allowed", input: "My New App", want: "My New App"},
		{name: "trim outer spaces", input: "  test  ", want: "test"},
		{name: "empty", input: "", wantErr: true},
		{name: "slash", input: "a/b", wantErr: true},
		{name: "backslash", input: "a\\b", wantErr: true},
		{name: "dot", input: ".", wantErr: true},
		{name: "dotdot", input: "..", wantErr: true},
		{name: "reserved", input: "CON", wantErr: true},
		{name: "invalid chars", input: "a:b", wantErr: true},
		{name: "trailing dot", input: "abc.", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := sanitizeProjectName(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for input %q", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for input %q: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("sanitizeProjectName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func setupTestDirs(t *testing.T) (string, *config.Config) {
	t.Helper()
	root := t.TempDir()
	// root/alpha/sub1
	// root/alpha/sub2
	// root/beta
	os.MkdirAll(filepath.Join(root, "alpha", "sub1"), 0755)
	os.MkdirAll(filepath.Join(root, "alpha", "sub2"), 0755)
	os.MkdirAll(filepath.Join(root, "beta"), 0755)

	cfg := &config.Config{
		ProjectsRoot: root,
		Accounts: []config.Account{
			{ID: "test", Label: "Test", Command: "echo", Enabled: true},
			{ID: "test2", Label: "Test2", Command: "echo", Enabled: true},
		},
	}
	return root, cfg
}

func sendKey(m tea.Model, keyType tea.KeyType) tea.Model {
	updated, _ := m.Update(tea.KeyMsg{Type: keyType})
	return updated
}

func TestBrowseRightNavigatesIntoSubdir(t *testing.T) {
	root, cfg := setupTestDirs(t)
	m := NewPicker(cfg)

	// cursor starts at 1 (first project = "alpha")
	if m.cursor != 1 {
		t.Fatalf("expected cursor=1, got %d", m.cursor)
	}
	if m.filtered[0].name != "alpha" {
		t.Fatalf("expected first project=alpha, got %s", m.filtered[0].name)
	}

	// Press Right to navigate into alpha
	result := sendKey(m, tea.KeyRight)
	pm := result.(PickerModel)

	if pm.browseDir != filepath.Join(root, "alpha") {
		t.Fatalf("expected browseDir=%s, got %s", filepath.Join(root, "alpha"), pm.browseDir)
	}
	if len(pm.browseStack) != 1 {
		t.Fatalf("expected browseStack length=1, got %d", len(pm.browseStack))
	}
	// Should see sub1, sub2
	if len(pm.projects) != 2 {
		t.Fatalf("expected 2 subprojects, got %d: %v", len(pm.projects), pm.projects)
	}
}

func TestBrowseLeftRestoresState(t *testing.T) {
	_, cfg := setupTestDirs(t)
	m := NewPicker(cfg)

	// Navigate into alpha
	result := sendKey(m, tea.KeyRight)
	// Navigate back
	result = sendKey(result, tea.KeyLeft)
	pm := result.(PickerModel)

	if pm.browseDir != cfg.ProjectsRoot {
		t.Fatalf("expected browseDir=%s, got %s", cfg.ProjectsRoot, pm.browseDir)
	}
	if len(pm.browseStack) != 0 {
		t.Fatalf("expected empty browseStack, got %d", len(pm.browseStack))
	}
	if pm.cursor != 1 {
		t.Fatalf("expected cursor restored to 1, got %d", pm.cursor)
	}
}

func TestBrowseLeftAtRootIsNoop(t *testing.T) {
	_, cfg := setupTestDirs(t)
	m := NewPicker(cfg)

	result := sendKey(m, tea.KeyLeft)
	pm := result.(PickerModel)

	if pm.browseDir != cfg.ProjectsRoot {
		t.Fatalf("expected browseDir unchanged, got %s", pm.browseDir)
	}
	if len(pm.browseStack) != 0 {
		t.Fatalf("expected empty browseStack, got %d", len(pm.browseStack))
	}
}

func TestBrowseRightOnCreateRowIsNoop(t *testing.T) {
	_, cfg := setupTestDirs(t)
	m := NewPicker(cfg)
	// Move cursor to 0 (create new folder)
	m.cursor = 0

	result := sendKey(m, tea.KeyRight)
	pm := result.(PickerModel)

	if pm.browseDir != cfg.ProjectsRoot {
		t.Fatalf("expected browseDir unchanged, got %s", pm.browseDir)
	}
}

func TestBrowseIntoEmptyDir(t *testing.T) {
	root, cfg := setupTestDirs(t)
	m := NewPicker(cfg)

	// Move to beta (index 1 in filtered = "beta" is second, cursor=2)
	m.cursor = 2
	if m.filtered[1].name != "beta" {
		t.Fatalf("expected second project=beta, got %s", m.filtered[1].name)
	}

	result := sendKey(m, tea.KeyRight)
	pm := result.(PickerModel)

	if pm.browseDir != filepath.Join(root, "beta") {
		t.Fatalf("expected browseDir=%s, got %s", filepath.Join(root, "beta"), pm.browseDir)
	}
	// beta has no subdirs
	if len(pm.projects) != 0 {
		t.Fatalf("expected 0 projects in empty dir, got %d", len(pm.projects))
	}
	// cursor should be at 0 (create new folder)
	if pm.cursor != 0 {
		t.Fatalf("expected cursor=0 in empty dir, got %d", pm.cursor)
	}
}

func TestBreadcrumbShownWhenBrowsing(t *testing.T) {
	_, cfg := setupTestDirs(t)
	m := NewPicker(cfg)

	// Navigate into alpha
	result := sendKey(m, tea.KeyRight)
	pm := result.(PickerModel)

	view := pm.View()
	if !strings.Contains(view, "alpha") {
		t.Fatalf("expected breadcrumb containing 'alpha' in view")
	}
}

func TestLaunchDirSetOnEnter(t *testing.T) {
	root, cfg := setupTestDirs(t)
	m := NewPicker(cfg)

	// Navigate into alpha
	result := sendKey(m, tea.KeyRight)
	// cursor should be at 1 (sub1)
	pm := result.(PickerModel)
	if pm.cursor != 1 {
		t.Fatalf("expected cursor=1, got %d", pm.cursor)
	}

	// Press Enter to select sub1
	result = sendKey(pm, tea.KeyEnter)
	pm = result.(PickerModel)

	expected := filepath.Join(root, "alpha", "sub1")
	if pm.launchDir != expected {
		t.Fatalf("expected launchDir=%s, got %s", expected, pm.launchDir)
	}
}

func TestScanProjects(t *testing.T) {
	root := t.TempDir()

	// Create some project dirs
	os.MkdirAll(filepath.Join(root, "alpha"), 0755)
	os.MkdirAll(filepath.Join(root, "beta"), 0755)
	os.MkdirAll(filepath.Join(root, "gamma"), 0755)
	// Hidden dir should be excluded
	os.MkdirAll(filepath.Join(root, ".hidden"), 0755)
	// File should be excluded from the dirs-only wrapper
	os.WriteFile(filepath.Join(root, "file.txt"), []byte("hi"), 0644)

	projects := scanProjects(root)

	if len(projects) != 3 {
		t.Fatalf("expected 3 projects, got %d: %v", len(projects), projects)
	}
	// Should be sorted
	if projects[0] != "alpha" || projects[1] != "beta" || projects[2] != "gamma" {
		t.Errorf("expected [alpha beta gamma], got %v", projects)
	}

	// Empty dir
	emptyRoot := t.TempDir()
	projects = scanProjects(emptyRoot)
	if len(projects) != 0 {
		t.Errorf("expected 0 projects for empty dir, got %d", len(projects))
	}

	// Non-existent dir
	projects = scanProjects(filepath.Join(root, "nonexistent"))
	if projects != nil {
		t.Errorf("expected nil for non-existent dir, got %v", projects)
	}
}

func TestScanEntriesIncludesFilesAndDirs(t *testing.T) {
	root := t.TempDir()

	os.MkdirAll(filepath.Join(root, "alpha"), 0755)
	os.MkdirAll(filepath.Join(root, "beta"), 0755)
	os.MkdirAll(filepath.Join(root, ".hidden"), 0755)
	os.WriteFile(filepath.Join(root, "README.md"), []byte("# hi"), 0644)
	os.WriteFile(filepath.Join(root, "notes.txt"), []byte("ok"), 0644)
	os.WriteFile(filepath.Join(root, ".env"), []byte("x"), 0644)

	entries := scanEntries(root)

	if len(entries) != 4 {
		t.Fatalf("expected 4 entries (2 dirs + 2 files), got %d: %+v", len(entries), entries)
	}
	// Dirs first, sorted, then files sorted.
	want := []entry{
		{name: "alpha", isDir: true},
		{name: "beta", isDir: true},
		{name: "README.md", isDir: false},
		{name: "notes.txt", isDir: false},
	}
	for i, w := range want {
		if entries[i] != w {
			t.Errorf("entries[%d] = %+v, want %+v", i, entries[i], w)
		}
	}
}

func TestScanEntriesEmpty(t *testing.T) {
	root := t.TempDir()
	entries := scanEntries(root)
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries for empty dir, got %d", len(entries))
	}
}

func TestScanEntriesNonExistent(t *testing.T) {
	entries := scanEntries(filepath.Join(t.TempDir(), "nope"))
	if entries != nil {
		t.Fatalf("expected nil for non-existent dir, got %+v", entries)
	}
}

func TestApplyFilterMatchesEntries(t *testing.T) {
	root := t.TempDir()
	os.MkdirAll(filepath.Join(root, "alpha"), 0755)
	os.MkdirAll(filepath.Join(root, "beta"), 0755)
	os.WriteFile(filepath.Join(root, "alphabet.md"), []byte("ok"), 0644)
	os.WriteFile(filepath.Join(root, "ignore.txt"), []byte("ok"), 0644)

	cfg := &config.Config{
		ProjectsRoot: root,
		Accounts: []config.Account{
			{ID: "test", Label: "Test", Command: "echo", Enabled: true},
		},
	}
	m := NewPicker(cfg)

	m.filter = "alpha"
	m.applyFilter()

	if len(m.filtered) != 2 {
		t.Fatalf("expected 2 matches for 'alpha', got %d: %+v", len(m.filtered), m.filtered)
	}
	if m.filtered[0].name != "alpha" || !m.filtered[0].isDir {
		t.Errorf("first match should be the alpha directory, got %+v", m.filtered[0])
	}
	if m.filtered[1].name != "alphabet.md" || m.filtered[1].isDir {
		t.Errorf("second match should be alphabet.md (file), got %+v", m.filtered[1])
	}
}

func TestEnterOnFileOpensViewer(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "README.md"), []byte("# Hello"), 0644)

	cfg := &config.Config{
		ProjectsRoot: root,
		Accounts: []config.Account{
			{ID: "test", Label: "Test", Command: "echo", Enabled: true},
		},
	}
	m := NewPicker(cfg)
	// cursor=1 (the only entry, which is the README file)
	if m.filtered[0].isDir {
		t.Fatalf("expected README.md to be a file entry, got %+v", m.filtered[0])
	}

	result := sendKey(m, tea.KeyEnter)
	pm := result.(PickerModel)
	if pm.stage != stageView {
		t.Fatalf("expected stage=stageView after Enter on file, got %d", pm.stage)
	}
	if pm.viewer == nil {
		t.Fatal("expected viewer to be set")
	}
	if pm.viewer.kind != viewerKindMarkdown {
		t.Fatalf("expected markdown viewer, got %v", pm.viewer.kind)
	}
}

func TestEscFromViewerReturnsToProjectStage(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "doc.md"), []byte("# Doc"), 0644)
	cfg := &config.Config{
		ProjectsRoot: root,
		Accounts: []config.Account{
			{ID: "test", Label: "Test", Command: "echo", Enabled: true},
		},
	}
	m := NewPicker(cfg)
	result := sendKey(m, tea.KeyEnter)
	pm := result.(PickerModel)
	if pm.stage != stageView {
		t.Fatalf("expected viewer stage, got %d", pm.stage)
	}

	result = sendKey(pm, tea.KeyEsc)
	pm = result.(PickerModel)
	if pm.stage != stageProject {
		t.Fatalf("expected stageProject after Esc, got %d", pm.stage)
	}
	if pm.viewer != nil {
		t.Fatal("expected viewer to be cleared")
	}
}

