package cmd

import (
	"fmt"
	"strings"

	"github.com/bcmister/qs/internal/config"
	"github.com/bcmister/qs/internal/session"
	"github.com/bcmister/qs/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

// compile-time assertion that the real engine satisfies the contract the
// dashboard drives.
var _ session.SessionEngine = (*session.PsmuxEngine)(nil)

var dashCmd = &cobra.Command{
	Use:   "dash",
	Short: "Open the session dashboard (command center)",
	RunE:  runDash,
}

func runDash(cmd *cobra.Command, args []string) error {
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

	keys, _ := config.LoadKeys()

	// The resolver is the firewall seam: the engine builds every child
	// environment from THIS function (a clean allowlisted base plus the
	// project's profile layers), never from os.Environ().
	resolver := session.LaunchResolver(func(projectID, accountID string) (env []string, command string, cmdArgs []string, dir string, rerr error) {
		e, spec, resolveErr := config.ResolveSessionEnv(cfg, keys, projectID, accountID)
		if resolveErr != nil {
			return nil, "", nil, "", resolveErr
		}
		return e, spec.Command, spec.Args, spec.Dir, nil
	})

	engine := session.NewPsmuxEngine(resolver)
	defer engine.Close()

	p := tea.NewProgram(tui.NewDashboard(cfg, engine), tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		return err
	}
	if m, ok := finalModel.(tui.DashboardModel); ok && m.Err() != nil {
		return fmt.Errorf("session attach exited with error: %w", m.Err())
	}
	return nil
}
