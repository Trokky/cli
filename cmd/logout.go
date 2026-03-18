package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/trokky/cli/internal/config"
)

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Remove stored credentials for the active instance",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("not logged in")
		}

		if cfg.ActiveInstance == "" {
			return fmt.Errorf("no active instance")
		}

		instance := cfg.ActiveInstance
		delete(cfg.Instances, instance)
		cfg.ActiveInstance = ""

		if err := cfg.Save(); err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Printf("✓ Logged out from %s\n", instance)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(logoutCmd)
}
