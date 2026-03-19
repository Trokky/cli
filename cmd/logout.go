package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/trokky/cli/internal/config"
)

var logoutCmd = &cobra.Command{
	Use:   "logout [instance-name]",
	Short: "Remove stored credentials for an instance",
	Long: `Remove stored credentials for a named instance.
If no name is given, removes the default instance.

This is a shortcut for 'trokky config remove <name>'.

Example:
  trokky logout
  trokky logout staging`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var name string
		if len(args) > 0 {
			name = args[0]
		} else {
			defaultName, _, err := config.GetDefaultInstance()
			if err != nil {
				return err
			}
			if defaultName == "" {
				return fmt.Errorf("no default instance configured")
			}
			name = defaultName
		}

		force, _ := cmd.Flags().GetBool("force")
		if !force {
			ok, err := confirmPrompt(fmt.Sprintf("Remove instance %q?", name))
			if err != nil {
				return err
			}
			if !ok {
				fmt.Println("Cancelled")
				return nil
			}
		}

		removed, err := config.RemoveInstance(name)
		if err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		if !removed {
			return fmt.Errorf("instance %q not found", name)
		}

		fmt.Printf("✓ Logged out from %q\n", name)
		return nil
	},
}

func init() {
	logoutCmd.Flags().Bool("force", false, "skip confirmation prompt")
	rootCmd.AddCommand(logoutCmd)
}
