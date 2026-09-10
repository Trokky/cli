package cmd

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// recorder captures every request a fake Trokky instance receives so a test can
// assert on what the restore actually did — in particular that nothing was
// uploaded before the documents were written.
type recorder struct {
	mu       sync.Mutex
	requests []recordedRequest
}

type recordedRequest struct {
	Method string
	Path   string
	Body   string
}

// apiPath is the request path with the instance's "/api" base stripped, so
// tests can assert on "/collections/posts" regardless of how the URL was
// normalised.
func apiPath(req *http.Request) string {
	return strings.TrimPrefix(req.URL.Path, "/api")
}

func (r *recorder) record(req *http.Request) {
	body, _ := io.ReadAll(req.Body)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, recordedRequest{Method: req.Method, Path: apiPath(req), Body: string(body)})
}

func (r *recorder) all() []recordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]recordedRequest, len(r.requests))
	copy(out, r.requests)
	return out
}

func (r *recorder) countPath(path string) int {
	n := 0
	for _, req := range r.all() {
		if req.Path == path {
			n++
		}
	}
	return n
}

const (
	testCollectionsResponse = `{"success":true,"data":{"collections":[{"name":"posts","title":"Posts","fields":[{"name":"title","type":"string"}]}]}}`
	testOldMediaID          = "media-old-1"
	testNewMediaID          = "media-new-1"
	testNewDocID            = "doc-new-1"
)

