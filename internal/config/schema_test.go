package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestMigrateV4toV5LosslessRoundTrip verifies that a v4 YAML file unmarshals
// into a v5 Config with the new slices empty and only the version bumped — the
// migration is purely additive.
func TestMigrateV4toV5LosslessRoundTrip(t *testing.T) {
	v4yaml := []byte(`version: 4
projectsRoot: /home/test/.1dev
defaultAccount: claude
lastAccount: codex
accounts:
  - id: claude
    label: Claude Code
    command: claude
    args: ["--dangerously-skip-permissions"]
    icon: "C"
    enabled: true
monitors:
  - layout: vertical
    windows:
      - tool: claude
      - tool: codex
`)

	cfg, err := migrateV4(v4yaml)
	if err != nil {
		t.Fatalf("migrateV4 failed: %v", err)
	}

	if cfg.Version != CurrentVersion {
		t.Errorf("expected version %d, got %d", CurrentVersion, cfg.Version)
	}
	// Existing data preserved.
	if cfg.ProjectsRoot != "/home/test/.1dev" {
		t.Errorf("projectsRoot not preserved, got %q", cfg.ProjectsRoot)
	}
	if cfg.DefaultAccount != "claude" || cfg.LastAccount != "codex" {
		t.Errorf("account fields not preserved: %q / %q", cfg.DefaultAccount, cfg.LastAccount)
	}
	if len(cfg.Accounts) != 1 || len(cfg.Monitors) != 1 {
		t.Errorf("accounts/monitors not preserved: %d / %d", len(cfg.Accounts), len(cfg.Monitors))
	}
	// New slices start empty.
	if len(cfg.Profiles) != 0 {
		t.Errorf("expected empty Profiles after migration, got %v", cfg.Profiles)
	}
	if len(cfg.Services) != 0 {
		t.Errorf("expected empty Services after migration, got %v", cfg.Services)
	}
	if len(cfg.Projects) != 0 {
		t.Errorf("expected empty Projects after migration, got %v", cfg.Projects)
	}
}

