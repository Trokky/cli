package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/trokky/cli/internal/auth"
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

		// Revoke the sign-in on the instance first, so it also disappears from its Studio's
		// connected applications; best effort, an unreachable instance must not block logout
		inst, _ := config.GetInstance(name)
		var revokeErr error
		if inst != nil && inst.AuthType == config.AuthTypeOAuth2 && (inst.RefreshToken != "" || inst.Token != "") {
			revokeErr = auth.RevokeToken(*inst)
		}

		removed, err := config.RemoveInstance(name)
		if err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}
		if !removed {
			return fmt.Errorf("instance %q not found", name)
		}

		fmt.Printf("✓ Logged out from %q\n", name)
		var refused *auth.RevokeRefusedError
		if errors.As(revokeErr, &refused) {
			fmt.Printf("  The instance did not accept the revocation (HTTP %d); revoke the access in its Studio under Account > Connected applications.\n", refused.Status)
		} else if revokeErr != nil {
			fmt.Println("  The instance could not be reached to revoke the sign-in; revoke it in its Studio under Account > Connected applications.")
		}
		return nil
	},
}

func init() {
	logoutCmd.Flags().Bool("force", false, "skip confirmation prompt")
	rootCmd.AddCommand(logoutCmd)
}
