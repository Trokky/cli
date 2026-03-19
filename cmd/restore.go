package cmd

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/trokky/cli/internal/backup"
	"github.com/trokky/cli/internal/client"
)

var restoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore content from a Trokky backup file",
	Long: `Restore content and media from a zip backup created by 'trokky backup'.

Example:
  trokky restore --input backup.zip
  trokky restore --input backup.zip --collections posts,pages
  trokky restore --input backup.zip --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := client.FromContext(cmd)
		if err != nil {
			return err
		}

		input, _ := cmd.Flags().GetString("input")
		collectionsFlag, _ := cmd.Flags().GetString("collections")
		withDeps, _ := cmd.Flags().GetBool("with-dependencies")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		clean, _ := cmd.Flags().GetBool("clean")
		overwrite, _ := cmd.Flags().GetBool("overwrite")

		// Open zip
		zr, err := zip.OpenReader(input)
		if err != nil {
			return fmt.Errorf("failed to open backup file: %w", err)
		}
		defer zr.Close()

		// Read manifest
		manifest, err := readManifestFromZip(&zr.Reader)
		if err != nil {
			return err
		}

		if manifest.Version != backup.ManifestVersion {
			return fmt.Errorf("unsupported backup version: %s (expected %s)", manifest.Version, backup.ManifestVersion)
		}

		fmt.Printf("Backup from %s\n", manifest.Timestamp)
		if manifest.Source.URL != "" {
			fmt.Printf("Source: %s\n", manifest.Source.URL)
		}

		// Determine collections to restore
		collectionsToRestore := make([]string, len(manifest.Schemas))
		for i, s := range manifest.Schemas {
			collectionsToRestore[i] = s.Name
		}

		if collectionsFlag != "" {
			requested := splitCSV(collectionsFlag)

			if withDeps {
				withDepsSet := make(map[string]bool)
				for _, r := range requested {
					withDepsSet[r] = true
					for _, dep := range manifest.DependencyGraph[r] {
						withDepsSet[dep] = true
					}
				}
				requested = nil
				for k := range withDepsSet {
					requested = append(requested, k)
				}
			}

			// Validate against backup
			backupSchemaSet := make(map[string]bool)
			for _, s := range manifest.Schemas {
				backupSchemaSet[s.Name] = true
			}
			var missing []string
			for _, r := range requested {
				if !backupSchemaSet[r] {
					missing = append(missing, r)
				}
			}
			if len(missing) > 0 {
				return fmt.Errorf("collections not found in backup: %s", strings.Join(missing, ", "))
			}

			collectionsToRestore = requested
			fmt.Printf("Selected %d collection(s) for restore\n", len(collectionsToRestore))
		}

		// Build schema map and restore order
		schemaMap := make(map[string]backup.SchemaDefinition)
		for _, s := range manifest.Schemas {
			schemaMap[s.Name] = s
		}

		restoreSet := make(map[string]bool)
		for _, name := range collectionsToRestore {
			restoreSet[name] = true
		}

		// Use manifest restore order, filtered to selected collections
		addedToOrder := make(map[string]bool)
		var restoreOrder []string
		for _, name := range manifest.RestoreOrder {
			if restoreSet[name] {
				restoreOrder = append(restoreOrder, name)
				addedToOrder[name] = true
			}
		}
		for _, name := range collectionsToRestore {
			if !addedToOrder[name] {
				restoreOrder = append(restoreOrder, name)
			}
		}

		fmt.Printf("Restore order: %v\n", restoreOrder)

		if dryRun {
			fmt.Println("\n[DRY RUN] No changes will be made")
		}

		// Pre-flight schema validation
		fmt.Print("Validating target instance... ")
		targetData, err := c.Get("/collections")
		if err != nil {
			fmt.Println("failed")
			return fmt.Errorf("failed to fetch target schemas: %w", err)
		}

		targetSchemas, err := backup.ParseSchemas(targetData)
		if err != nil {
			fmt.Println("failed")
			return fmt.Errorf("failed to parse target schemas: %w", err)
		}

		schemasToValidate := make([]backup.SchemaDefinition, 0, len(collectionsToRestore))
		for _, name := range collectionsToRestore {
			if s, ok := schemaMap[name]; ok {
				schemasToValidate = append(schemasToValidate, s)
			}
		}

		// Build target schema lookup for singleton detection
		targetSchemaMap := make(map[string]backup.SchemaDefinition)
		for _, s := range targetSchemas {
			targetSchemaMap[s.Name] = s
		}

		validation := backup.ValidateSchemaCompatibility(schemasToValidate, targetSchemas)
		if !validation.Compatible {
			fmt.Println("failed")
			for _, e := range validation.Errors {
				fmt.Printf("  - %s\n", e)
			}
			return fmt.Errorf("schema validation failed")
		}
		fmt.Println("passed")

		// Clean existing data if requested
		if clean && !dryRun {
			fmt.Print("Cleaning existing data... ")
			deletedCount := 0
			for _, collection := range collectionsToRestore {
				data, err := c.Get("/collections/" + collection + "?limit=10000")
				if err != nil {
					continue
				}
				docs := backup.ParseDocuments(data)
				for _, doc := range docs {
					docID := backup.ExtractDocID(doc)
					if docID != "" {
						if _, err := c.Delete("/collections/" + collection + "/" + docID); err == nil {
							deletedCount++
						}
					}
				}
			}
			fmt.Printf("%d document(s) deleted\n", deletedCount)
		}

		// ID mappings for reference rewriting
		idMappings := make(map[string]string)
		mediaRestored := 0

		// Restore media first (builds old ID -> new ID mappings)
		if len(manifest.MediaIndex) > 0 {
			if dryRun {
				fmt.Printf("  [DRY RUN] Would restore %d media file(s)\n", len(manifest.MediaIndex))
			} else {
				fmt.Printf("  Restoring media... ")

				// Build zip file lookup for media
				mediaZipFiles := make(map[string]*zip.File)
				for _, f := range zr.File {
					if strings.HasPrefix(f.Name, "media/") {
						// Strip "media/" prefix to get filename
						name := f.Name[6:]
						mediaZipFiles[name] = f
					}
				}

				for oldID, mediaInfo := range manifest.MediaIndex {
					zipFile, ok := mediaZipFiles[mediaInfo.Filename]
					if !ok {
						fmt.Fprintf(cmd.ErrOrStderr(), "\n    Warning: media file %s not found in archive\n", mediaInfo.Filename)
						continue
					}

					rc, err := zipFile.Open()
					if err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "\n    Warning: failed to open %s: %v\n", mediaInfo.Filename, err)
						continue
					}

					result, err := c.UploadFile(mediaInfo.Filename, rc)
					rc.Close()
					if err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "\n    Warning: failed to upload %s: %v\n", mediaInfo.Filename, err)
						continue
					}

					// Extract new ID from upload response
					newID := extractMediaID(result)
					if newID != "" {
						idMappings[oldID] = newID
						mediaRestored++
					} else {
						fmt.Fprintf(cmd.ErrOrStderr(), "\n    Warning: uploaded %s but could not extract new ID from response\n", mediaInfo.Filename)
					}
				}

				fmt.Printf("%d/%d file(s)\n", mediaRestored, len(manifest.MediaIndex))
			}
		}

		// Restore documents in order
		totalRestored := 0
		totalRefsUpdated := 0

		for _, collectionName := range restoreOrder {
			schema, hasSchema := schemaMap[collectionName]

			// Find document files for this collection
			prefix := "collections/" + collectionName + "/"
			var docFiles []*zip.File
			for _, f := range zr.File {
				if strings.HasPrefix(f.Name, prefix) && strings.HasSuffix(f.Name, ".json") {
					docFiles = append(docFiles, f)
				}
			}

			if len(docFiles) == 0 {
				fmt.Printf("  %s: no documents\n", collectionName)
				continue
			}

			if dryRun {
				fmt.Printf("  [DRY RUN] Would restore %d document(s) to %s\n", len(docFiles), collectionName)
				continue
			}

			fmt.Printf("  Restoring %s... ", collectionName)
			restored := 0

			for _, f := range docFiles {
				rc, err := f.Open()
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "\n    Warning: failed to open %s: %v\n", f.Name, err)
					continue
				}
				data, err := io.ReadAll(rc)
				rc.Close()
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "\n    Warning: failed to read %s: %v\n", f.Name, err)
					continue
				}

				var doc map[string]interface{}
				if err := json.Unmarshal(data, &doc); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "\n    Warning: invalid JSON in %s: %v\n", f.Name, err)
					continue
				}

				originalID := backup.ExtractDocID(doc)

				// Strip system fields
				cleanDoc := backup.StripSystemFields(doc)

				// Update references
				if hasSchema && len(idMappings) > 0 {
					var refCount int
					cleanDoc, refCount = backup.UpdateReferences(cleanDoc, schema, idMappings)
					totalRefsUpdated += refCount
				}

				// Sanitize
				cleanDoc = backup.SanitizeDocument(cleanDoc)

				// Determine if this is a singleton collection
				targetSchema, isTargetKnown := targetSchemaMap[collectionName]
				isSingleton := isTargetKnown && targetSchema.Singleton

				// Create/update document
				docJSON, err := json.Marshal(map[string]interface{}{"data": cleanDoc})
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "\n    Warning: failed to marshal doc: %v\n", err)
					continue
				}

				var respData []byte
				if isSingleton && originalID != "" {
					// Singleton: use PUT to upsert with original ID
					respData, err = c.Put("/collections/"+collectionName+"/"+originalID, bytes.NewReader(docJSON))
					if err != nil {
						// Fallback: try POST
						respData, err = c.Post("/collections/"+collectionName, bytes.NewReader(docJSON))
					}
				} else {
					// Regular document: POST to create
					respData, err = c.Post("/collections/"+collectionName, bytes.NewReader(docJSON))
				}

				if err != nil {
					if overwrite && originalID != "" {
						if _, putErr := c.Put("/collections/"+collectionName+"/"+originalID, bytes.NewReader(docJSON)); putErr != nil {
							fmt.Fprintf(cmd.ErrOrStderr(), "\n    Warning: failed to create/update doc %s: %v\n", originalID, putErr)
						} else {
							idMappings[originalID] = originalID
							restored++
						}
					}
					continue
				}

				// Extract new ID for mappings
				if originalID != "" {
					if isSingleton {
						// Singletons preserve their ID
						idMappings[originalID] = originalID
					} else {
						var result map[string]interface{}
						if json.Unmarshal(respData, &result) == nil {
							newID := backup.ExtractDocID(result)
							if newID == "" {
								if docResult, ok := result["document"].(map[string]interface{}); ok {
									newID = backup.ExtractDocID(docResult)
								}
							}
							if newID != "" {
								idMappings[originalID] = newID
							}
						}
					}
				}

				restored++
			}

			totalRestored += restored
			fmt.Printf("%d/%d document(s)\n", restored, len(docFiles))
		}

		// Summary
		fmt.Println()
		if dryRun {
			fmt.Println("Dry run completed - no changes made")
		} else {
			fmt.Println("Restore completed")
		}
		fmt.Println()
		fmt.Println("Restore Summary")
		fmt.Println("──────────────────────────────────────────────────")
		fmt.Printf("Documents restored:    %d\n", totalRestored)
		fmt.Printf("Media restored:        %d\n", mediaRestored)
		fmt.Printf("References updated:    %d\n", totalRefsUpdated)
		fmt.Printf("Collections:           %d\n", len(collectionsToRestore))
		if dryRun {
			fmt.Printf("Mode:                  Dry run\n")
		} else {
			fmt.Printf("Mode:                  Live restore\n")
		}
		fmt.Println("──────────────────────────────────────────────────")

		return nil
	},
}

// extractMediaID extracts the new media ID from an upload response.
// Handles various response shapes: {files: [{id}]}, {file: {id}}, {id}, etc.
func extractMediaID(result map[string]interface{}) string {
	// Try {files: [{id: ...}]}
	if files, ok := result["files"].([]interface{}); ok && len(files) > 0 {
		if f, ok := files[0].(map[string]interface{}); ok {
			if id := backup.ExtractDocID(f); id != "" {
				return id
			}
		}
	}
	// Try {file: {id: ...}}
	if file, ok := result["file"].(map[string]interface{}); ok {
		if id := backup.ExtractDocID(file); id != "" {
			return id
		}
	}
	// Try direct {id: ...}
	return backup.ExtractDocID(result)
}

func readManifestFromZip(zr *zip.Reader) (*backup.BackupManifest, error) {
	for _, f := range zr.File {
		if f.Name == "manifest.json" {
			rc, err := f.Open()
			if err != nil {
				return nil, fmt.Errorf("failed to open manifest: %w", err)
			}
			defer rc.Close()

			data, err := io.ReadAll(rc)
			if err != nil {
				return nil, fmt.Errorf("failed to read manifest: %w", err)
			}

			var manifest backup.BackupManifest
			if err := json.Unmarshal(data, &manifest); err != nil {
				return nil, fmt.Errorf("invalid manifest: %w", err)
			}

			return &manifest, nil
		}
	}
	return nil, fmt.Errorf("manifest.json not found in backup archive")
}

func init() {
	restoreCmd.Flags().String("input", "", "backup file path (e.g., backup.zip)")
	restoreCmd.MarkFlagRequired("input")
	restoreCmd.Flags().String("collections", "", "comma-separated list of collections to restore")
	restoreCmd.Flags().Bool("with-dependencies", false, "include dependencies of specified collections")
	restoreCmd.Flags().Bool("clean", false, "delete existing content before restore")
	restoreCmd.Flags().Bool("overwrite", false, "overwrite existing documents on conflict")
	restoreCmd.Flags().Bool("dry-run", false, "preview changes without applying")
	rootCmd.AddCommand(restoreCmd)
}
