package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/trokky/cli/internal/backup"
	"github.com/trokky/cli/internal/client"
)

var documentsCmd = &cobra.Command{
	Use:     "documents",
	Aliases: []string{"docs"},
	Short:   "Manage documents in a collection",
	Long:    `List, get, create, update, and delete documents in Trokky collections.`,
}

// --- documents list ---

var docsListCmd = &cobra.Command{
	Use:   "list <collection>",
	Short: "List documents in a collection",
	Long: `List documents with filtering, sorting, and pagination.

Example:
  trokky documents list posts
  trokky documents list posts --limit 5 --status published
  trokky documents list posts --filter '{"featured":true}' --sort _createdAt --order desc
  trokky documents list posts --format ids-only
  trokky documents list posts --count`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := client.FromContext(cmd)
		if err != nil {
			return err
		}

		collection := args[0]
		limit, _ := cmd.Flags().GetInt("limit")
		offset, _ := cmd.Flags().GetInt("offset")
		filter, _ := cmd.Flags().GetString("filter")
		sort, _ := cmd.Flags().GetString("sort")
		order, _ := cmd.Flags().GetString("order")
		status, _ := cmd.Flags().GetString("status")
		expand, _ := cmd.Flags().GetString("expand")
		format, _ := cmd.Flags().GetString("format")
		countOnly, _ := cmd.Flags().GetBool("count")

		// Validate format
		if format != "json" && format != "ids-only" {
			return fmt.Errorf("invalid format %q: must be 'json' or 'ids-only'", format)
		}

		// Build query params
		params := url.Values{}
		if limit > 0 {
			params.Set("limit", fmt.Sprintf("%d", limit))
		}
		if offset > 0 {
			params.Set("offset", fmt.Sprintf("%d", offset))
		}
		if filter != "" {
			params.Set("filter", filter)
		}
		if sort != "" {
			sortConfig := map[string]string{sort: order}
			sortJSON, _ := json.Marshal(sortConfig)
			params.Set("sort", string(sortJSON))
		}
		if status != "" {
			if filter != "" {
				// Merge with existing filter
				var existing map[string]interface{}
				if err := json.Unmarshal([]byte(filter), &existing); err != nil {
					return fmt.Errorf("--filter must be a JSON object when combined with --status")
				}
				existing["_status"] = status
				merged, _ := json.Marshal(existing)
				params.Set("filter", string(merged))
			} else {
				filterJSON, _ := json.Marshal(map[string]string{"_status": status})
				params.Set("filter", string(filterJSON))
			}
		}
		if expand != "" {
			params.Set("expand", expand)
		}
		if countOnly {
			params.Set("count", "true")
		}

		query := ""
		if len(params) > 0 {
			query = "?" + params.Encode()
		}

		data, err := c.Get("/collections/" + collection + query)
		if err != nil {
			return fmt.Errorf("failed to list %s: %w", collection, err)
		}

		// Handle count-only response
		if countOnly {
			var result map[string]interface{}
			if json.Unmarshal(data, &result) == nil {
				if meta, ok := result["meta"].(map[string]interface{}); ok {
					if total, ok := meta["total"].(float64); ok {
						fmt.Printf("%d\n", int(total))
						return nil
					}
				}
				if pagination, ok := result["pagination"].(map[string]interface{}); ok {
					if total, ok := pagination["total"].(float64); ok {
						fmt.Printf("%d\n", int(total))
						return nil
					}
				}
			}
			// Fallback: count the documents
			docs := backup.ParseDocuments(data)
			fmt.Printf("%d\n", len(docs))
			return nil
		}

		docs := backup.ParseDocuments(data)

		switch format {
		case "ids-only":
			for _, doc := range docs {
				id := backup.ExtractDocID(doc)
				if id != "" {
					fmt.Println(id)
				}
			}
		default:
			pretty, _ := json.MarshalIndent(docs, "", "  ")
			fmt.Println(string(pretty))
		}

		return nil
	},
}

