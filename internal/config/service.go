package config

import "time"

// Category classifies a service by the kind of integration it represents.
type Category string

const (
	CategoryAICoding     Category = "ai-coding"
	CategoryIssueTracker Category = "issue-tracker"
	CategoryEmailCal     Category = "email-cal"
	CategoryCloudDeploy  Category = "cloud-deploy"
	CategoryVCSIdentity  Category = "vcs-identity"
	CategoryDatabase     Category = "database"
	CategoryMCPServer    Category = "mcp-server"
	CategoryDocs         Category = "docs"
	CategoryOther        Category = "other"
)

// Service represents an external integration (issue tracker, deploy target,
// database, etc.) that a project may depend on. Status is probed at runtime.
type Service struct {
	ID          string   `yaml:"id"`
	Label       string   `yaml:"label"`
	Category    Category `yaml:"category"`
	Icon        string   `yaml:"icon,omitempty"`
	StatusCmd   string   `yaml:"statusCmd,omitempty"`
	RequiresEnv []string `yaml:"requiresEnv,omitempty"`
	Enabled     bool     `yaml:"enabled"`
}

// StatusState is the runtime connection state of a service.
type StatusState int

const (
	StatusUnknown StatusState = iota
	StatusConfigured
	StatusAuthed
	StatusReachable
	StatusError
)

// ServiceStatus is a runtime probe result for a single service.
type ServiceStatus struct {
	ServiceID string
	State     StatusState
	Detail    string
	CheckedAt time.Time
}

// ServiceByID finds a service by ID, returning nil if not found.
func ServiceByID(cfg *Config, id string) *Service {
	if cfg == nil {
		return nil
	}
	for i := range cfg.Services {
		if cfg.Services[i].ID == id {
			return &cfg.Services[i]
		}
	}
	return nil
}