// writeBackupZip builds a minimal but realistic v2 backup archive: a manifest,
// one document that references one media file, and that media file.
func writeBackupZip(t *testing.T) string {
	t.Helper()

	manifest := map[string]interface{}{
		"version":   "2.0",
		"timestamp": "2026-01-01T00:00:00Z",
		"source":    map[string]interface{}{"url": "https://cms.example.com/api"},
		"schemas": []map[string]interface{}{
			{
				"name":   "posts",
				"title":  "Posts",
				"fields": []map[string]interface{}{{"name": "title", "type": "string"}},
			},
		},
		"dependencyGraph": map[string][]string{},
		"restoreOrder":    []string{"posts"},
		"mediaIndex": map[string]interface{}{
			testOldMediaID: map[string]interface{}{
				"filename": "photo.png",
				"mimeType": "image/png",
				"size":     3,
			},
		},
		"statistics": map[string]interface{}{"totalDocuments": 1, "totalMedia": 1},
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}

	doc := map[string]interface{}{
		"id":    "doc-old-1",
		"title": "Example post",
		"cover": map[string]interface{}{
			"asset": map[string]interface{}{"_ref": testOldMediaID},
		},
	}
	docJSON, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "backup.zip")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	entries := []struct {
		name string
		data []byte
	}{
		{"manifest.json", manifestJSON},
		{"collections/posts/doc-old-1.json", docJSON},
		{"media/photo.png", []byte("png")},
	}
	for _, e := range entries {
		w, err := zw.Create(e.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(e.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// runRestore drives the real restore command through cobra with the given args
// and returns its stdout and stderr.
func runRestore(t *testing.T, args ...string) (string, string, error) {
	t.Helper()

	// --url/--token take priority over the config file, but point HOME at a temp
	// dir so a developer's ~/.trokky/config.yaml can never leak in.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TROKKY_URL", "")
	t.Setenv("TROKKY_TOKEN", "")
	t.Setenv("TROKKY_INSTANCE", "")

	// Cobra flags are sticky across Execute calls on the shared rootCmd.
	for name, def := range map[string]string{
		"collections":       "",
		"with-dependencies": "false",
		"clean":             "false",
		"overwrite":         "false",
		"dry-run":           "false",
	} {
		if err := restoreCmd.Flags().Set(name, def); err != nil {
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

	rootCmd.SetArgs(append([]string{"restore"}, args...))

	// Anything printed straight to the process streams counts as output too.
	var err error
	rawOut, rawErr := captureProcessOutput(t, func() {
		err = rootCmd.Execute()
	})

	return stdout.String() + rawOut, stderr.String() + rawErr, err
}

// captureProcessOutput runs fn with os.Stdout/os.Stderr redirected to pipes and
// returns what was written to each.
func captureProcessOutput(t *testing.T, fn func()) (string, string) {
	t.Helper()

	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = outW, errW

	var wg sync.WaitGroup
	var outBuf, errBuf bytes.Buffer
	wg.Add(2)
	go func() { defer wg.Done(); io.Copy(&outBuf, outR) }()
	go func() { defer wg.Done(); io.Copy(&errBuf, errR) }()

	fn()

	os.Stdout, os.Stderr = origOut, origErr
	outW.Close()
	errW.Close()
	wg.Wait()
	outR.Close()
	errR.Close()

	return outBuf.String(), errBuf.String()
}

// A token with content:read/media:read but no content:write must be rejected
// before a single media file is uploaded — uploading first mints new media IDs
// and orphans them against documents that were never written.
func TestRestoreAbortsBeforeUploadingMediaWhenDocumentWritesAreForbidden(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		if r.Method == http.MethodGet && apiPath(r) == "/collections" {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, testCollectionsResponse)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		io.WriteString(w, `{"success":false,"error":{"message":"content:write scope required"}}`)
	}))
	defer server.Close()

	stdout, _, err := runRestore(t, "--input", writeBackupZip(t), "--url", server.URL+"/api", "--token", "read-only", "-q")

	want := "token lacks content:write (required to restore documents): "
	if err == nil {
		t.Error("expected an error for a token that cannot write documents")
	} else if !strings.HasPrefix(err.Error(), want) {
		t.Errorf("error = %q, want it to start with %q", err.Error(), want)
	}
	if n := rec.countPath("/media/upload"); n != 0 {
		t.Errorf("%d request(s) hit /media/upload; the restore must abort before uploading anything", n)
	}
	if strings.Contains(stdout, "Restore completed") {
		t.Errorf("output claims success after a failed restore:\n%s", stdout)
	}
}

// A restore where every document write fails must exit non-zero and must not
// claim to have completed.
func TestRestoreFailedDocumentWritesExitNonZero(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		if r.Method == http.MethodGet && apiPath(r) == "/collections" {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, testCollectionsResponse)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"success":false,"error":{"message":"boom"}}`)
	}))
	defer server.Close()

	stdout, _, err := runRestore(t, "--input", writeBackupZip(t), "--url", server.URL+"/api", "--token", "t", "-q")

	if err == nil {
		t.Error("expected a non-zero exit when no document could be restored")
	} else if !strings.Contains(err.Error(), "restore failed") {
		t.Errorf("error = %q, want it to mention that the restore failed", err.Error())
	}
	if strings.Contains(stdout, "Restore completed") {
		t.Errorf("output claims success after a failed restore:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Documents restored:    0") {
		t.Errorf("summary does not report 0 documents restored:\n%s", stdout)
	}
	if n := rec.countPath("/media/upload"); n != 0 {
		t.Errorf("%d media upload(s) happened even though no document was restored", n)
	}
}

// Counters must describe work that actually landed on the server: when the
// reference-fix pass is rejected, "References updated" stays at 0 and the user
// is warned that the uploaded media is orphaned.
func TestRestoreCountersReportOnlyRealWork(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && apiPath(r) == "/collections":
			io.WriteString(w, testCollectionsResponse)
		case r.Method == http.MethodPost && apiPath(r) == "/collections/posts":
			io.WriteString(w, `{"success":true,"data":{"id":"`+testNewDocID+`"}}`)
		case r.Method == http.MethodPost && apiPath(r) == "/media/upload":
			io.WriteString(w, `{"success":true,"data":{"files":[{"id":"`+testNewMediaID+`"}]}}`)
		default:
			w.WriteHeader(http.StatusForbidden)
			io.WriteString(w, `{"success":false,"error":{"message":"content:write scope required"}}`)
		}
	}))
	defer server.Close()

	stdout, stderr, err := runRestore(t, "--input", writeBackupZip(t), "--url", server.URL+"/api", "--token", "t", "-q")

	if err == nil {
		t.Error("expected a non-zero exit when media references could not be written")
	}
	if !strings.Contains(stdout, "References updated:    0") {
		t.Errorf("References updated must count only documents that were rewritten:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Media restored:        1") {
		t.Errorf("summary does not report the uploaded media file:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Documents restored:    1") {
		t.Errorf("summary does not report the restored document:\n%s", stdout)
	}
	if strings.Contains(stdout, "Restore completed") {
		t.Errorf("output claims success although the documents still point at the old media IDs:\n%s", stdout)
	}
	if !strings.Contains(stderr, "ORPHANED MEDIA") {
		t.Errorf("no orphaned-media warning on stderr:\n%s", stderr)
	}
	if !strings.Contains(stderr, "1 media file(s) were uploaded under new IDs") {
		t.Errorf("orphaned-media warning does not name the counts:\n%s", stderr)
	}
}

func TestRestoreHappyPathReportsTrueCounts(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && apiPath(r) == "/collections":
			io.WriteString(w, testCollectionsResponse)
		case r.Method == http.MethodPost && apiPath(r) == "/collections/posts":
			io.WriteString(w, `{"success":true,"data":{"id":"`+testNewDocID+`"}}`)
		case r.Method == http.MethodPost && apiPath(r) == "/media/upload":
			io.WriteString(w, `{"success":true,"data":{"files":[{"id":"`+testNewMediaID+`"}]}}`)
		case r.Method == http.MethodPut && apiPath(r) == "/collections/posts/"+testNewDocID:
			io.WriteString(w, `{"success":true,"data":{"id":"`+testNewDocID+`"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"success":false,"error":{"message":"unexpected `+r.Method+` `+apiPath(r)+`"}}`)
		}
	}))
	defer server.Close()

	stdout, stderr, err := runRestore(t, "--input", writeBackupZip(t), "--url", server.URL+"/api", "--token", "t", "-q")
	if err != nil {
		t.Fatalf("restore failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	for _, want := range []string{
		"Restore completed",
		"Documents restored:    1",
		"Media restored:        1",
		"References updated:    1",
		"Collections restored:  1",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output is missing %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stderr, "ORPHANED MEDIA") {
		t.Errorf("orphaned-media warning on a clean run:\n%s", stderr)
	}

	// The document must be POSTed before the media is uploaded, and the
	// reference-fix PUT must carry the new media ID.
	var order []string
	var putBody string
	for _, req := range rec.all() {
		if req.Method == http.MethodPost || req.Method == http.MethodPut {
			order = append(order, req.Method+" "+req.Path)
		}
		if req.Method == http.MethodPut && req.Path == "/collections/posts/"+testNewDocID {
			putBody = req.Body
		}
	}
	wantOrder := []string{
		"POST /collections/posts",
		"POST /media/upload",
		"PUT /collections/posts/" + testNewDocID,
	}
	if strings.Join(order, ", ") != strings.Join(wantOrder, ", ") {
		t.Errorf("write order = %v, want %v", order, wantOrder)
	}
	if !strings.Contains(putBody, testNewMediaID) {
		t.Errorf("reference-fix PUT body does not contain the new media ID: %s", putBody)
	}
	if strings.Contains(putBody, testOldMediaID) {
		t.Errorf("reference-fix PUT body still contains the old media ID: %s", putBody)
	}
}

func TestRestoreDryRunMakesNoWrites(t *testing.T) {
	rec := &recorder{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.record(r)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && apiPath(r) == "/collections" {
			io.WriteString(w, testCollectionsResponse)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		io.WriteString(w, `{"success":false,"error":{"message":"a dry run must not write"}}`)
	}))
	defer server.Close()

	stdout, _, err := runRestore(t, "--input", writeBackupZip(t), "--url", server.URL+"/api", "--token", "t", "--dry-run", "-q")
	if err != nil {
		t.Fatalf("dry run failed: %v\n%s", err, stdout)
	}
	for _, want := range []string{
		"[DRY RUN] Would restore 1 media file(s)",
		"[DRY RUN] Would restore 1 document(s) to posts",
		"Dry run completed - no changes made",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output is missing %q:\n%s", want, stdout)
		}
	}
	for _, req := range rec.all() {
		if req.Method != http.MethodGet {
			t.Errorf("dry run issued a %s to %s", req.Method, req.Path)
		}
	}
}

func TestIsPermissionDenied(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"envelope 403", errors.New("content:write scope required (HTTP 403)"), true},
		{"envelope 401", errors.New("invalid token (HTTP 401)"), true},
		{"raw 403", errors.New("HTTP 403: Forbidden"), true},
		{"raw 401", errors.New("HTTP 401: Unauthorized"), true},
		{"upload 403", errors.New("upload failed: HTTP 403: Forbidden"), true},
		{"server error", errors.New("boom (HTTP 500)"), false},
		{"not found", errors.New("HTTP 404: not found"), false},
		{"conflict", errors.New("document already exists (HTTP 409)"), false},
		{"transport error", errors.New("request failed: connection refused"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isPermissionDenied(tt.err); got != tt.want {
				t.Errorf("isPermissionDenied(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
