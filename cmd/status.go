package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/trokky/cli/internal/client"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show instance health, schemas, and connection info",
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := client.FromContext(cmd)
		if err != nil {
			return err
		}

		health, err := c.Health()
		if err != nil {
			return fmt.Errorf("instance unreachable: %w", err)
		}

		fmt.Printf("Instance:  %s\n", c.BaseURL)
		fmt.Printf("Status:    %s\n", health.Status)
		fmt.Printf("Version:   %s\n", health.Version)

		// List collections
		collections, err := c.ListCollections()
		if err == nil && len(collections) > 0 {
			fmt.Printf("\nCollections:\n")
			for _, col := range collections {
				stats, err := c.CollectionStats(col)
				if err == nil {
					fmt.Printf("  %-25s %d documents\n", col, stats.TotalDocuments)
				} else {
					fmt.Printf("  %s\n", col)
				}
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
