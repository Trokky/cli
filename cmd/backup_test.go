package cmd

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/trokky/cli/internal/backup"
)

func runBackup(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TROKKY_URL", "")
	t.Setenv("TROKKY_TOKEN", "")
	t.Setenv("TROKKY_INSTANCE", "")
	for name, def := range map[string]string{"collections": "", "skip-media": "false", "description": "", "output": ""} {
		if err := backupCmd.Flags().Set(name, def); err != nil {
			t.Fatal(err)
		}
	}
	var stdout, stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)
	defer func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
	}()
	rootCmd.SetArgs(append([]string{"backup"}, args...))
	var err error
	rawOut, rawErr := captureProcessOutput(t, func() {
		err = rootCmd.Execute()
	})
	return stdout.String() + rawOut, stderr.String() + rawErr, err
}

// TestBackupKeepsSameNamedMediaApart covers the archive layout: two media items
// whose original filename is identical must land in distinct zip entries with
// their own bytes, and the manifest must say where each one is.
func TestBackupKeepsSameNamedMediaApart(t *testing.T) {
	files := map[string]string{"media-1": "bytesA", "media-2": "bytesB"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := apiPath(r)
		switch {
		case p == "/collections":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, testCollectionsResponse)
		case strings.HasPrefix(p, "/collections/posts"):
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"success":true,"data":{"documents":[{"id":"doc-1","title":"One"}]}}`)
		case p == "/media":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"success":true,"data":[{"id":"media-1","filename":"photo.png","mimeType":"image/png","size":6},{"id":"media-2","filename":"photo.png","mimeType":"image/png","size":6}]}`)
		case strings.HasPrefix(p, "/media/") && strings.HasSuffix(p, "/file"):
			id := strings.TrimSuffix(strings.TrimPrefix(p, "/media/"), "/file")
			io.WriteString(w, files[id])
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	out := filepath.Join(t.TempDir(), "b.zip")
	stdout, stderr, err := runBackup(t, "--output", out, "--url", server.URL+"/api", "--token", "t")
	if err != nil {
		t.Fatalf("backup failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}

	got := map[string]string{}
	var manifest backup.BackupManifest
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		rc.Close()
		if f.Name == "manifest.json" {
			if err := json.Unmarshal(b, &manifest); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if strings.HasPrefix(f.Name, "media/") {
			got[f.Name] = string(b)
		}
	}

	want := map[string]string{"media/media-1/photo.png": "bytesA", "media/media-2/photo.png": "bytesB"}
	for name, body := range want {
		if got[name] != body {
			t.Errorf("entry %s = %q, want %q (all media entries: %v)", name, got[name], body, got)
		}
	}
	if len(got) != 2 {
		t.Errorf("expected exactly 2 media entries, got %d: %v", len(got), got)
	}
	for id, path := range map[string]string{"media-1": "media/media-1/photo.png", "media-2": "media/media-2/photo.png"} {
		if manifest.MediaIndex[id].ArchivePath != path {
			t.Errorf("manifest archivePath for %s = %q, want %q", id, manifest.MediaIndex[id].ArchivePath, path)
		}
	}
}
