package useragent

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTransportNamesTheCLI(t *testing.T) {
	Version = "9.9.9"
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = r.UserAgent() }))
	defer srv.Close()

	client := &http.Client{Transport: Transport(nil)}
	if _, err := client.Get(srv.URL); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "trokky-cli/9.9.9 (") || strings.Contains(got, "Go-http-client") {
		t.Fatalf("User-Agent = %q", got)
	}

	// A caller's own User-Agent is kept
	req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
	req.Header.Set("User-Agent", "custom")
	if _, err := client.Do(req); err != nil {
		t.Fatal(err)
	}
	if got != "custom" {
		t.Fatalf("User-Agent = %q, want custom", got)
	}
}
