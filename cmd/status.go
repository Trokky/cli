package cmd

import (
	"fmt"
	"os"
	"time"

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

		fmt.Printf("Instance:    %s\n", c.BaseURL)

		// Health check with timing
		start := time.Now()
		health, err := c.Health()
		latency := time.Since(start)

		if err != nil {
			fmt.Fprintf(os.Stderr, "Status:      DOWN\n")
			fmt.Fprintf(os.Stderr, "Error:       %v\n", err)
			return nil
		}

		fmt.Printf("Status:      UP (%dms)\n", latency.Milliseconds())
		if health.Version != "" {
			fmt.Printf("Version:     %s\n", health.Version)
		}

		// List collections
		collections, err := c.ListCollections()
		if err == nil && len(collections) > 0 {
			fmt.Printf("\nCollections: %d\n", len(collections))
			for _, col := range collections {
				stats, err := c.CollectionStats(col)
				if err == nil {
					fmt.Printf("  %-25s %d documents (%d published, %d draft)\n",
						col, stats.TotalDocuments, stats.PublishedDocuments, stats.DraftDocuments)
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
