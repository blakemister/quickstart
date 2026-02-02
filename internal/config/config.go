package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const Command = "claude --dangerously-skip-permissions"

// Config represents the application configuration
type Config struct {
	Version      int             `yaml:"version"`
	ProjectsRoot string          `yaml:"projectsRoot"`
	Monitors     []MonitorConfig `yaml:"monitors"`
}

// MonitorConfig represents configuration for a single monitor
type MonitorConfig struct {
	Windows int    `yaml:"windows"`
	Layout  string `yaml:"layout"` // grid, vertical, full
}

// DefaultConfigPath returns the default configuration file path
func DefaultConfigPath() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".qk", "config.yaml")
}

// DefaultProjectsRoot returns the default projects directory
func DefaultProjectsRoot() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".1dev")
}

// Load reads the configuration from a file
func Load(path string) (*Config, error) {
	if path == "" {
		path = DefaultConfigPath()
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg Config
	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &cfg, nil
}

// Save writes the configuration to a file
func Save(cfg *Config, path string) error {
	if path == "" {
		path = DefaultConfigPath()
	}

	dir := filepath.Dir(path)
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	err = os.WriteFile(path, data, 0644)
	if err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}
