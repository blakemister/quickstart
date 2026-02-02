package cmd

import (
	"fmt"
	"os"

	"github.com/bcmister/qk/internal/config"
	"github.com/bcmister/qk/internal/monitor"
	"github.com/bcmister/qk/internal/window"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "qk",
	Short: "Launch terminal windows across monitors with project picker",
	RunE:  runQk,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(setCmd)
	rootCmd.AddCommand(monitorsCmd)
	rootCmd.AddCommand(versionCmd)
}

func runQk(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load("")
	if err != nil {
		if os.IsNotExist(err) {
			// Auto-configure: detect monitors, 1 window each, default projects root
			return autoLaunch()
		}
		return fmt.Errorf("failed to load config: %w", err)
	}

	return launch(cfg)
}

func autoLaunch() error {
	monitors, err := monitor.Detect()
	if err != nil {
		return fmt.Errorf("failed to detect monitors: %w", err)
	}

	monConfigs := make([]config.MonitorConfig, len(monitors))
	for i := range monitors {
		monConfigs[i] = config.MonitorConfig{Windows: 1, Layout: "full"}
	}

	cfg := &config.Config{
		Version:      2,
		ProjectsRoot: config.DefaultProjectsRoot(),
		Monitors:     monConfigs,
	}

	// Save so next run is instant
	config.Save(cfg, "")

	return launch(cfg)
}

func launch(cfg *config.Config) error {
	monitors, err := monitor.Detect()
	if err != nil {
		return fmt.Errorf("failed to detect monitors: %w", err)
	}

	// Build all launch configs up front
	var configs []window.LaunchConfig
	for i, mc := range cfg.Monitors {
		if i >= len(monitors) {
			break
		}
		positions := window.CalculateLayout(&monitors[i], mc.Windows, mc.Layout)
		for j, pos := range positions {
			configs = append(configs, window.LaunchConfig{
				Title:      fmt.Sprintf("qk-%d-%d", i+1, j+1),
				WorkingDir: cfg.ProjectsRoot,
				X:          pos.X,
				Y:          pos.Y,
				Width:      pos.Width,
				Height:     pos.Height,
			})
		}
	}

	fmt.Printf("Launching %d terminals...", len(configs))

	// Launch all in parallel
	results := window.LaunchAll(configs, config.Command)

	fmt.Println("done.")

	for _, r := range results {
		if r.Err != nil {
			fmt.Printf("  Warning: %s: %v\n", r.Title, r.Err)
		}
	}

	return nil
}

var monitorsCmd = &cobra.Command{
	Use:   "monitors",
	Short: "List detected monitors",
	RunE:  runMonitors,
}

func runMonitors(cmd *cobra.Command, args []string) error {
	monitors, err := monitor.Detect()
	if err != nil {
		return fmt.Errorf("failed to detect monitors: %w", err)
	}

	fmt.Printf("Detected %d monitors:\n\n", len(monitors))
	for i, m := range monitors {
		primary := ""
		if m.Primary {
			primary = " (Primary)"
		}
		fmt.Printf("  Monitor %d%s:\n", i+1, primary)
		fmt.Printf("    Resolution: %dx%d\n", m.Width, m.Height)
		fmt.Printf("    Position:   (%d, %d)\n", m.X, m.Y)
		fmt.Println()
	}
	return nil
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("qk v0.3.0")
	},
}
