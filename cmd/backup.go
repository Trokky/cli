package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/trokky/cli/internal/client"
)

var backupCmd = &cobra.Command{
	Use:   "backup [output-dir]",
	Short: "Backup all content and media from an instance",
	Long: `Export all singletons, collections, and media files to a local directory.

Example:
  trokky backup
  trokky backup ./backups/2024-01-15
  trokky backup --include-media=false`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := client.FromContext(cmd)
		if err != nil {
			return err
		}

		// Determine output directory
		outputDir := fmt.Sprintf("trokky-backup-%s", time.Now().Format("2006-01-02-150405"))
		if len(args) > 0 {
			outputDir = args[0]
		}

		includeMedia, _ := cmd.Flags().GetBool("include-media")

		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}

		fmt.Printf("Backing up %s → %s\n\n", c.BaseURL, outputDir)

		// Export collections
		collections, err := c.ListCollections()
		if err != nil {
			return fmt.Errorf("failed to list collections: %w", err)
		}

		collectionsDir := filepath.Join(outputDir, "collections")
		if err := os.MkdirAll(collectionsDir, 0755); err != nil {
			return err
		}

		for _, col := range collections {
			data, err := c.ExportCollection(col)
			if err != nil {
				fmt.Printf("  ✗ %s: %v\n", col, err)
				continue
			}
			outFile := filepath.Join(collectionsDir, col+".json")
			if err := os.WriteFile(outFile, data, 0644); err != nil {
				return err
			}
			fmt.Printf("  ✓ %s\n", col)
		}

		// Export media
		if includeMedia {
			fmt.Printf("\nBacking up media...\n")
			mediaDir := filepath.Join(outputDir, "media")
			count, err := c.ExportMedia(mediaDir)
			if err != nil {
				fmt.Printf("  ✗ media: %v\n", err)
			} else {
				fmt.Printf("  ✓ %d media files\n", count)
			}
		}

		fmt.Printf("\n✓ Backup complete: %s\n", outputDir)
		return nil
	},
}

func init() {
	backupCmd.Flags().Bool("include-media", true, "include media files in backup")
	rootCmd.AddCommand(backupCmd)
}