// --- documents get ---

var docsGetCmd = &cobra.Command{
	Use:   "get <collection> <id>",
	Short: "Get a single document by ID",
	Long: `Fetch a single document from a collection.

Example:
  trokky documents get posts post-123
  trokky documents get posts post-123 --expand author
  trokky documents get posts post-123 --field title`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := client.FromContext(cmd)
		if err != nil {
			return err
		}

		collection := args[0]
		id := args[1]
		expand, _ := cmd.Flags().GetString("expand")
		field, _ := cmd.Flags().GetString("field")

		params := url.Values{}
		if expand != "" {
			params.Set("expand", expand)
		}

		query := ""
		if len(params) > 0 {
			query = "?" + params.Encode()
		}

		data, err := c.Get("/collections/" + collection + "/" + id + query)
		if err != nil {
			return fmt.Errorf("document not found: %w", err)
		}

		// Extract specific field if requested
		if field != "" {
			var doc map[string]interface{}
			if err := json.Unmarshal(data, &doc); err != nil {
				return fmt.Errorf("failed to parse document: %w", err)
			}
			val := getByDotPath(doc, field)
			if val == nil {
				return fmt.Errorf("field %q not found", field)
			}
			pretty, _ := json.MarshalIndent(val, "", "  ")
			fmt.Println(string(pretty))
			return nil
		}

		var doc interface{}
		json.Unmarshal(data, &doc)
		pretty, _ := json.MarshalIndent(doc, "", "  ")
		fmt.Println(string(pretty))
		return nil
	},
}

// --- documents create ---

var docsCreateCmd = &cobra.Command{
	Use:   "create <collection> [file]",
	Short: "Create a new document",
	Long: `Create a new document in a collection from a file, --data flag, or stdin.

Example:
  trokky documents create posts article.json
  trokky documents create posts --data '{"title":"Hello"}'
  cat data.json | trokky documents create posts`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := client.FromContext(cmd)
		if err != nil {
			return err
		}

		collection := args[0]
		dataFlag, _ := cmd.Flags().GetString("data")

		docData, err := readDocumentInput(args[1:], dataFlag)
		if err != nil {
			return err
		}

		// Wrap in {data: ...}
		var docObj interface{}
		if err := json.Unmarshal(docData, &docObj); err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}

		body, _ := json.Marshal(map[string]interface{}{"data": docObj})
		respData, err := c.Post("/collections/"+collection, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("failed to create document: %w", err)
		}

		var result interface{}
		json.Unmarshal(respData, &result)
		pretty, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(pretty))
		return nil
	},
}

// --- documents update ---

var docsUpdateCmd = &cobra.Command{
	Use:   "update <collection> <id> [file]",
	Short: "Update an existing document",
	Long: `Update a document by ID from a file, --data flag, or stdin.

Example:
  trokky documents update posts post-123 updated.json
  trokky documents update posts post-123 --data '{"title":"New Title"}'`,
	Args: cobra.RangeArgs(2, 3),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := client.FromContext(cmd)
		if err != nil {
			return err
		}

		collection := args[0]
		id := args[1]
		dataFlag, _ := cmd.Flags().GetString("data")

		docData, err := readDocumentInput(args[2:], dataFlag)
		if err != nil {
			return err
		}

		var docObj interface{}
		if err := json.Unmarshal(docData, &docObj); err != nil {
			return fmt.Errorf("invalid JSON: %w", err)
		}

		body, _ := json.Marshal(map[string]interface{}{"data": docObj})
		path := "/collections/" + collection + "/" + id

		respData, err := c.Put(path, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("failed to update document: %w", err)
		}

		var result interface{}
		json.Unmarshal(respData, &result)
		pretty, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(pretty))
		return nil
	},
}

// --- documents delete ---

