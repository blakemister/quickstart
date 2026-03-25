package cmd

import (
	"fmt"

	"github.com/bcmister/qs/internal/monitor"
	"github.com/bcmister/qs/internal/tui"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

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

	fmt.Println()
	fmt.Printf(" %s Detected %d monitors\n\n",
		tui.TitleStyle.Render("◆"),
		len(monitors))

	for i, m := range monitors {
		badge := ""
		if m.Primary {
			badge = " Primary"
		}

		title := fmt.Sprintf("Monitor %d%s", i+1, badge)
		res := fmt.Sprintf("%d × %d", m.Width, m.Height)
		pos := fmt.Sprintf("(%d, %d)", m.X, m.Y)

		box := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(tui.ColorDimGray).
			Padding(0, 1).
			Width(38)

		content := fmt.Sprintf("%s\n%s  %s\n%s  %s",
			tui.SubtitleStyle.Render(title),
			tui.DimStyle.Render("Resolution"),
			tui.WhiteStyle.Render(res),
			tui.DimStyle.Render("Position  "),
			tui.DimStyle.Render(pos))

		fmt.Println("   " + box.Render(content))
	}

	fmt.Println()
	return nil
}
