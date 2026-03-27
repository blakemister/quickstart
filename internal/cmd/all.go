package cmd

import (
	"fmt"
	"strings"

	"github.com/bcmister/qs/internal/config"
	"github.com/bcmister/qs/internal/launcher"
	"github.com/bcmister/qs/internal/monitor"
	"github.com/bcmister/qs/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var allCmd = &cobra.Command{
	Use:   "all",
	Short: "Launch terminals across all monitors",
	RunE:  runAll,
}

func runAll(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if strings.TrimSpace(cfg.ProjectsRoot) == "" {
		return fmt.Errorf("projects root not configured — run qs setup first")
	}

	config.EnsureDefaults(cfg)
	projectsRoot, err := ensureProjectsRoot(cfg.ProjectsRoot)
	if err != nil {
		return err
	}
	cfg.ProjectsRoot = projectsRoot

	// Run the AllModel TUI to get window counts per monitor
	allModel := tui.NewAll(cfg)
	p := tea.NewProgram(allModel, tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return err
	}

	result, ok := finalModel.(tui.AllModel)
	if !ok {
		return fmt.Errorf("unexpected model type")
	}

	if result.Cancelled() || !result.Confirmed() {
		return nil
	}

	windowCounts := result.WindowCounts()

	// Detect monitors for positioning
	monitors, err := monitor.Detect()
	if err != nil {
		return fmt.Errorf("failed to detect monitors: %w", err)
	}

	// Build LaunchConfigs — each window runs plain `qs` with its own picker
	var configs []launcher.LaunchConfig
	for monIdx, mon := range monitors {
		count := 1
		if monIdx < len(windowCounts) {
			count = windowCounts[monIdx]
		}
		if count < 1 {
			count = 1
		}

		layout := layoutForCount(count)
		positions := launcher.CalculateLayout(&mon, count, layout)

		for winIdx, pos := range positions {
			configs = append(configs, launcher.LaunchConfig{
				Title:      fmt.Sprintf("qs-%d-%d", monIdx+1, winIdx+1),
				WorkingDir: cfg.ProjectsRoot,
				X:          pos.X,
				Y:          pos.Y,
				Width:      pos.Width,
				Height:     pos.Height,
				Command:    "qs",
			})
		}
	}

	if len(configs) == 0 {
		return fmt.Errorf("no windows to launch")
	}

	// Launch all windows as new terminals with their own pickers
	launcher.LaunchAll(configs)

	return nil
}

// layoutForCount maps window count to layout name (mirrors tui.layoutForCount)
func layoutForCount(count int) string {
	switch {
	case count <= 1:
		return "full"
	case count == 2:
		return "vertical"
	default:
		return "grid"
	}
}
