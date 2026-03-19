package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/trokky/cli/internal/auth"
	"github.com/trokky/cli/internal/config"
)

type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

type HealthResponse struct {
	Status  string `json:"status"`
	Version string `json:"version"`
}

type CollectionStats struct {
	TotalDocuments     int `json:"totalDocuments"`
	PublishedDocuments int `json:"publishedDocuments"`
	DraftDocuments     int `json:"draftDocuments"`
}

type apiResponse struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// FromContext creates a client from cobra command flags or stored config.
// It resolves credentials, auto-refreshes OAuth2 tokens, and displays instance info.
func FromContext(cmd *cobra.Command) (*Client, error) {
	url, _ := cmd.Flags().GetString("url")
	token, _ := cmd.Flags().GetString("token")
	instance, _ := cmd.Flags().GetString("instance")

	creds, err := config.RequireCredentials(config.ResolveOptions{
		URL:      url,
		Token:    token,
		Instance: instance,
	})
	if err != nil {
		return nil, err
	}

	// Auto-refresh OAuth2 tokens for config-based credentials
	resolvedToken := creds.Token
	if creds.Source == "config" && creds.Instance != nil {
		tok, refreshed, err := auth.GetValidToken(creds.InstanceName, *creds.Instance)
		if err != nil {
			return nil, err
		}
		resolvedToken = tok

		quiet, _ := cmd.Flags().GetBool("quiet")
		if !quiet {
			if refreshed {
				fmt.Fprintf(os.Stderr, "Using instance: %s (%s) (token refreshed)\n", creds.InstanceName, creds.URL)
			} else {
				fmt.Fprintf(os.Stderr, "Using instance: %s (%s)\n", creds.InstanceName, creds.URL)
			}
		}
	}

	return New(creds.URL, resolvedToken), nil
}

