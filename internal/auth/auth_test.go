package auth

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	cfg "github.com/trokky/cli/internal/config"
)

func overrideHome(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
}

// --- StartDeviceAuth ---

func TestStartDeviceAuth_Success(t *testing.T) {
	var gotPath, gotClientID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		gotClientID = body["client_id"]

		json.NewEncoder(w).Encode(DeviceAuthResponse{
			DeviceCode:              "dev-code-123",
			UserCode:                "ABCD-EFGH",
			VerificationURI:         "https://auth.example.com/device",
			VerificationURIComplete: "https://auth.example.com/device?code=ABCD-EFGH",
			ExpiresIn:               900,
			Interval:                5,
		})
	}))
	defer server.Close()

	result, err := StartDeviceAuth(server.URL)
	if err != nil {
		t.Fatalf("StartDeviceAuth() error: %v", err)
	}
	if gotPath != "/auth/device" {
		t.Fatalf("path = %q, want /auth/device", gotPath)
	}
	if gotClientID != ClientID {
		t.Fatalf("client_id = %q, want %q", gotClientID, ClientID)
	}
	if result.DeviceCode != "dev-code-123" {
		t.Fatalf("DeviceCode = %q", result.DeviceCode)
	}
	if result.UserCode != "ABCD-EFGH" {
		t.Fatalf("UserCode = %q", result.UserCode)
	}
	if result.ExpiresIn != 900 {
		t.Fatalf("ExpiresIn = %d", result.ExpiresIn)
	}
	if result.Interval != 5 {
		t.Fatalf("Interval = %d", result.Interval)
	}
}

func TestStartDeviceAuth_TrailingSlash(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode(DeviceAuthResponse{DeviceCode: "x"})
	}))
	defer server.Close()

	StartDeviceAuth(server.URL + "/")
	if gotPath != "/auth/device" {
		t.Fatalf("trailing slash not trimmed, got %q", gotPath)
	}
}

func TestStartDeviceAuth_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		json.NewEncoder(w).Encode(TokenErrorResponse{Error: "server_error", ErrorDescription: "internal failure"})
	}))
	defer server.Close()

	_, err := StartDeviceAuth(server.URL)
	if err == nil {
		t.Fatal("expected error on 500")
	}
	if !strings.Contains(err.Error(), "internal failure") {
		t.Fatalf("expected error description, got %q", err.Error())
	}
}

func TestStartDeviceAuth_ConnectionError(t *testing.T) {
	_, err := StartDeviceAuth("http://127.0.0.1:1")
	if err == nil {
		t.Fatal("expected error on connection failure")
	}
}

// --- PollForToken ---

func TestPollForToken_ImmediateSuccess(t *testing.T) {
	var gotDeviceCode, gotClientID string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		gotDeviceCode = body["device_code"]
		gotClientID = body["client_id"]

		json.NewEncoder(w).Encode(TokenResponse{
			AccessToken:  "access-tok",
			RefreshToken: "refresh-tok",
			ExpiresIn:    3600,
			Scope:        "openid profile",
		})
	}))
	defer server.Close()

	// interval=0 so it polls immediately (after 0s sleep)
	result, err := PollForToken(server.URL, "dev-code", 0, 10)
	if err != nil {
		t.Fatalf("PollForToken() error: %v", err)
	}
	if result.AccessToken != "access-tok" {
		t.Fatalf("AccessToken = %q", result.AccessToken)
	}
	if result.RefreshToken != "refresh-tok" {
		t.Fatalf("RefreshToken = %q", result.RefreshToken)
	}
	if gotDeviceCode != "dev-code" {
		t.Fatalf("device_code = %q, want 'dev-code'", gotDeviceCode)
	}
	if gotClientID != ClientID {
		t.Fatalf("client_id = %q, want %q", gotClientID, ClientID)
	}
}

func TestPollForToken_PendingThenSuccess(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 3 {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(TokenErrorResponse{Error: "authorization_pending"})
			return
		}
		json.NewEncoder(w).Encode(TokenResponse{AccessToken: "got-it", ExpiresIn: 3600})
	}))
	defer server.Close()

	result, err := PollForToken(server.URL, "code", 0, 10)
	if err != nil {
		t.Fatalf("PollForToken() error: %v", err)
	}
	if result.AccessToken != "got-it" {
		t.Fatalf("AccessToken = %q", result.AccessToken)
	}
	if callCount < 3 {
		t.Fatalf("expected at least 3 calls, got %d", callCount)
	}
}

