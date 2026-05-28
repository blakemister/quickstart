package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bcmister/qs/internal/config"
	"github.com/spf13/cobra"
)

var (
	projectProfile        string
	projectAccounts       []string
	projectServices       []string
	projectDefaultAccount string
	projectIDFlag         string
	projectLabel          string
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage projects (directory bindings)",
}

var projectAddCmd = &cobra.Command{
	Use:   "add <path>",
	Short: "Bind a directory to a profile, accounts, and services",
	Args:  cobra.ExactArgs(1),
	RunE:  runProjectAdd,
}

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured projects",
	RunE:  runProjectList,
}

func init() {
	projectAddCmd.Flags().StringVar(&projectIDFlag, "id", "", "Project ID (defaults to the folder name)")
	projectAddCmd.Flags().StringVar(&projectLabel, "label", "", "Human-readable label")
	projectAddCmd.Flags().StringVar(&projectProfile, "profile", "", "Profile ID to bind")
	projectAddCmd.Flags().StringSliceVar(&projectAccounts, "accounts", nil, "Account IDs (comma-separated)")
	projectAddCmd.Flags().StringSliceVar(&projectServices, "services", nil, "Service IDs (comma-separated)")
	projectAddCmd.Flags().StringVar(&projectDefaultAccount, "default-account", "", "Default account ID for this project")

	projectCmd.AddCommand(projectAddCmd)
	projectCmd.AddCommand(projectListCmd)
}

func runProjectAdd(cmd *cobra.Command, args []string) error {
	path, err := filepath.Abs(strings.TrimSpace(args[0]))
	if err != nil {
		return fmt.Errorf("project add: invalid path: %w", err)
	}

	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	config.EnsureDefaults(cfg)

	id := strings.TrimSpace(projectIDFlag)
	if id == "" {
		id = filepath.Base(path)
	}
	label := projectLabel
	if label == "" {
		label = id
	}

	p := config.Project{
		ID:             id,
		Label:          label,
		Path:           path,
		Profile:        projectProfile,
		Accounts:       projectAccounts,
		DefaultAccount: projectDefaultAccount,
		Services:       projectServices,
	}

	if err := config.AddProject(cfg, p); err != nil {
		return err
	}
	if err := config.Save(cfg, ""); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Added project %q -> %s\n", p.ID, p.Path)
	return nil
}

func runProjectList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	config.EnsureDefaults(cfg)

	if len(cfg.Projects) == 0 {
		fmt.Println("No projects configured.")
		return nil
	}
	for _, p := range cfg.Projects {
		fmt.Printf("%-16s %s\n", p.ID, p.Path)
		if p.Profile != "" {
			fmt.Printf("  profile: %s\n", p.Profile)
		}
		if len(p.Accounts) > 0 {
			fmt.Printf("  accounts: %s\n", strings.Join(p.Accounts, ", "))
		}
		if p.DefaultAccount != "" {
			fmt.Printf("  default account: %s\n", p.DefaultAccount)
		}
		if len(p.Services) > 0 {
			fmt.Printf("  services: %s\n", strings.Join(p.Services, ", "))
		}
	}
	return nil
}
