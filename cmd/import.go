package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/trokky/cli/internal/client"
)

var importCmd = &cobra.Command{
	Use:   "import [collection] [input-file]",
	Short: "Import documents into a collection from JSON",
	Long: `Import content from a JSON file into a Trokky instance.

Example:
  trokky import article articles.json
  trokky import rapport rapports.json --upsert`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := client.FromContext(cmd)
		if err != nil {
			return err
		}

		collection := args[0]
		inputFile := args[1]

		data, err := os.ReadFile(inputFile)
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}

		count, err := c.ImportCollection(collection, data)
		if err != nil {
			return fmt.Errorf("failed to import %s: %w", collection, err)
		}

		fmt.Printf("✓ Imported %d documents into %s\n", count, collection)
		return nil
	},
}

func init() {
	importCmd.Flags().Bool("upsert", false, "update existing documents instead of skipping")
	rootCmd.AddCommand(importCmd)
}
