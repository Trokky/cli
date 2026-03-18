package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
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
func FromContext(cmd *cobra.Command) (*Client, error) {
	instance, _ := cmd.Flags().GetString("instance")
	token, _ := cmd.Flags().GetString("token")

	if instance != "" && token != "" {
		return New(instance, token), nil
	}

	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("not logged in (run `trokky login` first)")
	}

	cfgURL, cfgToken, err := cfg.GetActiveToken()
	if err != nil {
		return nil, err
	}

	if instance == "" {
		instance = cfgURL
	}
	if token == "" {
		token = cfgToken
	}

	return New(instance, token), nil
}

func New(baseURL, token string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Token:      token,
		HTTPClient: &http.Client{},
	}
}

func (c *Client) request(method, path string, body io.Reader) ([]byte, error) {
	url := c.BaseURL + "/api" + path

	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Content-Type", "application/json")

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

	return data, nil
}

func (c *Client) Health() (*HealthResponse, error) {
	url := c.BaseURL + "/health"
	resp, err := c.HTTPClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	var health HealthResponse
	if err := json.Unmarshal(data, &health); err != nil {
		return &HealthResponse{Status: "ok"}, nil
	}
	return &health, nil
}

func (c *Client) ListCollections() ([]string, error) {
	data, err := c.request("GET", "/collections", nil)
	if err != nil {
		return nil, err
	}

	var resp apiResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	var collections []string
	if err := json.Unmarshal(resp.Data, &collections); err != nil {
		return nil, err
	}

	return collections, nil
}

func (c *Client) CollectionStats(collection string) (*CollectionStats, error) {
	data, err := c.request("GET", "/stats/"+collection, nil)
	if err != nil {
		return nil, err
	}

	var resp apiResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	var stats CollectionStats
	if err := json.Unmarshal(resp.Data, &stats); err != nil {
		return nil, err
	}

	return &stats, nil
}

func (c *Client) ExportCollection(collection string) ([]byte, error) {
	data, err := c.request("GET", "/collections/"+collection+"?limit=10000", nil)
	if err != nil {
		return nil, err
	}
	return data, nil
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
		_, err := c.request("POST", "/collections/"+collection, strings.NewReader(body))
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

	data, err := c.request("GET", "/media?limit=10000", nil)
	if err != nil {
		return 0, err
	}

	var resp apiResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return 0, err
	}

	var mediaItems []struct {
		ID       string `json:"id"`
		Filename string `json:"filename"`
	}
	if err := json.Unmarshal(resp.Data, &mediaItems); err != nil {
		return 0, err
	}

	// Save metadata
	metaFile := filepath.Join(outputDir, "_metadata.json")
	if err := os.WriteFile(metaFile, data, 0644); err != nil {
		return 0, err
	}

	count := 0
	for _, item := range mediaItems {
		url := c.BaseURL + "/api/media/" + item.ID + "/file"
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("Authorization", "Bearer "+c.Token)

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			continue
		}

		fileData, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		filename := item.Filename
		if filename == "" {
			filename = item.ID
		}
		outFile := filepath.Join(outputDir, filename)
		if err := os.WriteFile(outFile, fileData, 0644); err != nil {
			continue
		}
		count++
	}

	return count, nil
}

func (c *Client) ImportMedia(mediaDir string) (int, error) {
	// TODO: implement media upload from directory
	return 0, fmt.Errorf("media import not yet implemented")
}

func (c *Client) GenerateTypes() (string, error) {
	data, err := c.request("GET", "/schemas", nil)
	if err != nil {
		return "", err
	}

	var resp apiResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", err
	}

	// For now, return raw schemas — type generation will be enhanced
	return fmt.Sprintf("// Generated by trokky CLI\n// Source: %s\n\nexport type Schemas = %s\n", c.BaseURL, string(resp.Data)), nil
}
