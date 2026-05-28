package config

import "testing"

func TestAddProfile(t *testing.T) {
	cfg := &Config{}
	if err := AddProfile(cfg, Profile{ID: "work", Label: "Work"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(cfg.Profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(cfg.Profiles))
	}

	// Duplicate ID rejected.
	if err := AddProfile(cfg, Profile{ID: "work"}); err == nil {
		t.Error("expected error on duplicate profile id")
	}
	// Empty ID rejected.
	if err := AddProfile(cfg, Profile{ID: ""}); err == nil {
		t.Error("expected error on empty profile id")
	}
	if len(cfg.Profiles) != 1 {
		t.Errorf("expected profiles unchanged after errors, got %d", len(cfg.Profiles))
	}
}

func TestAddService(t *testing.T) {
	cfg := &Config{}
	if err := AddService(cfg, Service{ID: "jira", Label: "Jira", Category: CategoryIssueTracker}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := AddService(cfg, Service{ID: "jira"}); err == nil {
		t.Error("expected error on duplicate service id")
	}
	if err := AddService(cfg, Service{ID: ""}); err == nil {
		t.Error("expected error on empty service id")
	}
}

func TestAddProject(t *testing.T) {
	cfg := &Config{
		Accounts: []Account{{ID: "claude", Command: "claude", Enabled: true}},
		Profiles: []Profile{{ID: "work"}},
		Services: []Service{{ID: "jira"}},
	}

	// Valid project with resolvable references.
	ok := Project{
		ID:             "p",
		Label:          "P",
		Path:           "C:/dev/p",
		Profile:        "work",
		Accounts:       []string{"claude"},
		DefaultAccount: "claude",
		Services:       []string{"jira"},
	}
	if err := AddProject(cfg, ok); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Duplicate ID.
	if err := AddProject(cfg, Project{ID: "p"}); err == nil {
		t.Error("expected error on duplicate project id")
	}
	// Dangling profile.
	if err := AddProject(cfg, Project{ID: "q", Profile: "ghost"}); err == nil {
		t.Error("expected error on unknown profile reference")
	}
	// Dangling account.
	if err := AddProject(cfg, Project{ID: "r", Accounts: []string{"nope"}}); err == nil {
		t.Error("expected error on unknown account reference")
	}
	// Dangling default account.
	if err := AddProject(cfg, Project{ID: "s", DefaultAccount: "nope"}); err == nil {
		t.Error("expected error on unknown default account reference")
	}
	// Dangling service.
	if err := AddProject(cfg, Project{ID: "t", Services: []string{"nope"}}); err == nil {
		t.Error("expected error on unknown service reference")
	}
	// Empty ID.
	if err := AddProject(cfg, Project{ID: ""}); err == nil {
		t.Error("expected error on empty project id")
	}

	if len(cfg.Projects) != 1 {
		t.Errorf("expected exactly 1 project after rejections, got %d", len(cfg.Projects))
	}
}

func TestSetProfileKey(t *testing.T) {
	keys := make(AccountKeys)

	if err := SetProfileKey(keys, "work", "GH_TOKEN", "ghp_x"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if KeysForProfile(keys, "work")["GH_TOKEN"] != "ghp_x" {
		t.Errorf("expected GH_TOKEN stored under profile namespace, got %v", KeysForProfile(keys, "work"))
	}
	// Stored under the prefixed key, not a bare profile id.
	if _, ok := keys["work"]; ok {
		t.Error("profile secret must not be stored under bare profile id")
	}

	// Invalid env var name rejected.
	if err := SetProfileKey(keys, "work", "BAD NAME", "v"); err == nil {
		t.Error("expected error for invalid env var name")
	}
	// Empty profile id rejected.
	if err := SetProfileKey(keys, "", "OK", "v"); err == nil {
		t.Error("expected error for empty profile id")
	}
	// Nil map rejected.
	if err := SetProfileKey(nil, "work", "OK", "v"); err == nil {
		t.Error("expected error for nil keys map")
	}
}
