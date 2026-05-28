package cmd

import (
	"fmt"
	"sort"

	"github.com/bcmister/qs/internal/config"
	"github.com/spf13/cobra"
)

var keysCmd = &cobra.Command{
	Use:   "keys",
	Short: "Manage per-profile secrets",
}

var keysSetCmd = &cobra.Command{
	Use:   "set <profileID> <NAME> <VALUE>",
	Short: "Set a secret env var in a profile's namespace",
	Args:  cobra.ExactArgs(3),
	RunE:  runKeysSet,
}

var keysListCmd = &cobra.Command{
	Use:   "list <profileID>",
	Short: "List a profile's secret names (values masked)",
	Args:  cobra.ExactArgs(1),
	RunE:  runKeysList,
}

func init() {
	keysCmd.AddCommand(keysSetCmd)
	keysCmd.AddCommand(keysListCmd)
}

func runKeysSet(cmd *cobra.Command, args []string) error {
	profileID, name, value := args[0], args[1], args[2]

	keys, err := config.LoadKeys()
	if err != nil {
		return fmt.Errorf("failed to load keys: %w", err)
	}
	if err := config.SetProfileKey(keys, profileID, name, value); err != nil {
		return err
	}
	if err := config.SaveKeys(keys); err != nil {
		return fmt.Errorf("failed to save keys: %w", err)
	}

	// Never echo the value.
	fmt.Printf("Set %s for profile %q\n", name, profileID)
	return nil
}

func runKeysList(cmd *cobra.Command, args []string) error {
	profileID := args[0]

	keys, err := config.LoadKeys()
	if err != nil {
		return fmt.Errorf("failed to load keys: %w", err)
	}

	profileKeys := config.KeysForProfile(keys, profileID)
	if len(profileKeys) == 0 {
		fmt.Printf("No secrets set for profile %q.\n", profileID)
		return nil
	}

	names := make([]string, 0, len(profileKeys))
	for name := range profileKeys {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		fmt.Printf("%-28s %s\n", name, config.MaskValue(profileKeys[name]))
	}
	return nil
}
