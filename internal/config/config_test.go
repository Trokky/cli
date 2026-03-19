package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// overrideHome sets HOME to a temp dir so tests use an isolated config path.
// Returns a cleanup function that restores the original HOME.
func overrideHome(t *testing.T) string {
	t.Helper()
	tmpDir := t.TempDir()
	t.Setenv("HOME", tmpDir)
	return tmpDir
}

// clearEnvVars unsets all TROKKY_ env vars for test isolation.
func clearEnvVars(t *testing.T) {
	t.Helper()
	for _, key := range []string{EnvURL, EnvToken, EnvInstance} {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}
}

// --- Config Path ---

func TestConfigPath(t *testing.T) {
	overrideHome(t)
	path := ConfigPath()
	if !filepath.IsAbs(path) {
		t.Fatalf("expected absolute path, got %q", path)
	}
	if filepath.Base(path) != ConfigFile {
		t.Fatalf("expected filename %q, got %q", ConfigFile, filepath.Base(path))
	}
}

func TestConfigExists_False(t *testing.T) {
	overrideHome(t)
	if ConfigExists() {
		t.Fatal("expected config not to exist in fresh dir")
	}
}

func TestConfigExists_True(t *testing.T) {
	overrideHome(t)
	cfg := newConfig()
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	if !ConfigExists() {
		t.Fatal("expected config to exist after Save")
	}
}

// --- Load / Save ---

