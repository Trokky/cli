package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/trokky/cli/internal/backup"
	"github.com/trokky/cli/internal/client"
)

var cleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Delete all content from a Trokky instance",
	Long: `Clean (delete) all content from a Trokky instance.
Requires --confirm flag for actual deletion.

Example:
  trokky clean --dry-run
  trokky clean --confirm
  trokky clean --collections posts,pages --confirm
  trokky clean --documents-only --confirm`,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := client.FromContext(cmd)
		if err != nil {
			return err
		}

		collectionsFlag, _ := cmd.Flags().GetString("collections")
		mediaOnly, _ := cmd.Flags().GetBool("media-only")
		documentsOnly, _ := cmd.Flags().GetBool("documents-only")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		confirm, _ := cmd.Flags().GetBool("confirm")

		if mediaOnly && documentsOnly {
			return fmt.Errorf("--media-only and --documents-only are mutually exclusive")
		}

		if !dryRun && !confirm {
			fmt.Fprintf(os.Stderr, `WARNING: This operation will PERMANENTLY DELETE content from:
   %s

To proceed, add --confirm flag:
   trokky clean --confirm

Or use --dry-run to preview what would be deleted first.
`, c.BaseURL)
			return fmt.Errorf("confirmation required for destructive operation")
		}

		if dryRun {
			fmt.Println("[DRY RUN] No actual deletions will be performed")
		}

		totalDocsDeleted := 0
		totalMediaDeleted := 0

		// Clean documents
		if !mediaOnly {
			var collectionsToClean []string

			if collectionsFlag != "" {
				// Use user-specified collections directly
				collectionsToClean = splitCSV(collectionsFlag)
			} else {
				// Discover all collections
				fmt.Print("Discovering collections... ")
				schemasData, err := c.Get("/collections")
				if err != nil {
					fmt.Println("failed")
					return fmt.Errorf("failed to discover collections: %w", err)
				}

				schemas, err := backup.ParseSchemas(schemasData)
				if err != nil {
					fmt.Println("failed")
					return err
				}

				collectionsToClean = make([]string, len(schemas))
				for i, s := range schemas {
					collectionsToClean[i] = s.Name
				}
				fmt.Printf("%d collection(s)\n", len(collectionsToClean))
			}

			for _, collection := range collectionsToClean {
				data, err := c.Get("/collections/" + collection + "?limit=10000")
				if err != nil {
					fmt.Fprintf(os.Stderr, "  Warning: failed to list %s: %v\n", collection, err)
					continue
				}

				docs := backup.ParseDocuments(data)

				if len(docs) == 0 {
					fmt.Printf("  %s: no documents\n", collection)
					continue
				}

				if dryRun {
					fmt.Printf("  [DRY RUN] Would delete %d document(s) in %s\n", len(docs), collection)
					totalDocsDeleted += len(docs)
					continue
				}

				deleted := 0
				for _, doc := range docs {
					docID := backup.ExtractDocID(doc)
					if docID == "" {
						continue
					}
					if _, err := c.Delete("/collections/" + collection + "/" + docID); err != nil {
						fmt.Fprintf(os.Stderr, "  Warning: failed to delete %s/%s: %v\n", collection, docID, err)
					} else {
						deleted++
					}
				}

				totalDocsDeleted += deleted
				fmt.Printf("  %s: deleted %d/%d document(s)\n", collection, deleted, len(docs))
			}
		}

		// Clean media (paginated, with rate-limit handling)
		if !documentsOnly {
			fmt.Print("Cleaning media files...\n")

			totalMediaCount := 0
			for {
				mediaData, err := c.Get("/media?limit=100")
				if err != nil {
					fmt.Fprintf(os.Stderr, "  Warning: failed to list media: %v\n", err)
					break
				}

				var mediaItems []struct {
					ID       string `json:"id"`
					Filename string `json:"filename"`
				}
				if err := json.Unmarshal(mediaData, &mediaItems); err != nil {
					fmt.Fprintf(os.Stderr, "  Warning: failed to parse media list: %v\n", err)
					break
				}

				if len(mediaItems) == 0 {
					break
				}

				if totalMediaCount == 0 && dryRun {
					// For dry-run, just report the first batch count
					fmt.Printf("  [DRY RUN] Would delete media files (at least %d)\n", len(mediaItems))
					totalMediaDeleted = len(mediaItems)
					break
				}

				totalMediaCount += len(mediaItems)
				for _, item := range mediaItems {
					if dryRun {
						continue
					}
					// Retry with backoff on rate limit
					for attempt := 1; attempt <= 3; attempt++ {
						_, err := c.Delete("/media/" + item.ID)
						if err == nil {
							totalMediaDeleted++
							break
						}
						if attempt < 3 && (strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "rate")) {
							time.Sleep(time.Duration(attempt) * time.Second)
							continue
						}
						fmt.Fprintf(os.Stderr, "  Warning: failed to delete media %s: %v\n", item.ID, err)
						break
					}
					time.Sleep(100 * time.Millisecond)
				}

				fmt.Printf("  Deleted %d media file(s) so far...\n", totalMediaDeleted)
			}

			if totalMediaDeleted > 0 || totalMediaCount == 0 {
				fmt.Printf("  Media cleanup done: %d file(s) deleted\n", totalMediaDeleted)
			}
		}

		// Summary
		fmt.Println()
		mode := "Live deletion"
		if dryRun {
			mode = "Dry run"
		}

		fmt.Println("Clean Summary")
		fmt.Println("──────────────────────────────────────────────────")
		if !mediaOnly {
			fmt.Printf("Documents:       %d\n", totalDocsDeleted)
		}
		if !documentsOnly {
			fmt.Printf("Media files:     %d\n", totalMediaDeleted)
		}
		fmt.Printf("Instance:        %s\n", c.BaseURL)
		fmt.Printf("Mode:            %s\n", mode)
		fmt.Println("──────────────────────────────────────────────────")

		return nil
	},
}

func init() {
	cleanCmd.Flags().String("collections", "", "comma-separated list of collections to clean")
	cleanCmd.Flags().Bool("media-only", false, "clean only media files")
	cleanCmd.Flags().Bool("documents-only", false, "clean only documents")
	cleanCmd.Flags().Bool("dry-run", false, "preview what would be deleted")
	cleanCmd.Flags().Bool("confirm", false, "confirm destructive operation")
	rootCmd.AddCommand(cleanCmd)
}