func New(baseURL, token string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Token:      token,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// SetToken updates the client's API token.
func (c *Client) SetToken(token string) {
	c.Token = token
}

func (c *Client) request(method, path string, body io.Reader) ([]byte, error) {
	reqURL := c.BaseURL + path

	req, err := http.NewRequest(method, reqURL, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.Token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		var apiErr apiResponse
		if json.Unmarshal(data, &apiErr) == nil && apiErr.Error != nil {
			return nil, fmt.Errorf("%s (HTTP %d)", apiErr.Error.Message, resp.StatusCode)
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}

	// Auto-extract .data from {success: true, data: ...} envelope
	var envelope apiResponse
	if json.Unmarshal(data, &envelope) == nil && envelope.Success && envelope.Data != nil {
		return []byte(envelope.Data), nil
	}

	return data, nil
}

// Get performs a GET request to the given path.
func (c *Client) Get(path string) ([]byte, error) {
	return c.request(http.MethodGet, path, nil)
}

// Post performs a POST request to the given path with the provided body.
func (c *Client) Post(path string, body io.Reader) ([]byte, error) {
	return c.request(http.MethodPost, path, body)
}

// Put performs a PUT request to the given path with the provided body.
func (c *Client) Put(path string, body io.Reader) ([]byte, error) {
	return c.request(http.MethodPut, path, body)
}

// Delete performs a DELETE request to the given path.
func (c *Client) Delete(path string) ([]byte, error) {
	return c.request(http.MethodDelete, path, nil)
}

func (c *Client) Health() (*HealthResponse, error) {
	// Health endpoint is at the root, not under /api
	healthURL := c.BaseURL
	if strings.HasSuffix(healthURL, "/api") {
		healthURL = healthURL[:len(healthURL)-4]
	}
	healthURL += "/health"

	resp, err := c.HTTPClient.Get(healthURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read health response: %w", err)
	}

	var health HealthResponse
	if err := json.Unmarshal(data, &health); err != nil {
		return &HealthResponse{Status: "ok"}, nil
	}
	return &health, nil
}

func (c *Client) ListCollections() ([]string, error) {
	data, err := c.Get("/collections")
	if err != nil {
		return nil, err
	}

	var collections []string
	if err := json.Unmarshal(data, &collections); err != nil {
		return nil, err
	}

	return collections, nil
}

func (c *Client) CollectionStats(collection string) (*CollectionStats, error) {
	data, err := c.Get("/stats/" + collection)
	if err != nil {
		return nil, err
	}

	var stats CollectionStats
	if err := json.Unmarshal(data, &stats); err != nil {
		return nil, err
	}

	return &stats, nil
}

func (c *Client) ExportCollection(collection string) ([]byte, error) {
	return c.Get("/collections/" + collection + "?limit=10000")
}

func (c *Client) ImportCollection(collection string, data []byte) (int, error) {
	var docs []json.RawMessage

	// Try parsing as array directly
	if err := json.Unmarshal(data, &docs); err != nil {
		// Try parsing as API response wrapper
		var resp apiResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return 0, fmt.Errorf("invalid JSON format")
		}
		if err := json.Unmarshal(resp.Data, &docs); err != nil {
			return 0, fmt.Errorf("invalid document format")
		}
	}

	count := 0
	for _, doc := range docs {
		body := fmt.Sprintf(`{"data":%s}`, string(doc))
		_, err := c.Post("/collections/"+collection, strings.NewReader(body))
		if err != nil {
			continue
		}
		count++
	}

	return count, nil
}

func (c *Client) ExportMedia(outputDir string) (int, error) {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return 0, err
	}

	data, err := c.Get("/media?limit=10000")
	if err != nil {
		return 0, err
	}

	var mediaItems []struct {
		ID       string `json:"id"`
		Filename string `json:"filename"`
	}
	if err := json.Unmarshal(data, &mediaItems); err != nil {
		return 0, err
	}

	// Save metadata
	metaFile := filepath.Join(outputDir, "_metadata.json")
	if err := os.WriteFile(metaFile, data, 0644); err != nil {
		return 0, err
	}

	count := 0
	for _, item := range mediaItems {
		mediaURL := c.BaseURL + "/media/" + item.ID + "/file"
		req, err := http.NewRequest(http.MethodGet, mediaURL, nil)
		if err != nil {
			continue
		}
		req.Header.Set("Authorization", "Bearer "+c.Token)

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			continue
		}

		filename := filepath.Base(item.Filename)
		if filename == "" || filename == "." {
			filename = item.ID
		}
		outFile := filepath.Join(outputDir, filename)

		f, err := os.Create(outFile)
		if err != nil {
			resp.Body.Close()
			continue
		}
		_, err = io.Copy(f, resp.Body)
		resp.Body.Close()
		f.Close()
		if err != nil {
			continue
		}
		count++
	}

	return count, nil
}

// UploadFile uploads a file to the media endpoint via multipart form.
// Returns the parsed response body.
func (c *Client) UploadFile(filename string, data io.Reader) (map[string]interface{}, error) {
	// Build multipart body
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	part, err := writer.CreateFormFile("files", filename)
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}

	if _, err := io.Copy(part, data); err != nil {
		return nil, fmt.Errorf("failed to write file data: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Build request
	uploadURL := c.BaseURL + "/media/upload"
	req, err := http.NewRequest(http.MethodPost, uploadURL, &buf)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("upload failed: %w", err)
	}
	defer resp.Body.Close()

	respData, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read upload response: %w", err)
	}

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("upload failed: HTTP %d: %s", resp.StatusCode, string(respData))
	}

	// Parse response — handle {success: true, data: ...} envelope
	var envelope struct {
		Success bool                   `json:"success"`
		Data    map[string]interface{} `json:"data"`
	}
	if json.Unmarshal(respData, &envelope) == nil && envelope.Success && envelope.Data != nil {
		return envelope.Data, nil
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respData, &result); err != nil {
		return nil, fmt.Errorf("invalid upload response: %w", err)
	}
	return result, nil
}

func (c *Client) GenerateTypes() (string, error) {
	data, err := c.Get("/schemas")
	if err != nil {
		return "", err
	}

	// For now, return raw schemas — type generation will be enhanced
	return fmt.Sprintf("// Generated by trokky CLI\n// Source: %s\n\nexport type Schemas = %s\n", c.BaseURL, string(data)), nil
}
