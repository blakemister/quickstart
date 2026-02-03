package cmd

import (
	"fmt"
	"os"

	"github.com/bcmister/qk/internal/config"
	"github.com/bcmister/qk/internal/monitor"
	"github.com/bcmister/qk/internal/ui"
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
	rootCmd.AddCommand(tabCmd)
}

func runQk(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load("")
	if err != nil {
		if os.IsNotExist(err) {
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

	config.Save(cfg, "")

	return launch(cfg)
}

func launch(cfg *config.Config) error {
	monitors, err := monitor.Detect()
	if err != nil {
		return fmt.Errorf("failed to detect monitors: %w", err)
	}

	// Group configs by monitor for display
	type monGroup struct {
		monIdx  int
		configs []window.LaunchConfig
	}
	var groups []monGroup

	var allConfigs []window.LaunchConfig
	for i, mc := range cfg.Monitors {
		if i >= len(monitors) {
			break
		}
		positions := window.CalculateLayout(&monitors[i], mc.Windows, mc.Layout)
		g := monGroup{monIdx: i}
		for j, pos := range positions {
			lc := window.LaunchConfig{
				Title:      fmt.Sprintf("qk-%d-%d", i+1, j+1),
				WorkingDir: cfg.ProjectsRoot,
				X:          pos.X,
				Y:          pos.Y,
				Width:      pos.Width,
				Height:     pos.Height,
			}
			allConfigs = append(allConfigs, lc)
			g.configs = append(g.configs, lc)
		}
		groups = append(groups, g)
	}

	ui.Logo("")
	ui.Sep()

	// Use current terminal for first window, spawn others
	launchResult := window.LaunchAllWithCurrent(allConfigs, config.Command)
	results := launchResult.Results

	// Adjust messaging based on how many new windows were spawned
	newWindows := len(allConfigs) - 1
	if newWindows > 0 {
		ui.Head(fmt.Sprintf("Launching %d new terminals (using current for first)", newWindows))
	} else {
		ui.Head("Using current terminal")
	}
	fmt.Println()

	// Build result lookup
	resultMap := make(map[string]error)
	for _, r := range results {
		resultMap[r.Title] = r.Err
	}

	// Display per-monitor panels
	for _, g := range groups {
		badge := ""
		if monitors[g.monIdx].Primary {
			badge = "Primary"
		}
		ui.BoxStart(fmt.Sprintf("Monitor %d", g.monIdx+1), badge)
		for _, c := range g.configs {
			err := resultMap[c.Title]
			label := fmt.Sprintf("%s%s%s", ui.White, c.Title, ui.Reset)
			ui.BoxRow(fmt.Sprintf("%s  %s%s%s",
				label,
				statusColor(err == nil), statusIcon(err == nil), ui.Reset))
		}
		ui.BoxEnd()
	}

	// Print any warnings
	hasWarn := false
	for _, r := range results {
		if r.Err != nil {
			if !hasWarn {
				fmt.Println()
				hasWarn = true
			}
			ui.Warn(fmt.Sprintf("%s: %v", r.Title, r.Err))
		}
	}

	ui.Fin("Ready")
	fmt.Println()

	// Run picker in current terminal (blocking)
	return launchResult.RunPicker()
}

func statusIcon(ok bool) string {
	if ok {
		return ui.Check
	}
	return ui.Cross
}

func statusColor(ok bool) string {
	if ok {
		return ui.BrGreen
	}
	return ui.BrRed
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

	ui.Head(fmt.Sprintf("Detected %d monitors", len(monitors)))
	fmt.Println()

	for i, m := range monitors {
		badge := ""
		if m.Primary {
			badge = "Primary"
		}
		ui.BoxStart(fmt.Sprintf("Monitor %d", i+1), badge)
		ui.BoxRow(fmt.Sprintf("%sResolution%s   %s%d × %d%s",
			ui.DkGray, ui.Reset, ui.BrWhite, m.Width, m.Height, ui.Reset))
		ui.BoxRow(fmt.Sprintf("%sPosition%s     %s(%d, %d)%s",
			ui.DkGray, ui.Reset, ui.White, m.X, m.Y, ui.Reset))
		ui.BoxEnd()
	}

	fmt.Println()
	return nil
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("\n %sqk%s %sv0.3.0%s %s%s quickstart terminal launcher%s\n\n",
			ui.BrCyan+ui.Bold, ui.Reset,
			ui.BrWhite, ui.Reset,
			ui.DkGray, ui.Dot, ui.Reset)
	},
}
