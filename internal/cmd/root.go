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
	Short: "Quick project picker for terminal",
	RunE:  runQk,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.AddCommand(setCmd)
	rootCmd.AddCommand(monitorsCmd)
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(allCmd)
}

func runQk(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load("")
	if err != nil {
		if os.IsNotExist(err) {
			cfg = &config.Config{
				ProjectsRoot: config.DefaultProjectsRoot(),
			}
		} else {
			return fmt.Errorf("failed to load config: %w", err)
		}
	}
	// Run picker directly - no UI chrome, fastest path
	return window.RunPickerInCurrent(cfg.ProjectsRoot, config.Command)
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
