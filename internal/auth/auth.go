package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/trokky/cli/internal/config"
)

const (
	ClientID      = "trokky-cli"
	DefaultScopes = "openid profile content:read content:write content:delete media:read media:write offline_access"

	deviceAuthPath = "/auth/device"
	tokenPath      = "/auth/token"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

// DeviceAuthResponse is returned by the device authorization endpoint.
type DeviceAuthResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// TokenResponse is returned on successful token exchange.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope"`
}

// TokenErrorResponse is returned when token exchange fails.
type TokenErrorResponse struct {
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description,omitempty"`
}

// Description returns the best available error description.
func (e TokenErrorResponse) Description() string {
	if e.ErrorDescription != "" {
		return e.ErrorDescription
	}
	if e.Error != "" {
		return e.Error
	}
	return "unknown error"
}

// ExpiresAtFromNow computes an RFC3339 expiration timestamp from a duration in seconds.
func ExpiresAtFromNow(expiresInSeconds int) string {
	return time.Now().Add(time.Duration(expiresInSeconds) * time.Second).UTC().Format(time.RFC3339)
}

// StartDeviceAuth initiates the OAuth2 device authorization flow.
func StartDeviceAuth(baseURL string) (*DeviceAuthResponse, error) {
	endpoint := strings.TrimRight(baseURL, "/") + deviceAuthPath

	body, _ := json.Marshal(map[string]string{
		"client_id": ClientID,
		"scope":     DefaultScopes,
	})

	resp, err := httpClient.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var tokenErr TokenErrorResponse
		if json.Unmarshal(data, &tokenErr) == nil && tokenErr.ErrorDescription != "" {
			return nil, fmt.Errorf("device authorization failed: %s", tokenErr.Description())
		}
		return nil, fmt.Errorf("device authorization failed: HTTP %d", resp.StatusCode)
	}

	var result DeviceAuthResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("invalid response from server: %w", err)
	}

	return &result, nil
}

// PollForToken polls the token endpoint until the user authorizes or the code expires.
func PollForToken(baseURL, deviceCode string, interval, expiresIn int) (*TokenResponse, error) {
	endpoint := strings.TrimRight(baseURL, "/") + tokenPath
	pollInterval := time.Duration(interval) * time.Second
	deadline := time.Now().Add(time.Duration(expiresIn) * time.Second)

	for time.Now().Before(deadline) {
		time.Sleep(pollInterval)

		body, _ := json.Marshal(map[string]string{
			"grant_type":  "urn:ietf:params:oauth:grant-type:device_code",
			"device_code": deviceCode,
			"client_id":   ClientID,
		})

		resp, err := httpClient.Post(endpoint, "application/json", bytes.NewReader(body))
		if err != nil {
			continue
		}

		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			var token TokenResponse
			if err := json.Unmarshal(data, &token); err != nil {
				return nil, fmt.Errorf("invalid token response: %w", err)
			}
			return &token, nil
		}

		var tokenErr TokenErrorResponse
		if err := json.Unmarshal(data, &tokenErr); err != nil {
			continue
		}

		switch tokenErr.Error {
		case "authorization_pending":
			continue
		case "slow_down":
			pollInterval += 5 * time.Second
			continue
		case "access_denied":
			return nil, fmt.Errorf("authorization denied by user")
		case "expired_token":
			return nil, fmt.Errorf("device code expired, please try again")
		default:
			return nil, fmt.Errorf("token request failed: %s", tokenErr.Description())
		}
	}

	return nil, fmt.Errorf("device code expired, please try again")
}

// OpenBrowser attempts to open a URL in the user's default browser.
func OpenBrowser(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "linux":
		cmd = exec.Command("xdg-open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		return fmt.Errorf("unsupported platform")
	}
	return cmd.Start()
}

