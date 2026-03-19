package cmd

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/trokky/cli/internal/backup"
	"github.com/trokky/cli/internal/client"
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Create a backup of a Trokky instance",
	Long: `Create a schema-driven backup as a zip archive.

Example:
  trokky backup --output backup.zip
  trokky backup --output backup.zip --collections posts,pages
  trokky backup --output backup.zip --skip-media`,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := client.FromContext(cmd)
		if err != nil {
			return err
		}

		output, _ := cmd.Flags().GetString("output")
		collectionsFlag, _ := cmd.Flags().GetString("collections")
		skipMedia, _ := cmd.Flags().GetBool("skip-media")
		description, _ := cmd.Flags().GetString("description")

		// Fetch schemas (collections)
		fmt.Print("Fetching schemas... ")
		schemasData, err := c.Get("/collections")
		if err != nil {
			fmt.Println("failed")
			return fmt.Errorf("failed to fetch schemas: %w", err)
		}

		allSchemas, err := backup.ParseSchemas(schemasData)
		if err != nil {
			return err
		}

		if len(allSchemas) == 0 {
			fmt.Println("no schemas found")
			return fmt.Errorf("no schemas found in target instance")
		}
		fmt.Printf("%d collection(s)\n", len(allSchemas))

		// Filter schemas if requested
		schemasToBackup := allSchemas
		if collectionsFlag != "" {
			requested := splitCSV(collectionsFlag)
			requestedSet := make(map[string]bool)
			for _, r := range requested {
				requestedSet[r] = true
			}
			schemasToBackup = nil
			for _, s := range allSchemas {
				if requestedSet[s.Name] {
					schemasToBackup = append(schemasToBackup, s)
				}
			}
			if len(schemasToBackup) == 0 {
				return fmt.Errorf("none of the requested collections exist")
			}
			fmt.Printf("Selected %d collection(s)\n", len(schemasToBackup))
		}

		// Build dependency graph
		dependencyGraph := backup.BuildDependencyGraph(schemasToBackup)
		restoreOrder := backup.GetRestoreOrder(dependencyGraph)
		fmt.Printf("Restore order: %v\n", restoreOrder)

		// Create zip file
		zipFile, err := os.Create(output)
		if err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}

		zw := zip.NewWriter(zipFile)

		// Backup documents
		collectionStats := make(map[string]int)
		totalDocuments := 0

		for _, schema := range schemasToBackup {
			fmt.Printf("  Backing up %s... ", schema.Name)

			data, err := c.Get("/collections/" + schema.Name + "?limit=10000")
			if err != nil {
				fmt.Printf("failed: %v\n", err)
				collectionStats[schema.Name] = 0
				continue
			}

			docs := backup.ParseDocuments(data)

			// Write each document as a separate file
			for i, docObj := range docs {
				docID := backup.ExtractDocID(docObj)
				if docID == "" {
					docID = fmt.Sprintf("doc-%d", i)
				}

				path := fmt.Sprintf("collections/%s/%s.json", schema.Name, docID)
				w, err := zw.Create(path)
				if err != nil {
					fmt.Printf("\n    Warning: failed to create zip entry %s: %v\n", path, err)
					continue
				}

				pretty, err := json.MarshalIndent(docObj, "", "  ")
				if err != nil {
					fmt.Printf("\n    Warning: failed to marshal document %s: %v\n", docID, err)
					continue
				}
				if _, err := w.Write(pretty); err != nil {
					return fmt.Errorf("failed to write to archive: %w", err)
				}
			}

			collectionStats[schema.Name] = len(docs)
			totalDocuments += len(docs)
			fmt.Printf("%d document(s)\n", len(docs))
		}

		// Backup media
		mediaIndex := make(map[string]backup.MediaFileInfo)
		mediaCount := 0

		if !skipMedia {
			fmt.Print("  Backing up media... ")

			mediaData, err := c.Get("/media?limit=10000")
			if err != nil {
				fmt.Printf("failed: %v\n", err)
			} else {
				var mediaItems []struct {
					ID       string `json:"id"`
					Filename string `json:"filename"`
					MimeType string `json:"mimeType"`
					Size     int64  `json:"size"`
				}

				if err := json.Unmarshal(mediaData, &mediaItems); err == nil {
					for _, item := range mediaItems {
						mediaURL := c.BaseURL + "/media/" + item.ID + "/file"
						req, err := http.NewRequest(http.MethodGet, mediaURL, nil)
						if err != nil {
							fmt.Printf("\n    Warning: invalid media URL for %s\n", item.ID)
							continue
						}
						req.Header.Set("Authorization", "Bearer "+c.Token)

						resp, err := c.HTTPClient.Do(req)
						if err != nil {
							fmt.Printf("\n    Warning: failed to download %s: %v\n", item.Filename, err)
							continue
						}

						if resp.StatusCode >= 400 {
							resp.Body.Close()
							fmt.Printf("\n    Warning: HTTP %d downloading %s\n", resp.StatusCode, item.Filename)
							continue
						}

						filename := item.Filename
						if filename == "" {
							filename = item.ID
						}

						// Stream directly to zip
						w, err := zw.Create("media/" + filename)
						if err != nil {
							resp.Body.Close()
							continue
						}

						written, err := io.Copy(w, resp.Body)
						resp.Body.Close()
						if err != nil {
							fmt.Printf("\n    Warning: failed to write media %s: %v\n", filename, err)
							continue
						}

						mediaIndex[item.ID] = backup.MediaFileInfo{
							Filename: filename,
							MimeType: item.MimeType,
							Size:     written,
						}
						mediaCount++
					}
				}

				fmt.Printf("%d file(s)\n", mediaCount)
			}
		}

		// Write manifest
		manifest := backup.BackupManifest{
			Version:   backup.ManifestVersion,
			Timestamp: time.Now().UTC().Format(time.RFC3339),
			Source: backup.BackupSource{
				URL:         c.BaseURL,
				Description: description,
			},
			Schemas:         schemasToBackup,
			DependencyGraph: dependencyGraph,
			RestoreOrder:    restoreOrder,
			MediaIndex:      mediaIndex,
			Statistics: backup.BackupStatistics{
				TotalDocuments: totalDocuments,
				TotalMedia:     mediaCount,
				Collections:    collectionStats,
			},
		}

		manifestData, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to serialize manifest: %w", err)
		}

		w, err := zw.Create("manifest.json")
		if err != nil {
			return fmt.Errorf("failed to create manifest entry: %w", err)
		}
		if _, err := w.Write(manifestData); err != nil {
			return fmt.Errorf("failed to write manifest: %w", err)
		}

		// Close zip to flush central directory
		if err := zw.Close(); err != nil {
			return fmt.Errorf("failed to finalize archive: %w", err)
		}

		// Get file size
		var sizeMB float64
		if info, err := zipFile.Stat(); err == nil {
			sizeMB = float64(info.Size()) / 1024 / 1024
		}

		if err := zipFile.Close(); err != nil {
			return fmt.Errorf("failed to close archive: %w", err)
		}

		fmt.Println()
		fmt.Println("Backup Summary")
		fmt.Println("──────────────────────────────────────────────────")
		fmt.Printf("Output file:     %s\n", output)
		fmt.Printf("Documents:       %d\n", totalDocuments)
		fmt.Printf("Media files:     %d\n", mediaCount)
		fmt.Printf("Collections:     %d\n", len(schemasToBackup))
		fmt.Printf("Archive size:    %.2f MB\n", sizeMB)
		fmt.Println("──────────────────────────────────────────────────")

		return nil
	},
}

func splitCSV(s string) []string {
	var result []string
	for _, part := range strings.Split(s, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func init() {
	backupCmd.Flags().String("output", "", "output file path (e.g., backup.zip)")
	backupCmd.MarkFlagRequired("output")
	backupCmd.Flags().String("collections", "", "comma-separated list of collections to backup")
	backupCmd.Flags().Bool("skip-media", false, "skip media files")
	backupCmd.Flags().String("description", "", "backup description")
	rootCmd.AddCommand(backupCmd)
}
