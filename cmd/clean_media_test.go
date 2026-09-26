package cmd

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func runClean(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TROKKY_URL", "")
	t.Setenv("TROKKY_TOKEN", "")
	t.Setenv("TROKKY_INSTANCE", "")
	for name, def := range map[string]string{"collections": "", "media-only": "false", "documents-only": "false", "dry-run": "false", "confirm": "false"} {
		if err := cleanCmd.Flags().Set(name, def); err != nil {
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
	rootCmd.SetArgs(append([]string{"clean"}, args...))
	var err error
	rawOut, rawErr := captureProcessOutput(t, func() {
		err = rootCmd.Execute()
	})
	return stdout.String() + rawOut, stderr.String() + rawErr, err
}

// A token that can list media but not delete it gets the same first page back forever;
// clean must stop with an error instead of looping.
func TestCleanStopsWhenMediaCannotBeDeleted(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && apiPath(r) == "/media":
			io.WriteString(w, `{"success":true,"data":[{"id":"m1"},{"id":"m2"}]}`)
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusForbidden)
			io.WriteString(w, `{"success":false,"error":{"message":"media:delete required"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"success":false}`)
		}
	}))
	defer server.Close()

	done := make(chan error, 1)
	go func() {
		_, _, err := runClean(t, "--media-only", "--confirm", "--url", server.URL+"/api", "--token", "t")
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "could not be deleted") {
			t.Fatalf("err = %v, want a media deletion failure", err)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("clean did not stop; it loops on media it cannot delete")
	}
	if n := rec.countPath("/media"); n != 1 {
		t.Errorf("listed media %d times, want 1", n)
	}
}

// A backup that cannot list media must fail rather than write an archive without media.
func TestBackupFailsWhenMediaCannotBeListed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && apiPath(r) == "/collections":
			io.WriteString(w, testCollectionsResponse)
		case r.Method == http.MethodGet && strings.HasPrefix(apiPath(r), "/collections/"):
			io.WriteString(w, `{"success":true,"data":{"documents":[]}}`)
		case apiPath(r) == "/media":
			w.WriteHeader(http.StatusForbidden)
			io.WriteString(w, `{"success":false,"error":{"message":"media:read required"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"success":false}`)
		}
	}))
	defer server.Close()

	out := filepath.Join(t.TempDir(), "b.zip")
	_, _, err := runBackup(t, "--output", out, "--url", server.URL+"/api", "--token", "t")
	if err == nil || !strings.Contains(err.Error(), "could not list media") {
		t.Fatalf("err = %v, want the media listing failure", err)
	}
}
