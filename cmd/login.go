package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/trokky/cli/internal/auth"
	"github.com/trokky/cli/internal/config"
)

var loginCmd = &cobra.Command{
	Use:   "login [instance-url]",
	Short: "Authenticate with a Trokky instance",
	Long: `Login to a Trokky instance using username/password.
The API token will be stored locally for subsequent commands.

Example:
  trokky login https://cms.example.com
  trokky login http://localhost:3000`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		instanceURL := args[0]

		username, _ := cmd.Flags().GetString("username")
		password, _ := cmd.Flags().GetString("password")

		if username == "" || password == "" {
			var err error
			username, password, err = auth.PromptCredentials()
			if err != nil {
				return fmt.Errorf("failed to read credentials: %w", err)
			}
		}

		token, err := auth.Login(instanceURL, username, password)
		if err != nil {
			return fmt.Errorf("login failed: %w", err)
		}

		cfg, err := config.Load()
		if err != nil {
			cfg = config.New()
		}

		cfg.SetInstance(instanceURL, token)
		cfg.ActiveInstance = instanceURL

		if err := cfg.Save(); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Printf("✓ Logged in to %s\n", instanceURL)
		return nil
	},
}

func init() {
	loginCmd.Flags().StringP("username", "u", "", "username")
	loginCmd.Flags().StringP("password", "p", "", "password")
	rootCmd.AddCommand(loginCmd)
}
