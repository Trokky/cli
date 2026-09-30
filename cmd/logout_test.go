package cmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/trokky/cli/internal/config"
)

func runLogout(t *testing.T, name string) (string, error) {
	t.Helper()
	var err error
	out, _ := captureProcessOutput(t, func() {
		rootCmd.SetArgs([]string{"logout", name, "--force"})
		err = rootCmd.Execute()
	})
	return out, err
}

func addTestInstance(t *testing.T, name string, inst config.InstanceConfig) {
	t.Helper()
	if err := config.AddInstance(name, inst, false); err != nil {
		t.Fatal(err)
	}
}

func TestLogout_RevokesABrowserSignInThenForgetsIt(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var body map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/auth/revoke" {
			t.Errorf("unexpected request to %s", r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&body)
	}))
	defer server.Close()
	addTestInstance(t, "site", config.InstanceConfig{URL: server.URL + "/api", Token: "a", RefreshToken: "r", AuthType: config.AuthTypeOAuth2, ClientID: "trokky-cli"})

	out, err := runLogout(t, "site")
	if err != nil {
		t.Fatal(err)
	}
	if body["token"] != "r" || body["client_id"] != "trokky-cli" {
		t.Fatalf("revocation sent %v", body)
	}
	if strings.Contains(out, "revoke") {
		t.Fatalf("a successful revocation needs no note: %q", out)
	}
	if inst, _ := config.GetInstance("site"); inst != nil {
		t.Fatal("instance still configured")
	}
}

func TestLogout_ForgetsAnUnreachableOrRefusingInstanceAndSaysWhy(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	addTestInstance(t, "gone", config.InstanceConfig{URL: "http://127.0.0.1:1/api", RefreshToken: "r", AuthType: config.AuthTypeOAuth2})
	out, err := runLogout(t, "gone")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Preferences > Connected applications") || !strings.Contains(out, "could not be reached") {
		t.Fatalf("output %q", out)
	}
	if inst, _ := config.GetInstance("gone"); inst != nil {
		t.Fatal("an unreachable instance must still be forgotten")
	}

	old := httptest.NewServer(http.NotFoundHandler())
	defer old.Close()
	addTestInstance(t, "old", config.InstanceConfig{URL: old.URL + "/api", RefreshToken: "r", AuthType: config.AuthTypeOAuth2})
	out, err = runLogout(t, "old")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Preferences > Connected applications") || !strings.Contains(out, "did not accept the revocation (HTTP 404)") {
		t.Fatalf("output %q", out)
	}
	if inst, _ := config.GetInstance("old"); inst != nil {
		t.Fatal("a refusing instance must still be forgotten")
	}
}

func TestLogout_DoesNotRevokeAnAPIToken(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer server.Close()
	addTestInstance(t, "tok", config.InstanceConfig{URL: server.URL + "/api", Token: "api-token", AuthType: config.AuthTypeAPIToken})

	out, err := runLogout(t, "tok")
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 || strings.Contains(out, "revoke") {
		t.Fatalf("an API token is not an OAuth grant: %d calls, output %q", calls.Load(), out)
	}
}

// A browser sign-in that stopped working says how to sign in again
func TestCommand_Unauthorized_OAuthInstance_HintsAtLogin(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TROKKY_URL", "")
	t.Setenv("TROKKY_TOKEN", "")
	t.Setenv("TROKKY_INSTANCE", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"success":false,"error":{"message":"Invalid or expired authentication token"}}`))
	}))
	defer server.Close()
	addTestInstance(t, "site", config.InstanceConfig{URL: server.URL + "/api", Token: "a", AuthType: config.AuthTypeOAuth2})

	var err error
	captureProcessOutput(t, func() {
		// rootCmd keeps flags from earlier tests: clear the ones that would override the instance
		rootCmd.SetArgs([]string{"documents", "list", "posts", "--instance", "site", "--url", "", "--token", "", "-q"})
		err = rootCmd.Execute()
	})
	if err == nil || !strings.Contains(err.Error(), "trokky login "+server.URL+"/api --name site") {
		t.Fatalf("err = %v", err)
	}
}

// An API token does not come from a sign-in, so its 401 is not answered with "trokky login"
func TestCommand_Unauthorized_APIToken_NoLoginHint(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TROKKY_URL", "")
	t.Setenv("TROKKY_TOKEN", "")
	t.Setenv("TROKKY_INSTANCE", "")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"success":false,"error":{"message":"Invalid API token"}}`))
	}))
	defer server.Close()
	addTestInstance(t, "tok", config.InstanceConfig{URL: server.URL + "/api", Token: "t", AuthType: config.AuthTypeAPIToken})

	var err error
	captureProcessOutput(t, func() {
		rootCmd.SetArgs([]string{"documents", "list", "posts", "--instance", "tok", "--url", "", "--token", "", "-q"})
		err = rootCmd.Execute()
	})
	if err == nil || !strings.Contains(err.Error(), "Invalid API token") || strings.Contains(err.Error(), "trokky login") {
		t.Fatalf("err = %v", err)
	}
}