func TestPollForToken_AccessDenied(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(TokenErrorResponse{Error: "access_denied"})
	}))
	defer server.Close()

	_, err := PollForToken(server.URL, "code", 0, 10)
	if err == nil {
		t.Fatal("expected error on access_denied")
	}
	if !strings.Contains(err.Error(), "denied") {
		t.Fatalf("expected 'denied' in error, got %q", err.Error())
	}
}

func TestPollForToken_ExpiredToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(TokenErrorResponse{Error: "expired_token"})
	}))
	defer server.Close()

	_, err := PollForToken(server.URL, "code", 0, 10)
	if err == nil {
		t.Fatal("expected error on expired_token")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected 'expired' in error, got %q", err.Error())
	}
}

func TestPollForToken_SlowDown(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			w.WriteHeader(400)
			json.NewEncoder(w).Encode(TokenErrorResponse{Error: "slow_down"})
			return
		}
		json.NewEncoder(w).Encode(TokenResponse{AccessToken: "ok", ExpiresIn: 3600})
	}))
	defer server.Close()

	result, err := PollForToken(server.URL, "code", 0, 30)
	if err != nil {
		t.Fatalf("PollForToken() error: %v", err)
	}
	if result.AccessToken != "ok" {
		t.Fatalf("AccessToken = %q", result.AccessToken)
	}
}

// --- IsTokenExpired ---

func TestIsTokenExpired_Empty(t *testing.T) {
	if IsTokenExpired("", 0) {
		t.Fatal("empty tokenExpiresAt should not be expired")
	}
}

func TestIsTokenExpired_Future(t *testing.T) {
	future := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
	if IsTokenExpired(future, 0) {
		t.Fatal("token expiring in 1 hour should not be expired")
	}
}

func TestIsTokenExpired_Past(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	if !IsTokenExpired(past, 0) {
		t.Fatal("token that expired 1 hour ago should be expired")
	}
}

func TestIsTokenExpired_WithinBuffer(t *testing.T) {
	// Expires in 2 minutes, but buffer is 5 minutes -> should be "expired"
	soon := time.Now().Add(2 * time.Minute).UTC().Format(time.RFC3339)
	if !IsTokenExpired(soon, 300) {
		t.Fatal("token within buffer window should be considered expired")
	}
}

func TestIsTokenExpired_OutsideBuffer(t *testing.T) {
	// Expires in 10 minutes, buffer is 5 minutes -> not expired
	later := time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339)
	if IsTokenExpired(later, 300) {
		t.Fatal("token outside buffer window should not be expired")
	}
}

func TestIsTokenExpired_InvalidFormat(t *testing.T) {
	if IsTokenExpired("not-a-date", 0) {
		t.Fatal("invalid date format should not be considered expired")
	}
}

// --- RefreshAccessToken ---

func TestRefreshAccessToken_Success(t *testing.T) {
	overrideHome(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		json.NewDecoder(r.Body).Decode(&body)
		if body["grant_type"] != "refresh_token" {
			t.Errorf("grant_type = %q", body["grant_type"])
		}
		if body["refresh_token"] != "refresh-tok" {
			t.Errorf("refresh_token = %q", body["refresh_token"])
		}

		json.NewEncoder(w).Encode(TokenResponse{
			AccessToken:  "new-access",
			RefreshToken: "new-refresh",
			ExpiresIn:    3600,
		})
	}))
	defer server.Close()

	cfg.AddInstance("test", cfg.InstanceConfig{
		URL:          server.URL,
		Token:        "old-access",
		RefreshToken: "refresh-tok",
		AuthType:     cfg.AuthTypeOAuth2,
	}, true)

	result := RefreshAccessToken("test", cfg.InstanceConfig{
		URL:          server.URL,
		Token:        "old-access",
		RefreshToken: "refresh-tok",
		AuthType:     cfg.AuthTypeOAuth2,
	})

	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}
	if result.Token != "new-access" {
		t.Fatalf("Token = %q", result.Token)
	}
	if result.ExpiresAt == "" {
		t.Fatal("ExpiresAt should be set")
	}

	// Verify config was updated
	inst, _ := cfg.GetInstance("test")
	if inst != nil && inst.Token != "new-access" {
		t.Fatalf("stored token = %q, want 'new-access'", inst.Token)
	}
}

