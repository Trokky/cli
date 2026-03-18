package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/trokky/cli/internal/client"
)

var restoreCmd = &cobra.Command{
	Use:   "restore [backup-dir]",
	Short: "Restore content and media from a backup",
	Long: `Import all singletons, collections, and media files from a backup directory.

Example:
  trokky restore ./trokky-backup-2024-01-15-120000
  trokky restore ./backups/latest --include-media=false`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := client.FromContext(cmd)
		if err != nil {
			return err
		}

		backupDir := args[0]
		includeMedia, _ := cmd.Flags().GetBool("include-media")

		if _, err := os.Stat(backupDir); os.IsNotExist(err) {
			return fmt.Errorf("backup directory does not exist: %s", backupDir)
		}

		fmt.Printf("Restoring %s → %s\n\n", backupDir, c.BaseURL)

		// Import collections
		collectionsDir := filepath.Join(backupDir, "collections")
		entries, err := os.ReadDir(collectionsDir)
		if err == nil {
			for _, entry := range entries {
				if !strings.HasSuffix(entry.Name(), ".json") {
					continue
				}
				col := strings.TrimSuffix(entry.Name(), ".json")
				data, err := os.ReadFile(filepath.Join(collectionsDir, entry.Name()))
				if err != nil {
					fmt.Printf("  ✗ %s: %v\n", col, err)
					continue
				}
				count, err := c.ImportCollection(col, data)
				if err != nil {
					fmt.Printf("  ✗ %s: %v\n", col, err)
					continue
				}
				fmt.Printf("  ✓ %s (%d documents)\n", col, count)
			}
		}

		// Import media
		if includeMedia {
			mediaDir := filepath.Join(backupDir, "media")
			if _, err := os.Stat(mediaDir); err == nil {
				fmt.Printf("\nRestoring media...\n")
				count, err := c.ImportMedia(mediaDir)
				if err != nil {
					fmt.Printf("  ✗ media: %v\n", err)
				} else {
					fmt.Printf("  ✓ %d media files\n", count)
				}
			}
		}

		fmt.Printf("\n✓ Restore complete\n")
		return nil
	},
}

func init() {
	restoreCmd.Flags().Bool("include-media", true, "include media files in restore")
	rootCmd.AddCommand(restoreCmd)
}
