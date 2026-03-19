package client

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func jsonResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func apiSuccess(w http.ResponseWriter, data interface{}) {
	raw, _ := json.Marshal(data)
	jsonResponse(w, apiResponse{
		Success: true,
		Data:    json.RawMessage(raw),
	})
}

func apiError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"error":   map[string]string{"message": message},
	})
}

// --- New() ---

func TestNew(t *testing.T) {
	c := New("https://cms.example.com/api", "my-token")

	if c.BaseURL != "https://cms.example.com/api" {
		t.Fatalf("BaseURL = %q, want %q", c.BaseURL, "https://cms.example.com/api")
	}
	if c.Token != "my-token" {
		t.Fatalf("Token = %q, want %q", c.Token, "my-token")
	}
	if c.HTTPClient == nil {
		t.Fatal("HTTPClient should not be nil")
	}
	if c.HTTPClient.Timeout == 0 {
		t.Fatal("HTTPClient should have a timeout")
	}
}

func TestNew_TrimsTrailingSlash(t *testing.T) {
	c := New("https://cms.example.com/api/", "t")
	if c.BaseURL != "https://cms.example.com/api" {
		t.Fatalf("expected trailing slash to be trimmed, got %q", c.BaseURL)
	}
}

// --- SetToken ---

func TestSetToken(t *testing.T) {
	c := New("http://localhost", "old")
	c.SetToken("new")
	if c.Token != "new" {
		t.Fatalf("Token = %q, want 'new'", c.Token)
	}
}

// --- Health() ---

func TestHealth_Success(t *testing.T) {
	var gotPath string
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		jsonResponse(w, HealthResponse{Status: "ok", Version: "1.2.3"})
	})

	// Health endpoint is at root, not under /api
	c := New(server.URL+"/api", "token")
	health, err := c.Health()
	if err != nil {
		t.Fatalf("Health() error: %v", err)
	}
	if gotPath != "/health" {
		t.Fatalf("expected /health, got %s", gotPath)
	}
	if health.Status != "ok" {
		t.Fatalf("Status = %q, want %q", health.Status, "ok")
	}
	if health.Version != "1.2.3" {
		t.Fatalf("Version = %q, want %q", health.Version, "1.2.3")
	}
}

func TestHealth_InvalidJSON(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("not json"))
	})

	c := New(server.URL, "token")
	health, err := c.Health()
	if err != nil {
		t.Fatalf("Health() should not error on invalid JSON, got: %v", err)
	}
	if health.Status != "ok" {
		t.Fatalf("expected fallback status 'ok', got %q", health.Status)
	}
}

func TestHealth_ConnectionError(t *testing.T) {
	c := New("http://127.0.0.1:1", "token")
	_, err := c.Health()
	if err == nil {
		t.Fatal("expected error on connection failure")
	}
}

// --- request() internals ---

func TestRequest_SetsAuthHeader(t *testing.T) {
	var gotAuth string
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte(`{}`))
	})

	c := New(server.URL, "test-token")
	c.request(http.MethodGet, "/test", nil)

	if gotAuth != "Bearer test-token" {
		t.Fatalf("Authorization = %q, want %q", gotAuth, "Bearer test-token")
	}
}

func TestRequest_SetsContentTypeWithBody(t *testing.T) {
	var gotContentType string
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		w.Write([]byte(`{}`))
	})

	c := New(server.URL, "token")
	c.request(http.MethodPost, "/test", strings.NewReader(`{}`))

	if gotContentType != "application/json" {
		t.Fatalf("Content-Type = %q, want %q", gotContentType, "application/json")
	}
}

func TestRequest_NoContentTypeWithoutBody(t *testing.T) {
	var gotContentType string
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		w.Write([]byte(`{}`))
	})

	c := New(server.URL, "token")
	c.request(http.MethodGet, "/test", nil)

	if gotContentType != "" {
		t.Fatalf("Content-Type should be empty for bodyless request, got %q", gotContentType)
	}
}

func TestRequest_UsesBaseURLDirectly(t *testing.T) {
	var gotPath string
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte(`{}`))
	})

	c := New(server.URL, "token")
	c.request("GET", "/collections", nil)

	if gotPath != "/collections" {
		t.Fatalf("expected /collections, got %s", gotPath)
	}
}

func TestRequest_AutoExtractsData(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		apiSuccess(w, []string{"a", "b"})
	})

	c := New(server.URL, "token")
	data, err := c.request("GET", "/test", nil)
	if err != nil {
		t.Fatal(err)
	}

	var result []string
	json.Unmarshal(data, &result)
	if len(result) != 2 || result[0] != "a" {
		t.Fatalf("expected auto-extracted data, got %q", string(data))
	}
}

