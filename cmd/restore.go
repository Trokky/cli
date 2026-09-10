package cmd

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/trokky/cli/internal/backup"
	"github.com/trokky/cli/internal/client"
)

// restoredDoc records a document written during phase 1 so that phase 3 can
// rewrite its media references once the new media IDs are known.
type restoredDoc struct {
	collection string
	id         string
	body       map[string]interface{}
	schema     backup.SchemaDefinition
	hasSchema  bool
}

var restoreCmd = &cobra.Command{
	Use:   "restore",
	Short: "Restore content from a Trokky backup file",
	Long: `Restore content and media from a zip backup created by 'trokky backup'.

Documents are restored first, media is uploaded second, and references are
rewritten last. If the token cannot write documents the restore aborts before
anything is uploaded, so a half-restored instance is never left behind.

Example:
  trokky restore --input backup.zip
  trokky restore --input backup.zip --collections posts,pages
  trokky restore --input backup.zip --dry-run`,
	// A failed restore must not bury its error under a usage dump.
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		errOut := cmd.ErrOrStderr()

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

		// Ensure schema fields are parsed (handles map-based field format)
		for i := range manifest.Schemas {
			manifest.Schemas[i].UnmarshalFields()
		}

		fmt.Fprintf(out, "Backup from %s\n", manifest.Timestamp)
		if manifest.Source.URL != "" {
			fmt.Fprintf(out, "Source: %s\n", manifest.Source.URL)
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
			fmt.Fprintf(out, "Selected %d collection(s) for restore\n", len(collectionsToRestore))
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

		fmt.Fprintf(out, "Restore order: %v\n", restoreOrder)

		if dryRun {
			fmt.Fprintln(out, "\n[DRY RUN] No changes will be made")
		}

		// Pre-flight schema validation
		fmt.Fprint(out, "Validating target instance... ")
		targetData, err := c.Get("/collections")
		if err != nil {
			fmt.Fprintln(out, "failed")
			return fmt.Errorf("failed to fetch target schemas: %w", err)
		}

		targetSchemas, err := backup.ParseSchemas(targetData)
		if err != nil {
			fmt.Fprintln(out, "failed")
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
			fmt.Fprintln(out, "failed")
			for _, e := range validation.Errors {
				fmt.Fprintf(out, "  - %s\n", e)
			}
			return fmt.Errorf("schema validation failed")
		}
		fmt.Fprintln(out, "passed")

		// Clean existing data if requested
		if clean && !dryRun {
			// Clean documents
			fmt.Fprint(out, "Cleaning existing documents... ")
			deletedDocs := 0
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
							deletedDocs++
						}
					}
				}
			}
			fmt.Fprintf(out, "%d document(s) deleted\n", deletedDocs)

			// Clean media (paginated with rate-limit handling)
			fmt.Fprint(out, "Cleaning existing media... ")
			deletedMedia := 0
			for {
				mediaData, err := c.Get("/media?limit=100")
				if err != nil {
					break
				}
				var mediaItems []struct {
					ID string `json:"id"`
				}
				if json.Unmarshal(mediaData, &mediaItems) != nil || len(mediaItems) == 0 {
					break
				}
				for _, item := range mediaItems {
					for attempt := 1; attempt <= 3; attempt++ {
						_, err := c.Delete("/media/" + item.ID)
						if err == nil {
							deletedMedia++
							break
						}
						if attempt < 3 {
							time.Sleep(time.Duration(attempt) * time.Second)
						}
					}
					time.Sleep(100 * time.Millisecond)
				}
				fmt.Fprintf(out, "\r  Cleaning existing media... %d deleted", deletedMedia)
			}
			fmt.Fprintf(out, "\r  Cleaning existing media... %d file(s) deleted\n", deletedMedia)
		}

		if dryRun && len(manifest.MediaIndex) > 0 {
			fmt.Fprintf(out, "  [DRY RUN] Would restore %d media file(s)\n", len(manifest.MediaIndex))
		}

		// Document ID mappings (old ID -> new ID), used to rewrite document-to-document
		// references as later collections are restored. Media mappings are deliberately
		// kept separate: they only exist after phase 2.
		idMappings := make(map[string]string)
		mediaIDMappings := make(map[string]string)

		totalDocuments := 0
		totalRestored := 0
		totalFailed := 0
		totalRefsUpdated := 0
		collectionsRestored := 0
		mediaRestored := 0
		staleRefDocs := 0
		var restoredDocs []restoredDoc

		// ── Phase 1: restore documents ────────────────────────────────────────
		// Documents go first because media upload is the destructive half: every
		// upload mints a brand-new media ID. If the token cannot write documents
		// we must find out before a single byte is uploaded.
		for _, collectionName := range restoreOrder {
			schema, hasSchema := schemaMap[collectionName]

			// Check if this is a singleton collection (check target, fallback to backup)
			targetSchema, isTargetKnown := targetSchemaMap[collectionName]
			isSingleton := (isTargetKnown && targetSchema.Singleton) || (hasSchema && schema.Singleton)

			// Find document files for this collection
			prefix := "collections/" + collectionName + "/"
			var docFiles []*zip.File
			for _, f := range zr.File {
				if strings.HasPrefix(f.Name, prefix) && strings.HasSuffix(f.Name, ".json") {
					docFiles = append(docFiles, f)
				}
			}

			if len(docFiles) == 0 {
				fmt.Fprintf(out, "  %s: no documents\n", collectionName)
				continue
			}

			if dryRun {
				fmt.Fprintf(out, "  [DRY RUN] Would restore %d document(s) to %s\n", len(docFiles), collectionName)
				continue
			}

			totalDocuments += len(docFiles)
			fmt.Fprintf(out, "  Restoring %s... ", collectionName)
			restored := 0
			failed := 0

			for _, f := range docFiles {
				rc, err := f.Open()
				if err != nil {
					fmt.Fprintf(errOut, "\n    Warning: failed to open %s: %v\n", f.Name, err)
					failed++
					continue
				}
				data, err := io.ReadAll(rc)
				rc.Close()
				if err != nil {
					fmt.Fprintf(errOut, "\n    Warning: failed to read %s: %v\n", f.Name, err)
					failed++
					continue
				}

				var doc map[string]interface{}
				if err := json.Unmarshal(data, &doc); err != nil {
					fmt.Fprintf(errOut, "\n    Warning: invalid JSON in %s: %v\n", f.Name, err)
					failed++
					continue
				}

				originalID := backup.ExtractDocID(doc)

				// Strip system fields
				cleanDoc := backup.StripSystemFields(doc)

				// Rewrite document-to-document references using the mappings built
				// so far. Media references are left exactly as the backup has them;
				// phase 3 fixes those once the new media IDs exist.
				docRefs := 0
				if len(idMappings) > 0 {
					if hasSchema && len(schema.Fields) > 0 {
						var refCount int
						cleanDoc, refCount = backup.UpdateReferences(cleanDoc, schema, idMappings)
						docRefs += refCount
					}
					docRefs += backup.DeepUpdateMediaRefs(cleanDoc, idMappings)
					docRefs += backup.DeepReplaceMediaIDsInStrings(cleanDoc, idMappings)
				}

				// Sanitize
				cleanDoc = backup.SanitizeDocument(cleanDoc)

				// Create/update document
				docJSON, err := json.Marshal(map[string]interface{}{"data": cleanDoc})
				if err != nil {
					fmt.Fprintf(errOut, "\n    Warning: failed to marshal doc: %v\n", err)
					failed++
					continue
				}
				body := func() io.Reader { return bytes.NewReader(docJSON) }

				var respData []byte
				var writeErr error
				var firstErr error

				if isSingleton && originalID != "" {
					// Singleton: use PUT to upsert with original ID
					respData, writeErr = c.Put("/collections/"+collectionName+"/"+originalID, body())
					if writeErr != nil {
						// PUT failed — log and fallback to POST
						firstErr = writeErr
						fmt.Fprintf(errOut, "\n    Note: PUT failed for singleton %s/%s (%v), trying POST\n", collectionName, originalID, writeErr)
						respData, writeErr = c.Post("/collections/"+collectionName, body())
					}
				} else {
					// Regular document: POST to create
					respData, writeErr = c.Post("/collections/"+collectionName, body())
					if writeErr != nil && overwrite && originalID != "" {
						firstErr = writeErr
						respData, writeErr = c.Put("/collections/"+collectionName+"/"+originalID, body())
					}
				}

				if writeErr != nil {
					// A permission denial must never be masked by the PUT/POST
					// fallbacks: abort now, before anything is uploaded, so the
					// target instance cannot be left half-restored.
					if isPermissionDenied(writeErr) || isPermissionDenied(firstErr) {
						cause := firstErr
						if cause == nil {
							cause = writeErr
						}
						fmt.Fprintln(out)
						return fmt.Errorf("token lacks content:write (required to restore documents): %w", cause)
					}
					fmt.Fprintf(errOut, "\n    Warning: failed to create/update doc %s: %v\n", originalID, writeErr)
					failed++
					continue
				}

				// Extract the target ID so phase 3 knows where to PUT the fixed document.
				newID := originalID
				if isSingleton {
					if originalID != "" {
						idMappings[originalID] = originalID
					}
				} else {
					var result map[string]interface{}
					if len(respData) > 0 && json.Unmarshal(respData, &result) == nil {
						parsed := backup.ExtractDocID(result)
						if parsed == "" {
							if docResult, ok := result["document"].(map[string]interface{}); ok {
								parsed = backup.ExtractDocID(docResult)
							}
						}
						if parsed != "" {
							newID = parsed
						}
					}
					if originalID != "" && newID != "" {
						idMappings[originalID] = newID
					}
				}

				restored++
				totalRefsUpdated += docRefs
				restoredDocs = append(restoredDocs, restoredDoc{
					collection: collectionName,
					id:         newID,
					body:       cleanDoc,
					schema:     schema,
					hasSchema:  hasSchema,
				})
			}

			totalRestored += restored
			totalFailed += failed
			if restored > 0 {
				collectionsRestored++
			}
			fmt.Fprintf(out, "%d/%d document(s)\n", restored, len(docFiles))
			if failed > 0 {
				fmt.Fprintf(errOut, "    %d document(s) failed\n", failed)
			}
		}

		// ── Phase 2: upload media ─────────────────────────────────────────────
		// Only ever reached when phase 1 actually wrote something, so uploads
		// cannot orphan themselves against a document set that was never written.
		if !dryRun && len(manifest.MediaIndex) > 0 {
			if totalRestored == 0 {
				fmt.Fprintf(errOut, "  Skipping media upload: no documents were restored\n")
			} else {
				fmt.Fprint(out, "  Restoring media... ")

				// Build zip file lookup for media
				mediaZipFiles := make(map[string]*zip.File)
				for _, f := range zr.File {
					if strings.HasPrefix(f.Name, "media/") {
						// Strip "media/" prefix to get filename
						name := f.Name[6:]
						mediaZipFiles[name] = f
					}
				}

				mediaTotal := len(manifest.MediaIndex)
				mediaFailed := 0

				for oldID, mediaInfo := range manifest.MediaIndex {
					zipFile, ok := mediaZipFiles[mediaInfo.Filename]
					if !ok {
						fmt.Fprintf(errOut, "\n    Warning: media file %s not found in archive\n", mediaInfo.Filename)
						mediaFailed++
						continue
					}

					// Retry up to 3 times with backoff
					var result map[string]interface{}
					var uploadErr error
					for attempt := 1; attempt <= 3; attempt++ {
						rc, err := zipFile.Open()
						if err != nil {
							uploadErr = err
							break
						}
						result, uploadErr = c.UploadFile(mediaInfo.Filename, rc)
						rc.Close()
						if uploadErr == nil {
							break
						}
						if attempt < 3 {
							delay := time.Duration(attempt) * 2 * time.Second
							time.Sleep(delay)
						}
					}

					if uploadErr != nil {
						fmt.Fprintf(errOut, "\n    Warning: failed to upload %s (after 3 attempts): %v\n", mediaInfo.Filename, uploadErr)
						mediaFailed++
						continue
					}

					// Extract new ID from upload response
					newID := extractMediaID(result)
					if newID != "" {
						mediaIDMappings[oldID] = newID
						mediaRestored++
					} else {
						fmt.Fprintf(errOut, "\n    Warning: uploaded %s but could not extract new ID\n", mediaInfo.Filename)
						mediaFailed++
					}

					// Progress indicator
					fmt.Fprintf(out, "\r  Restoring media... %d/%d (%d failed)", mediaRestored, mediaTotal, mediaFailed)

					// Small delay between uploads to avoid overwhelming the server
					time.Sleep(200 * time.Millisecond)
				}

				fmt.Fprintf(out, "\r  Restoring media... %d/%d file(s)              \n", mediaRestored, mediaTotal)
				if mediaFailed > 0 {
					fmt.Fprintf(errOut, "    %d file(s) failed\n", mediaFailed)
				}
			}
		}

		// ── Phase 3: rewrite media references ─────────────────────────────────
		// Documents without media references are never rewritten.
		if len(mediaIDMappings) > 0 && len(restoredDocs) > 0 {
			fmt.Fprint(out, "  Updating media references... ")
			updatedDocs := 0

			for _, rd := range restoredDocs {
				fixed, refCount, err := applyMediaReferences(rd, mediaIDMappings)
				if err != nil {
					fmt.Fprintf(errOut, "\n    Warning: failed to rewrite references in %s: %v\n", rd.collection, err)
					staleRefDocs++
					continue
				}
				if refCount == 0 {
					continue
				}
				if rd.id == "" {
					fmt.Fprintf(errOut, "\n    Warning: restored document in %s has no known ID, cannot update its media references\n", rd.collection)
					staleRefDocs++
					continue
				}

				payload, err := json.Marshal(map[string]interface{}{"data": fixed})
				if err != nil {
					fmt.Fprintf(errOut, "\n    Warning: failed to marshal %s/%s: %v\n", rd.collection, rd.id, err)
					staleRefDocs++
					continue
				}

				if _, err := c.Put("/collections/"+rd.collection+"/"+rd.id, bytes.NewReader(payload)); err != nil {
					fmt.Fprintf(errOut, "\n    Warning: failed to update media references in %s/%s: %v\n", rd.collection, rd.id, err)
					staleRefDocs++
					continue
				}

				totalRefsUpdated += refCount
				updatedDocs++
			}

			fmt.Fprintf(out, "%d document(s) updated\n", updatedDocs)

		}

		// Any document that was not written, or was written but could not be
		// pointed at the new media IDs, leaves uploaded media orphaned.
		if mediaRestored > 0 && staleRefDocs+totalFailed > 0 {
			fmt.Fprintf(errOut, "\n!! WARNING: ORPHANED MEDIA !!\n")
			fmt.Fprintf(errOut, "%d media file(s) were uploaded under new IDs, but %d document(s) could not be updated to reference them.\n", mediaRestored, staleRefDocs+totalFailed)
			fmt.Fprintf(errOut, "That media is orphaned and those documents' images will 404.\n")
			fmt.Fprintf(errOut, "No rollback was attempted: deleting the media would break the documents that were updated.\n")
			fmt.Fprintf(errOut, "Re-run the restore with a token that can write documents, or repair the affected documents by hand.\n")
		}

		// Summary
		fmt.Fprintln(out)
		switch {
		case dryRun:
			fmt.Fprintln(out, "Dry run completed - no changes made")
		case totalRestored == 0 && totalDocuments > 0:
			fmt.Fprintln(out, "Restore FAILED - no documents were restored")
		case totalFailed > 0:
			fmt.Fprintf(out, "Restore incomplete - %d document(s) restored, %d failed\n", totalRestored, totalFailed)
		case staleRefDocs > 0:
			fmt.Fprintf(out, "Restore incomplete - %d document(s) still reference the old media IDs\n", staleRefDocs)
		default:
			fmt.Fprintln(out, "Restore completed")
		}
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Restore Summary")
		fmt.Fprintln(out, "──────────────────────────────────────────────────")
		fmt.Fprintf(out, "Documents restored:    %d\n", totalRestored)
		if totalFailed > 0 {
			fmt.Fprintf(out, "Documents failed:      %d\n", totalFailed)
		}
		fmt.Fprintf(out, "Media restored:        %d\n", mediaRestored)
		fmt.Fprintf(out, "References updated:    %d\n", totalRefsUpdated)
		if dryRun {
			fmt.Fprintf(out, "Collections selected:  %d\n", len(collectionsToRestore))
			fmt.Fprintf(out, "Mode:                  Dry run\n")
		} else {
			fmt.Fprintf(out, "Collections restored:  %d\n", collectionsRestored)
			fmt.Fprintf(out, "Mode:                  Live restore\n")
		}
		fmt.Fprintln(out, "──────────────────────────────────────────────────")

		if !dryRun {
			if totalRestored == 0 && totalDocuments > 0 {
				return fmt.Errorf("restore failed: none of the %d document(s) in the backup could be restored", totalDocuments)
			}
			if totalFailed > 0 {
				return fmt.Errorf("restore incomplete: %d document(s) failed to restore", totalFailed)
			}
			if staleRefDocs > 0 {
				return fmt.Errorf("restore incomplete: %d document(s) could not be updated to reference the uploaded media", staleRefDocs)
			}
		}

		return nil
	},
}

