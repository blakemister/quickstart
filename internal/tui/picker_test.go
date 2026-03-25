package tui

import (
	"os"
	"path/filepath"
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
	if m.filtered[0] != "alpha" {
		t.Fatalf("expected first project=alpha, got %s", m.filtered[0])
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
	if m.filtered[1] != "beta" {
		t.Fatalf("expected second project=beta, got %s", m.filtered[1])
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
	if !contains(view, "alpha") {
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

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsSubstring(s, substr)
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