func TestLoad_NoFile(t *testing.T) {
	overrideHome(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if cfg.Version != "1.0" {
		t.Fatalf("expected version '1.0', got %q", cfg.Version)
	}
	if len(cfg.Instances) != 0 {
		t.Fatalf("expected empty instances, got %d", len(cfg.Instances))
	}
}

func TestSaveAndLoad(t *testing.T) {
	overrideHome(t)

	cfg := newConfig()
	cfg.Default = "prod"
	cfg.Instances["prod"] = InstanceConfig{
		URL:      "https://cms.example.com/api",
		Token:    "tok-123",
		AuthType: "api-token",
	}

	if err := Save(cfg); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if loaded.Version != "1.0" {
		t.Fatalf("Version = %q, want '1.0'", loaded.Version)
	}
	if loaded.Default != "prod" {
		t.Fatalf("Default = %q, want 'prod'", loaded.Default)
	}
	inst, ok := loaded.Instances["prod"]
	if !ok {
		t.Fatal("instance 'prod' not found after load")
	}
	if inst.URL != "https://cms.example.com/api" {
		t.Fatalf("URL = %q", inst.URL)
	}
	if inst.Token != "tok-123" {
		t.Fatalf("Token = %q", inst.Token)
	}
	if inst.AuthType != "api-token" {
		t.Fatalf("AuthType = %q", inst.AuthType)
	}
}

func TestSave_FilePermissions(t *testing.T) {
	overrideHome(t)
	if err := Save(newConfig()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(ConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Fatalf("expected permissions 0600, got %o", perm)
	}
}

func TestSave_CreatesDirectory(t *testing.T) {
	overrideHome(t)
	// ConfigPath dir doesn't exist yet
	if err := Save(newConfig()); err != nil {
		t.Fatalf("Save() should create directory: %v", err)
	}
	if _, err := os.Stat(ConfigPath()); err != nil {
		t.Fatal("config file not created")
	}
}

func TestSave_WritesYAML(t *testing.T) {
	overrideHome(t)
	cfg := newConfig()
	cfg.Instances["test"] = InstanceConfig{URL: "http://localhost", Token: "t"}
	Save(cfg)

	data, _ := os.ReadFile(ConfigPath())
	var parsed map[string]interface{}
	if err := yaml.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("saved file is not valid YAML: %v", err)
	}
	if _, ok := parsed["version"]; !ok {
		t.Fatal("expected 'version' key in YAML")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	overrideHome(t)
	path := ConfigPath()
	os.MkdirAll(filepath.Dir(path), 0700)
	os.WriteFile(path, []byte("not: [valid: yaml: {{"), 0600)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() should not error on invalid YAML, got: %v", err)
	}
	if cfg.Version != "1.0" {
		t.Fatal("expected empty config fallback")
	}
}

func TestLoad_MissingVersion(t *testing.T) {
	overrideHome(t)
	path := ConfigPath()
	os.MkdirAll(filepath.Dir(path), 0700)
	os.WriteFile(path, []byte("instances: {}"), 0600)

	cfg, _ := Load()
	if cfg.Version != "1.0" {
		t.Fatalf("expected version backfilled to '1.0', got %q", cfg.Version)
	}
}

func TestLoad_NullInstances(t *testing.T) {
	overrideHome(t)
	path := ConfigPath()
	os.MkdirAll(filepath.Dir(path), 0700)
	os.WriteFile(path, []byte("version: '1.0'\ninstances:"), 0600)

	cfg, _ := Load()
	if cfg.Instances == nil {
		t.Fatal("expected Instances to be initialized, got nil")
	}
}

// --- AddInstance ---

func TestAddInstance_New(t *testing.T) {
	overrideHome(t)

	err := AddInstance("staging", InstanceConfig{
		URL:   "https://staging.example.com/api",
		Token: "st-tok",
	}, false)
	if err != nil {
		t.Fatalf("AddInstance() error: %v", err)
	}

	cfg, _ := Load()
	inst, ok := cfg.Instances["staging"]
	if !ok {
		t.Fatal("instance not found after add")
	}
	if inst.URL != "https://staging.example.com/api" {
		t.Fatalf("URL = %q", inst.URL)
	}
	if inst.AddedAt == "" {
		t.Fatal("expected AddedAt to be set")
	}
	if inst.UpdatedAt == "" {
		t.Fatal("expected UpdatedAt to be set")
	}
}

func TestAddInstance_FirstBecomesDefault(t *testing.T) {
	overrideHome(t)

	AddInstance("first", InstanceConfig{URL: "http://a", Token: "t"}, false)

	cfg, _ := Load()
	if cfg.Default != "first" {
		t.Fatalf("first instance should become default, got %q", cfg.Default)
	}
}

func TestAddInstance_SetAsDefault(t *testing.T) {
	overrideHome(t)

	AddInstance("first", InstanceConfig{URL: "http://a", Token: "t"}, false)
	AddInstance("second", InstanceConfig{URL: "http://b", Token: "t"}, true)

	cfg, _ := Load()
	if cfg.Default != "second" {
		t.Fatalf("Default = %q, want 'second'", cfg.Default)
	}
}

func TestAddInstance_NotSetAsDefault(t *testing.T) {
	overrideHome(t)

	AddInstance("first", InstanceConfig{URL: "http://a", Token: "t"}, false)
	AddInstance("second", InstanceConfig{URL: "http://b", Token: "t"}, false)

	cfg, _ := Load()
	if cfg.Default != "first" {
		t.Fatalf("Default should stay 'first', got %q", cfg.Default)
	}
}

func TestAddInstance_UpdatePreservesAddedAt(t *testing.T) {
	overrideHome(t)

	AddInstance("x", InstanceConfig{URL: "http://a", Token: "old"}, false)
	cfg1, _ := Load()
	originalAddedAt := cfg1.Instances["x"].AddedAt

	AddInstance("x", InstanceConfig{URL: "http://a", Token: "new"}, false)
	cfg2, _ := Load()

	if cfg2.Instances["x"].AddedAt != originalAddedAt {
		t.Fatal("AddedAt should be preserved on update")
	}
	if cfg2.Instances["x"].Token != "new" {
		t.Fatal("Token should be updated")
	}
}

// --- RemoveInstance ---

func TestRemoveInstance_Exists(t *testing.T) {
	overrideHome(t)
	AddInstance("rm-me", InstanceConfig{URL: "http://a", Token: "t"}, false)

	removed, err := RemoveInstance("rm-me")
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("expected removed=true")
	}

	cfg, _ := Load()
	if _, ok := cfg.Instances["rm-me"]; ok {
		t.Fatal("instance should be gone after remove")
	}
}

func TestRemoveInstance_NotFound(t *testing.T) {
	overrideHome(t)

	removed, err := RemoveInstance("nope")
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Fatal("expected removed=false for nonexistent instance")
	}
}

