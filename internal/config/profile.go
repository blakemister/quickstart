package config

// GitIdentity holds per-profile git author/committer identity. When set, it is
// layered into the session env as GIT_AUTHOR_* / GIT_COMMITTER_* so commits made
// inside a launched tool are attributed to the right person.
type GitIdentity struct {
	Name  string `yaml:"name,omitempty"`
	Email string `yaml:"email,omitempty"`
}

// Profile groups secrets, git identity, and per-tool config dirs for one
// company/context. Secrets themselves live in keys.yaml under "profile:<id>";
// the Profile struct only references non-secret wiring.
type Profile struct {
	ID          string      `yaml:"id"`
	Label       string      `yaml:"label"`
	Icon        string      `yaml:"icon,omitempty"`
	Color       string      `yaml:"color,omitempty"`
	GitIdentity GitIdentity `yaml:"gitIdentity,omitempty"`
	// AccountConfigDirs maps a tool key (e.g. "claude", "gh", "gws") to an
	// isolated config directory for that profile.
	AccountConfigDirs map[string]string `yaml:"accountConfigDirs,omitempty"`
	// ExpectedEnv lists env var names a fully-configured profile should provide,
	// used for "N of M connected" reporting.
	ExpectedEnv []string `yaml:"expectedEnv,omitempty"`
}

// ProfileByID finds a profile by ID, returning nil if not found.
func ProfileByID(cfg *Config, id string) *Profile {
	if cfg == nil {
		return nil
	}
	for i := range cfg.Profiles {
		if cfg.Profiles[i].ID == id {
			return &cfg.Profiles[i]
		}
	}
	return nil
}
