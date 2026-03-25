package cmd

import (
	"github.com/bcmister/qs/internal/config"
	"github.com/bcmister/qs/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Run the setup wizard",
	RunE:  runSetup,
}

func runSetup(cmd *cobra.Command, args []string) error {
	// Load existing config if available (nil is fine for first run)
	existing, _ := config.Load("")

	setup := tui.NewSetup(existing)
	p := tea.NewProgram(setup, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
