package config

import (
	"path/filepath"
	"strings"
)

// Project binds a directory to a profile, a set of accounts, and a set of
// services. A project's profile determines which secrets and identity are
// layered into the session env when a tool is launched there.
type Project struct {
	ID             string   `yaml:"id"`
	Label          string   `yaml:"label"`
	Path           string   `yaml:"path"`
	Profile        string   `yaml:"profile,omitempty"`
	Accounts       []string `yaml:"accounts,omitempty"`
	DefaultAccount string   `yaml:"defaultAccount,omitempty"`
	Services       []string `yaml:"services,omitempty"`
}

// ProjectByID finds a project by ID, returning nil if not found.
func ProjectByID(projects []Project, id string) *Project {
	for i := range projects {
		if projects[i].ID == id {
			return &projects[i]
		}
	}
	return nil
}

// ProjectForPath returns the project whose Path matches the given filesystem
// path (compared case-insensitively after cleaning, matching Windows
// semantics). Returns nil when the path is unbound to any project.
func ProjectForPath(cfg *Config, path string) *Project {
	if cfg == nil {
		return nil
	}
	target := normalizePath(path)
	if target == "" {
		return nil
	}
	for i := range cfg.Projects {
		if normalizePath(cfg.Projects[i].Path) == target {
			return &cfg.Projects[i]
		}
	}
	return nil
}

// ServicesForProject resolves a project's service IDs to Service values,
// skipping any IDs that do not resolve. Returns nil when the project is not
// found.
func ServicesForProject(cfg *Config, projectID string) []Service {
	if cfg == nil {
		return nil
	}
	p := ProjectByID(cfg.Projects, projectID)
	if p == nil {
		return nil
	}
	var result []Service
	for _, id := range p.Services {
		if svc := ServiceByID(cfg, id); svc != nil {
			result = append(result, *svc)
		}
	}
	return result
}

// normalizePath cleans a path and lowercases it for case-insensitive
// comparison on Windows. Empty input yields empty output.
func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return strings.ToLower(filepath.Clean(path))
}
