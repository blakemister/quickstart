package cmd

import (
	"fmt"
	"strings"

	"github.com/bcmister/qs/internal/config"
	"github.com/spf13/cobra"
)

var (
	profileID          string
	profileLabel       string
	profileGitName     string
	profileGitEmail    string
	profileColor       string
	profileAccountDirs []string
)

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Manage profiles (identity/secret contexts)",
}

var profileAddCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a profile",
	RunE:  runProfileAdd,
}

var profileListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured profiles",
	RunE:  runProfileList,
}

func init() {
	profileAddCmd.Flags().StringVar(&profileID, "id", "", "Profile ID (required, unique)")
	profileAddCmd.Flags().StringVar(&profileLabel, "label", "", "Human-readable label")
	profileAddCmd.Flags().StringVar(&profileGitName, "git-name", "", "Git author/committer name")
	profileAddCmd.Flags().StringVar(&profileGitEmail, "git-email", "", "Git author/committer email")
	profileAddCmd.Flags().StringVar(&profileColor, "color", "", "Display color")
	profileAddCmd.Flags().StringArrayVar(&profileAccountDirs, "account-dir", nil,
		"Per-tool isolated config dir as accountID=path (repeatable)")

	profileCmd.AddCommand(profileAddCmd)
	profileCmd.AddCommand(profileListCmd)
}

func runProfileAdd(cmd *cobra.Command, args []string) error {
	if strings.TrimSpace(profileID) == "" {
		return fmt.Errorf("profile add: --id is required")
	}

	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	config.EnsureDefaults(cfg)

	dirs, err := parseKeyValues(profileAccountDirs)
	if err != nil {
		return fmt.Errorf("profile add: --account-dir: %w", err)
	}

	label := profileLabel
	if label == "" {
		label = profileID
	}

	p := config.Profile{
		ID:    profileID,
		Label: label,
		Color: profileColor,
		GitIdentity: config.GitIdentity{
			Name:  profileGitName,
			Email: profileGitEmail,
		},
		AccountConfigDirs: dirs,
	}

	if err := config.AddProfile(cfg, p); err != nil {
		return err
	}
	if err := config.Save(cfg, ""); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	fmt.Printf("Added profile %q\n", p.ID)
	return nil
}

func runProfileList(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load("")
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	config.EnsureDefaults(cfg)

	if len(cfg.Profiles) == 0 {
		fmt.Println("No profiles configured.")
		return nil
	}
	for _, p := range cfg.Profiles {
		fmt.Printf("%-16s %s\n", p.ID, p.Label)
		if p.GitIdentity.Name != "" || p.GitIdentity.Email != "" {
			fmt.Printf("  git: %s <%s>\n", p.GitIdentity.Name, p.GitIdentity.Email)
		}
		for tool, dir := range p.AccountConfigDirs {
			fmt.Printf("  %s dir: %s\n", tool, dir)
		}
	}
	return nil
}

// parseKeyValues parses repeated "key=value" entries into a map. An entry with
// no '=' or an empty key is rejected so a typo never silently drops config.
func parseKeyValues(entries []string) (map[string]string, error) {
	if len(entries) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		eq := strings.IndexByte(e, '=')
		if eq <= 0 {
			return nil, fmt.Errorf("expected key=value, got %q", e)
		}
		out[e[:eq]] = e[eq+1:]
	}
	return out, nil
}