// IsTokenExpired checks if a token is expired or about to expire.
// bufferSeconds defaults to 300 (5 minutes) if 0.
func IsTokenExpired(tokenExpiresAt string, bufferSeconds int) bool {
	if tokenExpiresAt == "" {
		return false
	}
	if bufferSeconds == 0 {
		bufferSeconds = 300
	}

	expiresAt, err := time.Parse(time.RFC3339, tokenExpiresAt)
	if err != nil {
		return false
	}

	return time.Now().After(expiresAt.Add(-time.Duration(bufferSeconds) * time.Second))
}

// TokenRefreshResult holds the outcome of a token refresh attempt.
type TokenRefreshResult struct {
	Success        bool
	Token          string
	ExpiresAt      string
	Error          string
	RequiresReauth bool
}

// RefreshAccessToken refreshes an OAuth2 token using a refresh token.
func RefreshAccessToken(instanceName string, instance config.InstanceConfig) TokenRefreshResult {
	if instance.RefreshToken == "" {
		return TokenRefreshResult{Error: "no refresh token available", RequiresReauth: true}
	}
	if instance.AuthType != config.AuthTypeOAuth2 {
		return TokenRefreshResult{Error: "instance does not use OAuth2", RequiresReauth: false}
	}

	endpoint := strings.TrimRight(instance.URL, "/") + tokenPath

	body, _ := json.Marshal(map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": instance.RefreshToken,
		"client_id":     ClientID,
	})

	resp, err := httpClient.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		return TokenRefreshResult{Error: fmt.Sprintf("token refresh failed: %v", err), RequiresReauth: false}
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		var tokenErr TokenErrorResponse
		json.Unmarshal(data, &tokenErr)
		desc := tokenErr.Description()
		requiresReauth := resp.StatusCode == http.StatusBadRequest || resp.StatusCode == http.StatusUnauthorized
		return TokenRefreshResult{Error: fmt.Sprintf("token refresh failed: %s", desc), RequiresReauth: requiresReauth}
	}

	var tokenResp TokenResponse
	if err := json.Unmarshal(data, &tokenResp); err != nil {
		return TokenRefreshResult{Error: fmt.Sprintf("invalid refresh response: %v", err), RequiresReauth: false}
	}

	expiresAt := ExpiresAtFromNow(tokenResp.ExpiresIn)

	// Update stored config
	cfg, err := config.Load()
	if err == nil {
		if inst, ok := cfg.Instances[instanceName]; ok {
			inst.Token = tokenResp.AccessToken
			inst.TokenExpiresAt = expiresAt
			if tokenResp.RefreshToken != "" {
				inst.RefreshToken = tokenResp.RefreshToken
			}
			inst.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
			cfg.Instances[instanceName] = inst
			if saveErr := config.Save(cfg); saveErr != nil {
				return TokenRefreshResult{
					Success:   true,
					Token:     tokenResp.AccessToken,
					ExpiresAt: expiresAt,
					Error:     fmt.Sprintf("token refreshed but failed to save config: %v", saveErr),
				}
			}
		}
	}

	return TokenRefreshResult{Success: true, Token: tokenResp.AccessToken, ExpiresAt: expiresAt}
}

// GetValidToken returns a valid token, refreshing if necessary.
func GetValidToken(instanceName string, instance config.InstanceConfig) (token string, refreshed bool, err error) {
	if instance.AuthType != config.AuthTypeOAuth2 || instance.TokenExpiresAt == "" {
		return instance.Token, false, nil
	}

	if !IsTokenExpired(instance.TokenExpiresAt, 0) {
		return instance.Token, false, nil
	}

	result := RefreshAccessToken(instanceName, instance)
	if result.Success {
		return result.Token, true, nil
	}

	if result.RequiresReauth {
		return "", false, fmt.Errorf("session expired for instance %q. Please run: trokky login %s",
			instanceName, instance.URL)
	}

	return instance.Token, false, nil
}

// DeriveInstanceName derives a short name from a URL.
func DeriveInstanceName(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "default"
	}
	parts := strings.Split(parsed.Hostname(), ".")
	if len(parts) > 0 && parts[0] != "" {
		return parts[0]
	}
	return "default"
}