var docsDeleteCmd = &cobra.Command{
	Use:   "delete <collection> <id> [ids...]",
	Short: "Delete one or more documents",
	Long: `Delete documents by ID. Supports multiple IDs.

Example:
  trokky documents delete posts post-123
  trokky documents delete posts post-123 post-456 post-789
  trokky documents delete posts post-123 --force`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		c, err := client.FromContext(cmd)
		if err != nil {
			return err
		}

		collection := args[0]
		ids := args[1:]
		force, _ := cmd.Flags().GetBool("force")
		quiet, _ := cmd.Flags().GetBool("quiet")

		if !force {
			ok, err := confirmPrompt(fmt.Sprintf("Delete %d document(s) from %s?", len(ids), collection))
			if err != nil {
				return err
			}
			if !ok {
				fmt.Println("Cancelled")
				return nil
			}
		}

		deleted := 0
		for _, id := range ids {
			_, err := c.Delete("/collections/" + collection + "/" + id)
			if err != nil {
				if !quiet {
					fmt.Fprintf(os.Stderr, "Failed to delete %s: %v\n", id, err)
				}
				continue
			}
			deleted++
			if !quiet {
				fmt.Printf("Deleted %s\n", id)
			}
		}

		if !quiet {
			fmt.Printf("\n%d/%d document(s) deleted\n", deleted, len(ids))
		}
		return nil
	},
}

// --- helpers ---

func readDocumentInput(fileArgs []string, dataFlag string) ([]byte, error) {
	// Priority 1: --data flag
	if dataFlag != "" {
		return []byte(dataFlag), nil
	}

	// Priority 2: file argument
	if len(fileArgs) > 0 {
		data, err := os.ReadFile(fileArgs[0])
		if err != nil {
			return nil, fmt.Errorf("failed to read file: %w", err)
		}
		return data, nil
	}

	// Priority 3: stdin
	stat, err := os.Stdin.Stat()
	if err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("failed to read stdin: %w", err)
		}
		return data, nil
	}

	return nil, fmt.Errorf("no input provided. Use a file argument, --data flag, or pipe via stdin")
}

func getByDotPath(obj map[string]interface{}, path string) interface{} {
	parts := strings.Split(path, ".")
	var current interface{} = obj

	for _, part := range parts {
		switch v := current.(type) {
		case map[string]interface{}:
			current = v[part]
		default:
			return nil
		}
	}

	return current
}

func init() {
	// list flags
	docsListCmd.Flags().Int("limit", 20, "maximum number of documents to return")
	docsListCmd.Flags().Int("offset", 0, "number of documents to skip")
	docsListCmd.Flags().String("filter", "", "JSON filter conditions")
	docsListCmd.Flags().String("sort", "", "field to sort by")
	docsListCmd.Flags().String("order", "asc", "sort order (asc or desc)")
	docsListCmd.Flags().String("status", "", "filter by status (published or draft)")
	docsListCmd.Flags().String("expand", "", "expand reference fields")
	docsListCmd.Flags().String("format", "json", "output format (json, ids-only)")
	docsListCmd.Flags().Bool("count", false, "show document count only")

	// get flags
	docsGetCmd.Flags().String("expand", "", "expand reference fields")
	docsGetCmd.Flags().String("field", "", "extract a specific field by dot-path")

	// create flags
	docsCreateCmd.Flags().String("data", "", "inline JSON data")

	// update flags
	docsUpdateCmd.Flags().String("data", "", "inline JSON data")

	// delete flags
	docsDeleteCmd.Flags().Bool("force", false, "skip confirmation prompt")
	docsDeleteCmd.Flags().Bool("quiet", false, "suppress output")

	// Register subcommands
	documentsCmd.AddCommand(docsListCmd)
	documentsCmd.AddCommand(docsGetCmd)
	documentsCmd.AddCommand(docsCreateCmd)
	documentsCmd.AddCommand(docsUpdateCmd)
	documentsCmd.AddCommand(docsDeleteCmd)

	rootCmd.AddCommand(documentsCmd)
}
