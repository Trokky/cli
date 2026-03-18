package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/trokky/cli/internal/client"
)

var exportCmd = &cobra.Command{
	Use:   "export [collection] [output-file]",
	Short: "Export a collection or singleton to JSON",
	Long: `Export content from a Trokky instance to a JSON file.

Example:
  trokky export article articles.json
  trokky export --all ./export/`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := client.FromContext(cmd)
		if err != nil {
			return err
		}

		collection := args[0]

		data, err := c.ExportCollection(collection)
		if err != nil {
			return fmt.Errorf("failed to export %s: %w", collection, err)
		}

		// Write to file or stdout
		if len(args) > 1 {
			if err := os.WriteFile(args[1], data, 0644); err != nil {
				return fmt.Errorf("failed to write file: %w", err)
			}
			fmt.Printf("✓ Exported %s → %s\n", collection, args[1])
		} else {
			fmt.Println(string(data))
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(exportCmd)
}
