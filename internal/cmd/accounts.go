package cmd

import (
	"fmt"
	"os"

	"github.com/bcmister/qs/internal/config"
	"github.com/bcmister/qs/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var accountsCmd = &cobra.Command{
	Use:   "accounts",
	Short: "Manage AI tool accounts",
	RunE:  runAccounts,
}

func runAccounts(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load("")
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println()
			fmt.Println("  No configuration found. Run `qs setup` first.")
			fmt.Println()
			return nil
		}
		return fmt.Errorf("failed to load config: %w", err)
	}

	config.EnsureDefaults(cfg)
	accounts := tui.NewAccounts(cfg)
	p := tea.NewProgram(accounts, tea.WithAltScreen())
	_, err = p.Run()
	return err
}