func TestRemoveInstance_ReassignsDefault(t *testing.T) {
	overrideHome(t)

	AddInstance("a", InstanceConfig{URL: "http://a", Token: "t"}, true)
	AddInstance("b", InstanceConfig{URL: "http://b", Token: "t"}, false)

	RemoveInstance("a")

	cfg, _ := Load()
	if cfg.Default == "a" {
		t.Fatal("default should not be the removed instance")
	}
	if cfg.Default != "b" {
		t.Fatalf("default should be reassigned to remaining instance, got %q", cfg.Default)
	}
}

func TestRemoveInstance_ClearsDefaultWhenEmpty(t *testing.T) {
	overrideHome(t)
	AddInstance("only", InstanceConfig{URL: "http://a", Token: "t"}, true)

	RemoveInstance("only")

	cfg, _ := Load()
	if cfg.Default != "" {
		t.Fatalf("default should be empty when no instances remain, got %q", cfg.Default)
	}
}

// --- ListInstances ---

func TestListInstances_Empty(t *testing.T) {
	overrideHome(t)

	instances, defaultName, err := ListInstances()
	if err != nil {
		t.Fatal(err)
	}
	if len(instances) != 0 {
		t.Fatalf("expected 0 instances, got %d", len(instances))
	}
	if defaultName != "" {
		t.Fatalf("expected empty default, got %q", defaultName)
	}
}

func TestListInstances_Multiple(t *testing.T) {
	overrideHome(t)
	AddInstance("a", InstanceConfig{URL: "http://a", Token: "t"}, true)
	AddInstance("b", InstanceConfig{URL: "http://b", Token: "t"}, false)

	instances, defaultName, _ := ListInstances()
	if len(instances) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(instances))
	}
	if defaultName != "a" {
		t.Fatalf("default = %q, want 'a'", defaultName)
	}
}

// --- GetInstance ---

func TestGetInstance_Found(t *testing.T) {
	overrideHome(t)
	AddInstance("prod", InstanceConfig{URL: "http://prod", Token: "t"}, false)

	inst, err := GetInstance("prod")
	if err != nil {
		t.Fatal(err)
	}
	if inst == nil {
		t.Fatal("expected instance, got nil")
	}
	if inst.URL != "http://prod" {
		t.Fatalf("URL = %q", inst.URL)
	}
}

func TestGetInstance_NotFound(t *testing.T) {
	overrideHome(t)

	inst, err := GetInstance("nope")
	if err != nil {
		t.Fatal(err)
	}
	if inst != nil {
		t.Fatal("expected nil for nonexistent instance")
	}
}

// --- GetDefaultInstance ---

func TestGetDefaultInstance_Set(t *testing.T) {
	overrideHome(t)
	AddInstance("prod", InstanceConfig{URL: "http://prod", Token: "t"}, true)

	name, inst, err := GetDefaultInstance()
	if err != nil {
		t.Fatal(err)
	}
	if name != "prod" {
		t.Fatalf("name = %q, want 'prod'", name)
	}
	if inst == nil {
		t.Fatal("expected instance, got nil")
	}
}

func TestGetDefaultInstance_NotSet(t *testing.T) {
	overrideHome(t)

	name, inst, err := GetDefaultInstance()
	if err != nil {
		t.Fatal(err)
	}
	if name != "" {
		t.Fatalf("expected empty name, got %q", name)
	}
	if inst != nil {
		t.Fatal("expected nil instance")
	}
}

// --- SetDefaultInstance ---

func TestSetDefaultInstance_Success(t *testing.T) {
	overrideHome(t)
	AddInstance("a", InstanceConfig{URL: "http://a", Token: "t"}, true)
	AddInstance("b", InstanceConfig{URL: "http://b", Token: "t"}, false)

	ok, err := SetDefaultInstance("b")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected true")
	}

	cfg, _ := Load()
	if cfg.Default != "b" {
		t.Fatalf("Default = %q, want 'b'", cfg.Default)
	}
}

func TestSetDefaultInstance_NotFound(t *testing.T) {
	overrideHome(t)

	ok, err := SetDefaultInstance("nope")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected false for nonexistent instance")
	}
}

// --- MaskToken ---

func TestMaskToken_Long(t *testing.T) {
	result := MaskToken("abcdefghijklmnop")
	if result != "abcd...mnop" {
		t.Fatalf("MaskToken = %q, want 'abcd...mnop'", result)
	}
}

