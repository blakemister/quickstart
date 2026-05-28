package config

import "fmt"

// These helpers mutate the in-memory config and keys; they do NOT persist.
// Callers are responsible for calling Save / SaveKeys, matching the existing
// CloneAccount / SetAccountKey convention.

// AddProfile appends a profile after validating its ID is non-empty and unique.
func AddProfile(cfg *Config, p Profile) error {
	if cfg == nil {
		return fmt.Errorf("add profile: nil config")
	}
	if p.ID == "" {
		return fmt.Errorf("add profile: id cannot be empty")
	}
	if ProfileByID(cfg, p.ID) != nil {
		return fmt.Errorf("add profile: id %q already exists", p.ID)
	}
	cfg.Profiles = append(cfg.Profiles, p)
	return nil
}

// AddService appends a service after validating its ID is non-empty and unique.
func AddService(cfg *Config, s Service) error {
	if cfg == nil {
		return fmt.Errorf("add service: nil config")
	}
	if s.ID == "" {
		return fmt.Errorf("add service: id cannot be empty")
	}
	if ServiceByID(cfg, s.ID) != nil {
		return fmt.Errorf("add service: id %q already exists", s.ID)
	}
	cfg.Services = append(cfg.Services, s)
	return nil
}

// AddProject appends a project after validating its ID is non-empty and unique
// and that any profile/account/service references resolve to existing entries.
func AddProject(cfg *Config, p Project) error {
	if cfg == nil {
		return fmt.Errorf("add project: nil config")
	}
	if p.ID == "" {
		return fmt.Errorf("add project: id cannot be empty")
	}
	if ProjectByID(cfg.Projects, p.ID) != nil {
		return fmt.Errorf("add project: id %q already exists", p.ID)
	}
	if p.Profile != "" && ProfileByID(cfg, p.Profile) == nil {
		return fmt.Errorf("add project: unknown profile %q", p.Profile)
	}
	for _, id := range p.Accounts {
		if AccountByID(cfg.Accounts, id) == nil {
			return fmt.Errorf("add project: unknown account %q", id)
		}
	}
	if p.DefaultAccount != "" && AccountByID(cfg.Accounts, p.DefaultAccount) == nil {
		return fmt.Errorf("add project: unknown default account %q", p.DefaultAccount)
	}
	for _, id := range p.Services {
		if ServiceByID(cfg, id) == nil {
			return fmt.Errorf("add project: unknown service %q", id)
		}
	}
	cfg.Projects = append(cfg.Projects, p)
	return nil
}

// SetProfileKey writes a secret into a profile's namespace ("profile:<id>")
// after validating the env var name. It does not persist; callers SaveKeys.
func SetProfileKey(keys AccountKeys, profileID, name, value string) error {
	if keys == nil {
		return fmt.Errorf("set profile key: nil keys map")
	}
	if profileID == "" {
		return fmt.Errorf("set profile key: profile id cannot be empty")
	}
	if err := ValidateEnvVarName(name); err != nil {
		return fmt.Errorf("set profile key: %w", err)
	}
	SetAccountKey(keys, ProfileKeyID(profileID), name, value)
	return nil
}
