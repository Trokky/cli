package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/trokky/cli/internal/client"
)

var generateTypesCmd = &cobra.Command{
	Use:   "generate-types",
	Short: "Generate TypeScript types from instance schemas",
	Long: `Fetch schemas from a Trokky instance and generate TypeScript type definitions.

Example:
  trokky generate-types -o ./src/types/trokky
  trokky generate-types --instance https://cms.example.com -o ./types`,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := client.FromContext(cmd)
		if err != nil {
			return err
		}

		outputDir, _ := cmd.Flags().GetString("output")
		if outputDir == "" {
			outputDir = "./src/types/trokky"
		}

		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}

		types, err := c.GenerateTypes()
		if err != nil {
			return fmt.Errorf("failed to generate types: %w", err)
		}

		outFile := filepath.Join(outputDir, "index.ts")
		if err := os.WriteFile(outFile, []byte(types), 0644); err != nil {
			return fmt.Errorf("failed to write types: %w", err)
		}

		fmt.Printf("✓ Types generated → %s\n", outFile)
		return nil
	},
}

func init() {
	generateTypesCmd.Flags().StringP("output", "o", "", "output directory (default ./src/types/trokky)")
	rootCmd.AddCommand(generateTypesCmd)
}