func TestMaskToken_Short(t *testing.T) {
	result := MaskToken("short")
	if result != "****" {
		t.Fatalf("MaskToken = %q, want '****'", result)
	}
}

func TestMaskToken_ExactlyThreshold(t *testing.T) {
	// 12 chars = at threshold, should mask
	result := MaskToken("123456789012")
	if result != "****" {
		t.Fatalf("MaskToken = %q, want '****'", result)
	}
}

func TestMaskToken_JustAboveThreshold(t *testing.T) {
	// 13 chars = above threshold
	result := MaskToken("1234567890123")
	if result != "1234...0123" {
		t.Fatalf("MaskToken = %q, want '1234...0123'", result)
	}
}

// --- ResolveCredentials ---

func TestResolveCredentials_CLIFlags(t *testing.T) {
	clearEnvVars(t)

	creds, err := ResolveCredentials(ResolveOptions{
		URL:   "http://from-flag",
		Token: "flag-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	if creds == nil {
		t.Fatal("expected credentials")
	}
	if creds.Source != "cli" {
		t.Fatalf("Source = %q, want 'cli'", creds.Source)
	}
	if creds.URL != "http://from-flag" {
		t.Fatalf("URL = %q", creds.URL)
	}
	if creds.Token != "flag-token" {
		t.Fatalf("Token = %q", creds.Token)
	}
}

func TestResolveCredentials_CLIFlagsPartial(t *testing.T) {
	overrideHome(t)
	clearEnvVars(t)

	// Only URL, no token — should not resolve from CLI
	creds, _ := ResolveCredentials(ResolveOptions{URL: "http://only-url"})
	if creds != nil {
		t.Fatalf("expected nil when only URL provided, got Source=%q", creds.Source)
	}
}

func TestResolveCredentials_EnvVars(t *testing.T) {
	overrideHome(t)
	clearEnvVars(t)
	os.Setenv(EnvURL, "http://from-env")
	os.Setenv(EnvToken, "env-token")

	creds, err := ResolveCredentials(ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if creds == nil {
		t.Fatal("expected credentials from env")
	}
	if creds.Source != "env" {
		t.Fatalf("Source = %q, want 'env'", creds.Source)
	}
	if creds.URL != "http://from-env" {
		t.Fatalf("URL = %q", creds.URL)
	}
}

func TestResolveCredentials_CLIOverridesEnv(t *testing.T) {
	clearEnvVars(t)
	os.Setenv(EnvURL, "http://env")
	os.Setenv(EnvToken, "env-tok")

	creds, _ := ResolveCredentials(ResolveOptions{
		URL:   "http://cli",
		Token: "cli-tok",
	})
	if creds.Source != "cli" {
		t.Fatalf("CLI flags should override env, got Source=%q", creds.Source)
	}
}

func TestResolveCredentials_Config(t *testing.T) {
	overrideHome(t)
	clearEnvVars(t)
	AddInstance("prod", InstanceConfig{URL: "http://prod", Token: "prod-tok"}, true)

	creds, err := ResolveCredentials(ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if creds == nil {
		t.Fatal("expected credentials from config")
	}
	if creds.Source != "config" {
		t.Fatalf("Source = %q, want 'config'", creds.Source)
	}
	if creds.InstanceName != "prod" {
		t.Fatalf("InstanceName = %q, want 'prod'", creds.InstanceName)
	}
	if creds.URL != "http://prod" {
		t.Fatalf("URL = %q", creds.URL)
	}
}

func TestResolveCredentials_InstanceFlag(t *testing.T) {
	overrideHome(t)
	clearEnvVars(t)
	AddInstance("prod", InstanceConfig{URL: "http://prod", Token: "p"}, true)
	AddInstance("staging", InstanceConfig{URL: "http://staging", Token: "s"}, false)

	creds, _ := ResolveCredentials(ResolveOptions{Instance: "staging"})
	if creds.InstanceName != "staging" {
		t.Fatalf("InstanceName = %q, want 'staging'", creds.InstanceName)
	}
	if creds.URL != "http://staging" {
		t.Fatalf("URL = %q, want 'http://staging'", creds.URL)
	}
}

func TestResolveCredentials_InstanceEnvVar(t *testing.T) {
	overrideHome(t)
	clearEnvVars(t)
	AddInstance("prod", InstanceConfig{URL: "http://prod", Token: "p"}, true)
	AddInstance("dev", InstanceConfig{URL: "http://dev", Token: "d"}, false)
	os.Setenv(EnvInstance, "dev")

	creds, _ := ResolveCredentials(ResolveOptions{})
	if creds.InstanceName != "dev" {
		t.Fatalf("InstanceName = %q, want 'dev'", creds.InstanceName)
	}
}

func TestResolveCredentials_InstanceFlagOverridesEnv(t *testing.T) {
	overrideHome(t)
	clearEnvVars(t)
	AddInstance("a", InstanceConfig{URL: "http://a", Token: "a"}, true)
	AddInstance("b", InstanceConfig{URL: "http://b", Token: "b"}, false)
	os.Setenv(EnvInstance, "a")

	creds, _ := ResolveCredentials(ResolveOptions{Instance: "b"})
	if creds.InstanceName != "b" {
		t.Fatalf("--instance flag should override TROKKY_INSTANCE env, got %q", creds.InstanceName)
	}
}

func TestResolveCredentials_NoCredentials(t *testing.T) {
	overrideHome(t)
	clearEnvVars(t)

	creds, err := ResolveCredentials(ResolveOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if creds != nil {
		t.Fatalf("expected nil when no credentials available, got Source=%q", creds.Source)
	}
}

func TestResolveCredentials_InstanceNotFound(t *testing.T) {
	overrideHome(t)
	clearEnvVars(t)

	creds, _ := ResolveCredentials(ResolveOptions{Instance: "nonexistent"})
	if creds != nil {
		t.Fatal("expected nil for nonexistent instance")
	}
}

// --- RequireCredentials ---

func TestRequireCredentials_Success(t *testing.T) {
	clearEnvVars(t)

	creds, err := RequireCredentials(ResolveOptions{
		URL:   "http://ok",
		Token: "tok",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.URL != "http://ok" {
		t.Fatalf("URL = %q", creds.URL)
	}
}

func TestRequireCredentials_NoCredentials(t *testing.T) {
	overrideHome(t)
	clearEnvVars(t)

	_, err := RequireCredentials(ResolveOptions{})
	if err == nil {
		t.Fatal("expected error when no credentials")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "No credentials found") {
		t.Fatalf("expected helpful error message, got: %q", errMsg)
	}
	if !strings.Contains(errMsg, EnvURL) {
		t.Fatal("error should mention TROKKY_URL env var")
	}
	if !strings.Contains(errMsg, "trokky config add") {
		t.Fatal("error should mention trokky config add")
	}
}

func TestRequireCredentials_InstanceNotFound(t *testing.T) {
	overrideHome(t)
	clearEnvVars(t)

	_, err := RequireCredentials(ResolveOptions{Instance: "ghost"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Fatal("error should mention the missing instance name")
	}
}

// --- YAML roundtrip for all InstanceConfig fields ---

func TestInstanceConfig_YAMLRoundtrip(t *testing.T) {
	overrideHome(t)

	AddInstance("full", InstanceConfig{
		URL:            "https://cms.example.com/api",
		Token:          "access-token",
		RefreshToken:   "refresh-token",
		AuthType:       "oauth2",
		TokenExpiresAt: "2026-03-18T12:00:00Z",
		Description:    "Production instance",
	}, true)

	inst, _ := GetInstance("full")
	if inst == nil {
		t.Fatal("instance not found")
	}
	if inst.URL != "https://cms.example.com/api" {
		t.Fatalf("URL = %q", inst.URL)
	}
	if inst.RefreshToken != "refresh-token" {
		t.Fatalf("RefreshToken = %q", inst.RefreshToken)
	}
	if inst.AuthType != "oauth2" {
		t.Fatalf("AuthType = %q", inst.AuthType)
	}
	if inst.TokenExpiresAt != "2026-03-18T12:00:00Z" {
		t.Fatalf("TokenExpiresAt = %q", inst.TokenExpiresAt)
	}
	if inst.Description != "Production instance" {
		t.Fatalf("Description = %q", inst.Description)
	}
	if inst.AddedAt == "" {
		t.Fatal("AddedAt should be set")
	}
	if inst.UpdatedAt == "" {
		t.Fatal("UpdatedAt should be set")
	}
}
