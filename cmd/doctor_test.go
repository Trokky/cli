package cmd

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func runDoctor(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TROKKY_URL", "")
	t.Setenv("TROKKY_TOKEN", "")
	t.Setenv("TROKKY_INSTANCE", "")
	for name, def := range map[string]string{"json": "false", "strict": "false"} {
		if err := doctorCmd.Flags().Set(name, def); err != nil {
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
	rootCmd.SetArgs(append([]string{"doctor"}, args...))
	var err error
	rawOut, rawErr := captureProcessOutput(t, func() {
		err = rootCmd.Execute()
	})
	return stdout.String() + rawOut, stderr.String() + rawErr, err
}

func doctorServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if apiPath(r) != "/system/doctor" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		io.WriteString(w, body)
	}))
	t.Cleanup(server.Close)
	return server
}

func reportJSON(t *testing.T, status string, checks ...doctorCheck) string {
	t.Helper()
	data, err := json.Marshal(map[string]interface{}{
		"success": true,
		"data":    doctorReport{Environment: "production", Status: status, Checks: checks},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestDoctorPrintsEachCheckAndFailsOnAFailure(t *testing.T) {
	server := doctorServer(t, http.StatusOK, reportJSON(t, "fail",
		doctorCheck{ID: "signing-key", Title: "Signing key (JWT secret)", Status: "ok", Details: []string{"Set, 48 characters."}},
		doctorCheck{ID: "content", Title: "Stored content", Status: "fail", Details: []string{"posts: 2 document(s) fail validation, by field: body (2)."}},
	))

	stdout, _, err := runDoctor(t, "--url", server.URL+"/api", "--token", "t")
	if err == nil {
		t.Fatal("a failed check must exit non-zero")
	}
	for _, want := range []string{
		"(production)",
		"[ok  ] Signing key (JWT secret)",
		"[FAIL] Stored content",
		"posts: 2 document(s) fail validation",
		"Some checks failed.",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("output is missing %q:\n%s", want, stdout)
		}
	}
}

func TestDoctorWarningsPassUnlessStrict(t *testing.T) {
	server := doctorServer(t, http.StatusOK, reportJSON(t, "warn",
		doctorCheck{ID: "schemas", Title: "Schemas", Status: "warn", Details: []string{`page: unknown key "isSingleton"`}},
	))

	stdout, _, err := runDoctor(t, "--url", server.URL+"/api", "--token", "t")
	if err != nil {
		t.Fatalf("a warning alone must not fail: %v", err)
	}
	if !strings.Contains(stdout, "[WARN] Schemas") || !strings.Contains(stdout, "No failures, some warnings.") {
		t.Errorf("unexpected output:\n%s", stdout)
	}

	if _, _, err := runDoctor(t, "--url", server.URL+"/api", "--token", "t", "--strict"); err == nil {
		t.Error("--strict must fail on a warning")
	}
}

func TestDoctorJSON(t *testing.T) {
	server := doctorServer(t, http.StatusOK, reportJSON(t, "ok",
		doctorCheck{ID: "storage", Title: "Storage persistence", Status: "ok", Details: []string{"fine"}},
	))
	stdout, _, err := runDoctor(t, "--url", server.URL+"/api", "--token", "t", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var report doctorReport
	if err := json.Unmarshal([]byte(stdout), &report); err != nil {
		t.Fatalf("--json output is not the report: %v\n%s", err, stdout)
	}
	if report.Status != "ok" || report.Checks[0].ID != "storage" {
		t.Errorf("unexpected report: %+v", report)
	}
}

func TestDoctorExplainsAnOldServerAWrongURLAndAMissingPermission(t *testing.T) {
	// An older server: the API answers /api/health but has no doctor endpoint
	old := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/health" {
			io.WriteString(w, `{"success":true,"data":{"status":"healthy"}}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
		io.WriteString(w, `<!DOCTYPE html><html><body>Cannot GET /api/system/doctor</body></html>`)
	}))
	defer old.Close()
	_, _, err := runDoctor(t, "--url", old.URL+"/api", "--token", "t")
	if err == nil || !strings.Contains(err.Error(), "needs Trokky 3.6 or later") || strings.Contains(err.Error(), "<html>") {
		t.Errorf("older server: error = %v", err)
	}

	// A server that only answers health at the root
	rootOnly := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/health" {
			io.WriteString(w, `{"status":"healthy"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer rootOnly.Close()
	_, _, err = runDoctor(t, "--url", rootOnly.URL+"/api", "--token", "t")
	if err == nil || !strings.Contains(err.Error(), "needs Trokky 3.6 or later") {
		t.Errorf("root health: error = %v", err)
	}

	// A wrong URL: nothing answers at all
	wrong := doctorServer(t, http.StatusNotFound, `<html>nope</html>`)
	_, _, err = runDoctor(t, "--url", wrong.URL+"/not-the-api", "--token", "t")
	if err == nil || !strings.Contains(err.Error(), "no Trokky API answered") {
		t.Errorf("wrong URL: error = %v", err)
	}

	forbidden := doctorServer(t, http.StatusForbidden, `{"success":false,"error":{"message":"nope"}}`)
	_, _, err = runDoctor(t, "--url", forbidden.URL+"/api", "--token", "t")
	if err == nil || !strings.Contains(err.Error(), "settings:read") || !strings.Contains(err.Error(), "API token") {
		t.Errorf("forbidden: error = %v", err)
	}
}

func TestDoctorRejectsAnUnreadableResponse(t *testing.T) {
	server := doctorServer(t, http.StatusOK, `{"success":true,"data":{"something":"else"}}`)
	if _, _, err := runDoctor(t, "--url", server.URL+"/api", "--token", "t"); err == nil || !strings.Contains(err.Error(), "could not be read") {
		t.Errorf("error = %v", err)
	}
}