// TestLoadV4FileMigratesToV5 verifies a v4 file on disk loads and migrates.
func TestLoadV4FileMigratesToV5(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	v4yaml := []byte(`version: 4
projectsRoot: /test
defaultAccount: claude
accounts:
  - id: claude
    label: Claude Code
    command: claude
    args: []
    icon: "C"
    enabled: true
monitors:
  - layout: full
    windows:
      - tool: claude
`)
	if err := os.WriteFile(path, v4yaml, 0644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.Version != CurrentVersion {
		t.Errorf("expected version %d after load, got %d", CurrentVersion, cfg.Version)
	}
	if len(cfg.Profiles) != 0 {
		t.Errorf("expected no profiles from a v4 file, got %v", cfg.Profiles)
	}
}

// TestEnsureDefaultsSeedsPersonalProfile verifies the default profile is seeded
// with no secrets and no git override.
func TestEnsureDefaultsSeedsPersonalProfile(t *testing.T) {
	cfg := &Config{}
	EnsureDefaults(cfg)

	if len(cfg.Profiles) != 1 {
		t.Fatalf("expected 1 seeded profile, got %d", len(cfg.Profiles))
	}
	p := cfg.Profiles[0]
	if p.ID != DefaultProfileID {
		t.Errorf("expected profile id %q, got %q", DefaultProfileID, p.ID)
	}
	if p.GitIdentity.Name != "" || p.GitIdentity.Email != "" {
		t.Errorf("expected no git override on default profile, got %+v", p.GitIdentity)
	}
	if len(p.AccountConfigDirs) != 0 {
		t.Errorf("expected no config dirs on default profile, got %v", p.AccountConfigDirs)
	}
}

// TestEnsureDefaultsPreservesUserGitIdentity guards the force-overwrite footgun:
// a user-set GitIdentity must survive EnsureDefaults untouched.
func TestEnsureDefaultsPreservesUserGitIdentity(t *testing.T) {
	cfg := &Config{
		Profiles: []Profile{
			{
				ID:    "work",
				Label: "Work",
				GitIdentity: GitIdentity{
					Name:  "Blake Work",
					Email: "blake@work.example",
				},
				AccountConfigDirs: map[string]string{"claude": "C:/work/.claude"},
			},
		},
	}
	EnsureDefaults(cfg)

	work := ProfileByID(cfg, "work")
	if work == nil {
		t.Fatal("expected work profile to survive")
	}
	if work.GitIdentity.Name != "Blake Work" || work.GitIdentity.Email != "blake@work.example" {
		t.Errorf("user git identity was clobbered: %+v", work.GitIdentity)
	}
	if work.AccountConfigDirs["claude"] != "C:/work/.claude" {
		t.Errorf("user config dir was clobbered: %v", work.AccountConfigDirs)
	}
}

// TestValidateProjectRefsDropsDangling verifies dangling profile/account/service
// references are pruned without panic and without dropping the project itself.
func TestValidateProjectRefsDropsDangling(t *testing.T) {
	cfg := &Config{
		Profiles: []Profile{{ID: "personal", Label: "Personal"}},
		Accounts: []Account{{ID: "claude", Command: "claude", Enabled: true}},
		Services: []Service{{ID: "jira", Label: "Jira", Category: CategoryIssueTracker, Enabled: true}},
		Projects: []Project{
			{
				ID:             "proj",
				Label:          "Proj",
				Path:           "C:/dev/proj",
				Profile:        "ghost",                       // dangling
				Accounts:       []string{"claude", "missing"}, // one valid, one dangling
				DefaultAccount: "also-missing",                // dangling
				Services:       []string{"jira", "nope"},      // one valid, one dangling
			},
		},
	}
	EnsureDefaults(cfg)

	if len(cfg.Projects) != 1 {
		t.Fatalf("project should be retained, got %d projects", len(cfg.Projects))
	}
	p := cfg.Projects[0]
	if p.Profile != "" {
		t.Errorf("expected dangling profile ref dropped, got %q", p.Profile)
	}
	if len(p.Accounts) != 1 || p.Accounts[0] != "claude" {
		t.Errorf("expected only valid account ref retained, got %v", p.Accounts)
	}
	if p.DefaultAccount != "" {
		t.Errorf("expected dangling defaultAccount dropped, got %q", p.DefaultAccount)
	}
	if len(p.Services) != 1 || p.Services[0] != "jira" {
		t.Errorf("expected only valid service ref retained, got %v", p.Services)
	}
}

// TestProjectForPath verifies path binding lookup and the unbound (nil) case.
func TestProjectForPath(t *testing.T) {
	cfg := &Config{
		Projects: []Project{
			{ID: "a", Path: "C:/dev/a"},
			{ID: "b", Path: "C:/dev/b"},
		},
	}

	if p := ProjectForPath(cfg, "C:/dev/a"); p == nil || p.ID != "a" {
		t.Errorf("expected project a, got %v", p)
	}
	// Case-insensitive + cleaning.
	if p := ProjectForPath(cfg, "c:/dev/./b/"); p == nil || p.ID != "b" {
		t.Errorf("expected project b for messy path, got %v", p)
	}
	if p := ProjectForPath(cfg, "C:/dev/unbound"); p != nil {
		t.Errorf("expected nil for unbound path, got %v", p)
	}
	if p := ProjectForPath(cfg, ""); p != nil {
		t.Errorf("expected nil for empty path, got %v", p)
	}
}

// TestProfileAndServiceByID covers the basic lookup helpers.
func TestProfileAndServiceByID(t *testing.T) {
	cfg := &Config{
		Profiles: []Profile{{ID: "x"}, {ID: "y"}},
		Services: []Service{{ID: "s1"}, {ID: "s2"}},
	}
	if ProfileByID(cfg, "y") == nil {
		t.Error("expected to find profile y")
	}
	if ProfileByID(cfg, "z") != nil {
		t.Error("expected nil for unknown profile")
	}

	if ServiceByID(cfg, "s1") == nil {
		t.Error("expected to find service s1")
	}
	if ServiceByID(cfg, "s3") != nil {
		t.Error("expected nil for unknown service")
	}
}

// TestServicesForProject resolves service IDs, skipping unresolved ones.
func TestServicesForProject(t *testing.T) {
	cfg := &Config{
		Services: []Service{
			{ID: "jira", Label: "Jira"},
			{ID: "railway", Label: "Railway"},
		},
		Projects: []Project{
			{ID: "p", Services: []string{"railway", "ghost", "jira"}},
		},
	}
	got := ServicesForProject(cfg, "p")
	if len(got) != 2 {
		t.Fatalf("expected 2 resolved services, got %d", len(got))
	}
	if got[0].ID != "railway" || got[1].ID != "jira" {
		t.Errorf("expected order preserved [railway, jira], got [%s, %s]", got[0].ID, got[1].ID)
	}
	if ServicesForProject(cfg, "missing") != nil {
		t.Error("expected nil for unknown project")
	}
}

// TestV5RoundTripWithNewSections verifies the new sections survive Save/Load.
func TestV5RoundTripWithNewSections(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := &Config{
		Version:        CurrentVersion,
		ProjectsRoot:   "/test",
		DefaultAccount: "claude",
		Accounts:       []Account{{ID: "claude", Command: "claude", Enabled: true}},
		Monitors:       []MonitorConfig{{Layout: "full", Windows: []WindowConfig{{Tool: "claude"}}}},
		Profiles: []Profile{
			{ID: "work", Label: "Work", GitIdentity: GitIdentity{Name: "B", Email: "b@w.io"}},
		},
		Services: []Service{
			{ID: "jira", Label: "Jira", Category: CategoryIssueTracker, RequiresEnv: []string{"JIRA_TOKEN"}, Enabled: true},
		},
		Projects: []Project{
			{ID: "p", Label: "P", Path: "/test/p", Profile: "work", Accounts: []string{"claude"}, Services: []string{"jira"}},
		},
	}

	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.Version != CurrentVersion {
		t.Errorf("expected version %d, got %d", CurrentVersion, loaded.Version)
	}
	if len(loaded.Profiles) != 1 || loaded.Profiles[0].GitIdentity.Email != "b@w.io" {
		t.Errorf("profiles not round-tripped: %+v", loaded.Profiles)
	}
	if len(loaded.Services) != 1 || loaded.Services[0].Category != CategoryIssueTracker {
		t.Errorf("services not round-tripped: %+v", loaded.Services)
	}
	if len(loaded.Projects) != 1 || loaded.Projects[0].Profile != "work" {
		t.Errorf("projects not round-tripped: %+v", loaded.Projects)
	}
}

// TestKeysForProfile verifies profile secret namespacing.
func TestKeysForProfile(t *testing.T) {
	keys := AccountKeys{
		"profile:work": {"GH_TOKEN": "ghp_x"},
		"claude":       {"ANTHROPIC_API_KEY": "sk"},
	}
	pk := KeysForProfile(keys, "work")
	if pk == nil || pk["GH_TOKEN"] != "ghp_x" {
		t.Errorf("expected work profile keys, got %v", pk)
	}
	if KeysForProfile(keys, "missing") != nil {
		t.Error("expected nil for unknown profile")
	}
	if KeysForProfile(nil, "work") != nil {
		t.Error("expected nil for nil map")
	}
}
