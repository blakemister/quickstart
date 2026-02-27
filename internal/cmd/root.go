package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bcmister/qs/internal/config"
	"github.com/bcmister/qs/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var projectFlag string

var rootCmd = &cobra.Command{
	Use:   "qs",
	Short: "Quickstart terminal launcher",
	RunE:  runRoot,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.Flags().StringVar(&projectFlag, "project", "", "Pre-select a project and skip to tool selection")
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(accountsCmd)
	rootCmd.AddCommand(monitorsCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(allCmd)
}

func runRoot(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load("")
	if err != nil {
		if os.IsNotExist(err) {
			cfg, err = runFirstRunFlow(nil)
			if err != nil {
				return err
			}
			if cfg == nil {
				return nil
			}
		} else {
			return fmt.Errorf("failed to load config: %w", err)
		}
	}

	if strings.TrimSpace(cfg.ProjectsRoot) == "" {
		cfg, err = runFirstRunFlow(cfg)
		if err != nil {
			return err
		}
		if cfg == nil {
			return nil
		}
	}

	config.EnsureDefaults(cfg)
	projectsRoot, err := ensureProjectsRoot(cfg.ProjectsRoot)
	if err != nil {
		return err
	}
	cfg.ProjectsRoot = projectsRoot

	var picker tui.PickerModel
	if projectFlag != "" {
		// Validate project directory exists
		projectDir := filepath.Join(cfg.ProjectsRoot, projectFlag)
		info, statErr := os.Stat(projectDir)
		if statErr != nil || !info.IsDir() {
			return fmt.Errorf("project %q not found in %s", projectFlag, cfg.ProjectsRoot)
		}
		picker = tui.NewPickerWithProject(cfg, projectFlag)
	} else {
		picker = tui.NewPicker(cfg)
	}
	p := tea.NewProgram(picker, tea.WithAltScreen())
	_, err = p.Run()
	return err
}

func runFirstRunFlow(existing *config.Config) (*config.Config, error) {
	firstRun := tui.NewFirstRun(existing)
	p := tea.NewProgram(firstRun, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return nil, err
	}

	model, ok := finalModel.(tui.FirstRunModel)
	if !ok {
		return nil, fmt.Errorf("unexpected first-run model type")
	}
	result := model.Result()

	switch result.Action {
	case tui.FirstRunQuit:
		return nil, nil
	case tui.FirstRunLaunchSetup:
		setup := tui.NewSetup(existing)
		setupProgram := tea.NewProgram(setup, tea.WithAltScreen())
		if _, err := setupProgram.Run(); err != nil {
			return nil, err
		}

		cfg, err := config.Load("")
		if err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, fmt.Errorf("failed to load config after setup: %w", err)
		}
		if strings.TrimSpace(cfg.ProjectsRoot) == "" {
			return nil, nil
		}
		config.EnsureDefaults(cfg)
		return cfg, nil
	case tui.FirstRunSetPathNow:
		projectsRoot := strings.TrimSpace(result.ProjectsRoot)
		normalizedRoot, err := ensureProjectsRoot(projectsRoot)
		if err != nil {
			return nil, err
		}

		cfg := existing
		if cfg == nil {
			cfg = config.NewDefaultConfig(normalizedRoot)
		} else {
			cfg.ProjectsRoot = normalizedRoot
			config.EnsureDefaults(cfg)
		}

		if err := config.Save(cfg, ""); err != nil {
			return nil, fmt.Errorf("failed to save config: %w", err)
		}
		return cfg, nil
	default:
		return nil, nil
	}
}

func ensureProjectsRoot(projectsRoot string) (string, error) {
	root := strings.TrimSpace(projectsRoot)
	if root == "" {
		return "", fmt.Errorf("project path is not configured")
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("invalid project path %q: %w", root, err)
	}

	info, statErr := os.Stat(absRoot)
	if statErr == nil {
		if !info.IsDir() {
			return "", fmt.Errorf("project path is not a directory: %s", absRoot)
		}
		return absRoot, nil
	}
	if !os.IsNotExist(statErr) {
		return "", fmt.Errorf("failed to access project path %s: %w", absRoot, statErr)
	}
	if err := os.MkdirAll(absRoot, 0755); err != nil {
		return "", fmt.Errorf("failed to create project path %s: %w", absRoot, err)
	}
	return absRoot, nil
}