// isPermissionDenied reports whether err came back as an HTTP 401/403.
// internal/client formats API errors as "<message> (HTTP 403)" when the body
// carries an error envelope and "HTTP 403: <body>" when it does not.
func isPermissionDenied(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, marker := range []string{" (HTTP 401)", " (HTTP 403)", "HTTP 401", "HTTP 403"} {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// applyMediaReferences applies the media ID mappings to a copy of a restored
// document. It returns the rewritten copy and the number of references changed,
// or a zero count when the document has no media references at all (in which
// case it must not be written back).
func applyMediaReferences(rd restoredDoc, mediaIDMappings map[string]string) (map[string]interface{}, int, error) {
	original, err := json.Marshal(rd.body)
	if err != nil {
		return nil, 0, err
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(original, &doc); err != nil {
		return nil, 0, err
	}

	count := 0
	if rd.hasSchema && len(rd.schema.Fields) > 0 {
		var refCount int
		doc, refCount = backup.UpdateReferences(doc, rd.schema, mediaIDMappings)
		count += refCount
	}
	count += backup.DeepUpdateMediaRefs(doc, mediaIDMappings)
	count += backup.DeepReplaceMediaIDsInStrings(doc, mediaIDMappings)
	if count == 0 {
		return nil, 0, nil
	}

	rewritten, err := json.Marshal(doc)
	if err != nil {
		return nil, 0, err
	}
	if bytes.Equal(original, rewritten) {
		return nil, 0, nil
	}
	return doc, count, nil
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