func TestRequest_PassesThroughNonEnvelope(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"key":"value"}`))
	})

	c := New(server.URL, "token")
	data, _ := c.request("GET", "/test", nil)
	if string(data) != `{"key":"value"}` {
		t.Fatalf("non-envelope response should pass through, got %q", string(data))
	}
}

func TestRequest_HTTP400_APIError(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		apiError(w, 400, "bad request field")
	})

	c := New(server.URL, "token")
	_, err := c.request("GET", "/test", nil)
	if err == nil {
		t.Fatal("expected error on 400")
	}
	if !strings.Contains(err.Error(), "bad request field") {
		t.Fatalf("expected error message in response, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "400") {
		t.Fatalf("expected HTTP status in error, got %q", err.Error())
	}
}

func TestRequest_HTTP500_RawError(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("Internal Server Error"))
	})

	c := New(server.URL, "token")
	_, err := c.request("GET", "/test", nil)
	if err == nil {
		t.Fatal("expected error on 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected 500 in error, got %q", err.Error())
	}
}

func TestRequest_SendsBody(t *testing.T) {
	var gotBody string
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Write([]byte(`{}`))
	})

	c := New(server.URL, "token")
	c.request("POST", "/test", strings.NewReader(`{"key":"value"}`))

	if gotBody != `{"key":"value"}` {
		t.Fatalf("expected body, got %q", gotBody)
	}
}

// --- Get/Post/Put/Delete ---

func TestGet(t *testing.T) {
	var gotMethod string
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.Write([]byte(`{"ok":true}`))
	})

	c := New(server.URL, "token")
	data, err := c.Get("/test")
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != "GET" {
		t.Fatalf("method = %q", gotMethod)
	}
	if !strings.Contains(string(data), "ok") {
		t.Fatalf("data = %q", string(data))
	}
}

func TestPost(t *testing.T) {
	var gotMethod, gotBody string
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Write([]byte(`{}`))
	})

	c := New(server.URL, "token")
	c.Post("/test", strings.NewReader(`{"x":1}`))
	if gotMethod != "POST" {
		t.Fatalf("method = %q", gotMethod)
	}
	if gotBody != `{"x":1}` {
		t.Fatalf("body = %q", gotBody)
	}
}

func TestPut(t *testing.T) {
	var gotMethod string
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.Write([]byte(`{}`))
	})

	c := New(server.URL, "token")
	c.Put("/test", strings.NewReader(`{}`))
	if gotMethod != "PUT" {
		t.Fatalf("method = %q", gotMethod)
	}
}

func TestDelete(t *testing.T) {
	var gotMethod string
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.Write([]byte(`{}`))
	})

	c := New(server.URL, "token")
	c.Delete("/test")
	if gotMethod != "DELETE" {
		t.Fatalf("method = %q", gotMethod)
	}
}

// --- ListCollections() ---

func TestListCollections_Success(t *testing.T) {
	var gotPath string
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		apiSuccess(w, []string{"posts", "pages", "authors"})
	})

	c := New(server.URL, "token")
	collections, err := c.ListCollections()
	if err != nil {
		t.Fatalf("ListCollections() error: %v", err)
	}
	if gotPath != "/collections" {
		t.Fatalf("expected /collections, got %s", gotPath)
	}
	if len(collections) != 3 {
		t.Fatalf("expected 3 collections, got %d", len(collections))
	}
	if collections[0] != "posts" {
		t.Fatalf("expected 'posts', got %q", collections[0])
	}
}

func TestListCollections_Empty(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		apiSuccess(w, []string{})
	})

	c := New(server.URL, "token")
	collections, err := c.ListCollections()
	if err != nil {
		t.Fatalf("ListCollections() error: %v", err)
	}
	if len(collections) != 0 {
		t.Fatalf("expected 0 collections, got %d", len(collections))
	}
}

func TestListCollections_APIError(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		apiError(w, 401, "unauthorized")
	})

	c := New(server.URL, "token")
	_, err := c.ListCollections()
	if err == nil {
		t.Fatal("expected error on 401")
	}
}

// --- CollectionStats() ---

func TestCollectionStats_Success(t *testing.T) {
	var gotPath string
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		apiSuccess(w, CollectionStats{
			TotalDocuments:     42,
			PublishedDocuments: 30,
			DraftDocuments:     12,
		})
	})

	c := New(server.URL, "token")
	stats, err := c.CollectionStats("posts")
	if err != nil {
		t.Fatalf("CollectionStats() error: %v", err)
	}
	if gotPath != "/stats/posts" {
		t.Fatalf("expected /stats/posts, got %s", gotPath)
	}
	if stats.TotalDocuments != 42 {
		t.Fatalf("TotalDocuments = %d, want 42", stats.TotalDocuments)
	}
	if stats.PublishedDocuments != 30 {
		t.Fatalf("PublishedDocuments = %d, want 30", stats.PublishedDocuments)
	}
	if stats.DraftDocuments != 12 {
		t.Fatalf("DraftDocuments = %d, want 12", stats.DraftDocuments)
	}
}

// --- ExportCollection() ---

func TestExportCollection_Success(t *testing.T) {
	var gotPath, gotLimit string
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotLimit = r.URL.Query().Get("limit")
		apiSuccess(w, []map[string]string{{"_id": "1", "title": "Hello"}})
	})

	c := New(server.URL, "token")
	data, err := c.ExportCollection("posts")
	if err != nil {
		t.Fatalf("ExportCollection() error: %v", err)
	}
	if gotPath != "/collections/posts" {
		t.Fatalf("expected /collections/posts, got %s", gotPath)
	}
	if gotLimit != "10000" {
		t.Fatalf("expected limit=10000, got %q", gotLimit)
	}
	if !strings.Contains(string(data), "Hello") {
		t.Fatalf("expected response to contain 'Hello', got %q", string(data))
	}
}

// --- ImportCollection() ---

func TestImportCollection_ArrayFormat(t *testing.T) {
	var received []string
	var gotMethods []string
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethods = append(gotMethods, r.Method)
		body, _ := io.ReadAll(r.Body)
		received = append(received, string(body))
		apiSuccess(w, map[string]string{"id": "new-id"})
	})

	c := New(server.URL, "token")
	input := `[{"title":"Post 1"},{"title":"Post 2"}]`
	count, err := c.ImportCollection("posts", []byte(input))
	if err != nil {
		t.Fatalf("ImportCollection() error: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 imported, got %d", count)
	}
	if len(received) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(received))
	}
	// Verify methods
	for _, m := range gotMethods {
		if m != http.MethodPost {
			t.Fatalf("expected POST, got %s", m)
		}
	}
	// Verify body wraps in {data: ...}
	for _, body := range received {
		if !strings.HasPrefix(body, `{"data":`) {
			t.Fatalf("expected body wrapped in {data:...}, got %q", body)
		}
	}
}

func TestImportCollection_APIResponseFormat(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		apiSuccess(w, map[string]string{"id": "new-id"})
	})

	c := New(server.URL, "token")
	// Input in API response wrapper format
	rawDocs, _ := json.Marshal([]map[string]string{{"title": "Post 1"}})
	input, _ := json.Marshal(apiResponse{
		Success: true,
		Data:    json.RawMessage(rawDocs),
	})
	count, err := c.ImportCollection("posts", input)
	if err != nil {
		t.Fatalf("ImportCollection() error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 imported, got %d", count)
	}
}

func TestImportCollection_InvalidJSON(t *testing.T) {
	c := New("http://localhost", "token")
	_, err := c.ImportCollection("posts", []byte("not json"))
	if err == nil {
		t.Fatal("expected error on invalid JSON")
	}
}

func TestImportCollection_EmptyArray(t *testing.T) {
	requestMade := false
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		requestMade = true
		apiSuccess(w, map[string]string{"id": "x"})
	})

	c := New(server.URL, "token")
	count, err := c.ImportCollection("posts", []byte(`[]`))
	if err != nil {
		t.Fatalf("ImportCollection() error: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 imported, got %d", count)
	}
	if requestMade {
		t.Fatal("should not make any requests for empty array")
	}
}

func TestImportCollection_PartialFailure(t *testing.T) {
	callCount := 0
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 2 {
			apiError(w, 500, "server error")
			return
		}
		apiSuccess(w, map[string]string{"id": "ok"})
	})

	c := New(server.URL, "token")
	input := `[{"title":"A"},{"title":"B"},{"title":"C"}]`
	count, err := c.ImportCollection("posts", []byte(input))
	if err != nil {
		t.Fatalf("ImportCollection() error: %v", err)
	}
	// 2 out of 3 should succeed (second one fails)
	if count != 2 {
		t.Fatalf("expected 2 successful imports, got %d", count)
	}
}

// --- ExportMedia() ---

func TestExportMedia_Success(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/media" {
			apiSuccess(w, []map[string]string{
				{"id": "m1", "filename": "photo.jpg"},
				{"id": "m2", "filename": "doc.pdf"},
			})
			return
		}
		if strings.HasPrefix(r.URL.Path, "/media/") && strings.HasSuffix(r.URL.Path, "/file") {
			w.Write([]byte("file-content"))
			return
		}
		w.WriteHeader(404)
	})

	outDir := filepath.Join(t.TempDir(), "media")
	c := New(server.URL, "token")
	count, err := c.ExportMedia(outDir)
	if err != nil {
		t.Fatalf("ExportMedia() error: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 media files, got %d", count)
	}

	// Check metadata file
	if _, err := os.Stat(filepath.Join(outDir, "_metadata.json")); err != nil {
		t.Fatal("_metadata.json not created")
	}

	// Check media files
	for _, name := range []string{"photo.jpg", "doc.pdf"} {
		data, err := os.ReadFile(filepath.Join(outDir, name))
		if err != nil {
			t.Fatalf("media file %s not created: %v", name, err)
		}
		if string(data) != "file-content" {
			t.Fatalf("media file %s has wrong content", name)
		}
	}
}

func TestExportMedia_EmptyFilename(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/media" {
			apiSuccess(w, []map[string]string{
				{"id": "m1", "filename": ""},
			})
			return
		}
		w.Write([]byte("content"))
	})

	outDir := filepath.Join(t.TempDir(), "media")
	c := New(server.URL, "token")
	count, err := c.ExportMedia(outDir)
	if err != nil {
		t.Fatalf("ExportMedia() error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1, got %d", count)
	}

	// File should be saved with ID as name
	if _, err := os.Stat(filepath.Join(outDir, "m1")); err != nil {
		t.Fatal("expected file saved with ID as filename")
	}
}

func TestExportMedia_ChecksAuthHeader(t *testing.T) {
	var gotMediaAuth string
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/media" {
			apiSuccess(w, []map[string]string{
				{"id": "m1", "filename": "f.jpg"},
			})
			return
		}
		gotMediaAuth = r.Header.Get("Authorization")
		w.Write([]byte("ok"))
	})

	outDir := filepath.Join(t.TempDir(), "media")
	c := New(server.URL, "my-token")
	count, err := c.ExportMedia(outDir)
	if err != nil {
		t.Fatalf("ExportMedia() error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 media file, got %d", count)
	}
	if gotMediaAuth != "Bearer my-token" {
		t.Fatalf("media download auth header = %q, want %q", gotMediaAuth, "Bearer my-token")
	}
}

// --- UploadFile() ---

func TestUploadFile_Success(t *testing.T) {
	var gotContentType, gotAuth string
	var gotBody []byte
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotAuth = r.Header.Get("Authorization")
		gotBody, _ = io.ReadAll(r.Body)

		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"files": []map[string]string{
					{"id": "media-new-123", "filename": "photo.jpg"},
				},
			},
		})
	})

	c := New(server.URL, "my-token")
	result, err := c.UploadFile("photo.jpg", strings.NewReader("fake-image-data"))
	if err != nil {
		t.Fatalf("UploadFile() error: %v", err)
	}
	if gotAuth != "Bearer my-token" {
		t.Fatalf("auth = %q", gotAuth)
	}
	if !strings.Contains(gotContentType, "multipart/form-data") {
		t.Fatalf("Content-Type = %q, want multipart/form-data", gotContentType)
	}
	if !strings.Contains(string(gotBody), "fake-image-data") {
		t.Fatal("body should contain file data")
	}
	if !strings.Contains(string(gotBody), "photo.jpg") {
		t.Fatal("body should contain filename")
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	// Verify envelope was unwrapped — result should be the inner data
	if _, ok := result["files"]; !ok {
		t.Fatalf("expected result to contain 'files', got %v", result)
	}
}

func TestUploadFile_ServerError(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte("internal error"))
	})

	c := New(server.URL, "token")
	_, err := c.UploadFile("test.jpg", strings.NewReader("data"))
	if err == nil {
		t.Fatal("expected error on 500")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected HTTP status in error, got %q", err.Error())
	}
}

// --- GenerateTypes() ---

func TestGenerateTypes_Success(t *testing.T) {
	var gotPath string
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		apiSuccess(w, map[string]interface{}{
			"post": map[string]string{"title": "string"},
		})
	})

	c := New(server.URL, "token")
	types, err := c.GenerateTypes()
	if err != nil {
		t.Fatalf("GenerateTypes() error: %v", err)
	}
	if gotPath != "/schemas" {
		t.Fatalf("expected /schemas, got %s", gotPath)
	}
	if !strings.Contains(types, "Generated by trokky CLI") {
		t.Fatal("expected header comment")
	}
	if !strings.Contains(types, "export type Schemas") {
		t.Fatal("expected TypeScript export")
	}
	if !strings.Contains(types, server.URL) {
		t.Fatal("expected source URL in output")
	}
	if !strings.Contains(types, "post") {
		t.Fatal("expected schema content in output")
	}
}

func TestGenerateTypes_APIError(t *testing.T) {
	server := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		apiError(w, 401, "not authorized")
	})

	c := New(server.URL, "token")
	_, err := c.GenerateTypes()
	if err == nil {
		t.Fatal("expected error on 401")
	}
}