func TestRefreshAccessToken_NoRefreshToken(t *testing.T) {
	result := RefreshAccessToken("x", cfg.InstanceConfig{
		URL:      "http://localhost",
		Token:    "tok",
		AuthType: "oauth2",
	})
	if result.Success {
		t.Fatal("expected failure when no refresh token")
	}
	if !result.RequiresReauth {
		t.Fatal("should require reauth")
	}
}

func TestRefreshAccessToken_NotOAuth2(t *testing.T) {
	result := RefreshAccessToken("x", cfg.InstanceConfig{
		URL:          "http://localhost",
		Token:        "tok",
		RefreshToken: "rt",
		AuthType:     cfg.AuthTypeAPIToken,
	})
	if result.Success {
		t.Fatal("expected failure for non-oauth2 instance")
	}
}

func TestRefreshAccessToken_ServerRejects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		json.NewEncoder(w).Encode(TokenErrorResponse{Error: "invalid_grant", ErrorDescription: "refresh token expired"})
	}))
	defer server.Close()

	result := RefreshAccessToken("x", cfg.InstanceConfig{
		URL:          server.URL,
		Token:        "old",
		RefreshToken: "expired-rt",
		AuthType:     cfg.AuthTypeOAuth2,
	})
	if result.Success {
		t.Fatal("expected failure")
	}
	if !result.RequiresReauth {
		t.Fatal("401 should require reauth")
	}
	if !strings.Contains(result.Error, "refresh token expired") {
		t.Fatalf("error = %q", result.Error)
	}
}

// --- GetValidToken ---

func TestGetValidToken_NotOAuth2(t *testing.T) {
	token, refreshed, err := GetValidToken("x", cfg.InstanceConfig{
		Token:    "my-token",
		AuthType: "api-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	if token != "my-token" {
		t.Fatalf("token = %q", token)
	}
	if refreshed {
		t.Fatal("should not be refreshed")
	}
}

func TestGetValidToken_NotExpired(t *testing.T) {
	future := time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339)
	token, refreshed, err := GetValidToken("x", cfg.InstanceConfig{
		Token:          "valid-token",
		AuthType:       "oauth2",
		TokenExpiresAt: future,
	})
	if err != nil {
		t.Fatal(err)
	}
	if token != "valid-token" {
		t.Fatalf("token = %q", token)
	}
	if refreshed {
		t.Fatal("should not be refreshed")
	}
}

func TestGetValidToken_Expired_RefreshSucceeds(t *testing.T) {
	overrideHome(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(TokenResponse{AccessToken: "fresh", ExpiresIn: 3600})
	}))
	defer server.Close()

	past := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	cfg.AddInstance("x", cfg.InstanceConfig{
		URL:            server.URL,
		Token:          "stale",
		RefreshToken:   "rt",
		AuthType:       "oauth2",
		TokenExpiresAt: past,
	}, true)

	token, refreshed, err := GetValidToken("x", cfg.InstanceConfig{
		URL:            server.URL,
		Token:          "stale",
		RefreshToken:   "rt",
		AuthType:       "oauth2",
		TokenExpiresAt: past,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if token != "fresh" {
		t.Fatalf("token = %q, want 'fresh'", token)
	}
	if !refreshed {
		t.Fatal("should be refreshed")
	}
}

func TestGetValidToken_Expired_RequiresReauth(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	_, _, err := GetValidToken("x", cfg.InstanceConfig{
		URL:            "http://localhost",
		Token:          "stale",
		AuthType:       "oauth2",
		TokenExpiresAt: past,
		// No refresh token -> requires reauth
	})
	if err == nil {
		t.Fatal("expected error when reauth required")
	}
	if !strings.Contains(err.Error(), "session expired") {
		t.Fatalf("expected session expired message, got %q", err.Error())
	}
}

// --- DeriveInstanceName ---

func TestDeriveInstanceName(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://cms.example.com/api", "cms"},
		{"https://my-trokky.example.com", "my-trokky"},
		{"http://localhost:3000/api", "localhost"},
		{"https://example.com", "example"},
		{"not-a-url", "default"},
	}

	for _, tt := range tests {
		got := DeriveInstanceName(tt.url)
		if got != tt.want {
			t.Errorf("DeriveInstanceName(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}
