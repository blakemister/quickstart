package cmd

import (
	"fmt"

	"github.com/bcmister/qs/internal/tui"
	"github.com/spf13/cobra"
)

// version is set at build time via -ldflags -X github.com/bcmister/qs/internal/cmd.version=...
var version = "v0.4.0"

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println()
		fmt.Print(tui.RenderMascot())
		fmt.Printf(" %s %s %s\n\n",
			tui.TitleStyle.Render("qs"),
			tui.SubtitleStyle.Render(version),
			tui.DimStyle.Render("· quickstart terminal launcher"))
	},
}
