package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	ConfigDir  = ".trokky"
	ConfigFile = "config.yaml"

	EnvURL      = "TROKKY_URL"
	EnvToken    = "TROKKY_TOKEN"
	EnvInstance = "TROKKY_INSTANCE"

	AuthTypeAPIToken = "api-token"
	AuthTypeOAuth2   = "oauth2"
)

type InstanceConfig struct {
	URL            string `yaml:"url"`
	Token          string `yaml:"token"`
	RefreshToken   string `yaml:"refreshToken,omitempty"`
	AuthType       string `yaml:"authType,omitempty"` // AuthTypeAPIToken or AuthTypeOAuth2
	TokenExpiresAt string `yaml:"tokenExpiresAt,omitempty"`
	Description    string `yaml:"description,omitempty"`
	AddedAt        string `yaml:"addedAt,omitempty"`
	UpdatedAt      string `yaml:"updatedAt,omitempty"`
}

type Config struct {
	Version   string                    `yaml:"version"`
	Default   string                    `yaml:"default,omitempty"`
	Instances map[string]InstanceConfig `yaml:"instances"`
}

// ResolvedCredentials holds credentials resolved from flags, env, or config.
type ResolvedCredentials struct {
	URL          string
	Token        string
	Source       string // "cli", "env", or "config"
	InstanceName string          // set when Source is "config"
	Instance     *InstanceConfig // set when Source is "config"
}

// ResolveOptions controls how credentials are resolved.
type ResolveOptions struct {
	URL      string
	Token    string
	Instance string
}

func newConfig() *Config {
	return &Config{
		Version:   "1.0",
		Instances: make(map[string]InstanceConfig),
	}
}

// ConfigPath returns the full path to the config file.
func ConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ConfigDir, ConfigFile)
}

// ConfigExists reports whether the config file exists on disk.
func ConfigExists() bool {
	_, err := os.Stat(ConfigPath())
	return err == nil
}

// Load reads the config from ~/.trokky/config.yaml.
// Returns an empty config if the file does not exist.
func Load() (*Config, error) {
	path := ConfigPath()
	if path == "" {
		return nil, fmt.Errorf("cannot determine home directory")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return newConfig(), nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return newConfig(), nil
	}

	if cfg.Version == "" {
		cfg.Version = "1.0"
	}
	if cfg.Instances == nil {
		cfg.Instances = make(map[string]InstanceConfig)
	}

	return &cfg, nil
}

// Save writes the config to ~/.trokky/config.yaml.
func Save(cfg *Config) error {
	path := ConfigPath()
	if path == "" {
		return fmt.Errorf("cannot determine home directory")
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

// AddInstance adds or updates a named instance in the config.
// If setAsDefault is true, or this is the first instance, it becomes the default.
func AddInstance(name string, inst InstanceConfig, setAsDefault bool) error {
	cfg, err := Load()
	if err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)

	existing, exists := cfg.Instances[name]
	if exists {
		inst.AddedAt = existing.AddedAt
	} else {
		inst.AddedAt = now
	}
	inst.UpdatedAt = now

	cfg.Instances[name] = inst

	if setAsDefault || len(cfg.Instances) == 1 {
		cfg.Default = name
	}

	return Save(cfg)
}

// RemoveInstance removes a named instance. Returns true if it existed.
// If the removed instance was the default, the default is reassigned.
func RemoveInstance(name string) (bool, error) {
	cfg, err := Load()
	if err != nil {
		return false, err
	}

	if _, ok := cfg.Instances[name]; !ok {
		return false, nil
	}

	delete(cfg.Instances, name)

	if cfg.Default == name {
		cfg.Default = ""
		for k := range cfg.Instances {
			cfg.Default = k
			break
		}
	}

	return true, Save(cfg)
}

// ListInstances returns all configured instances and the default name.
func ListInstances() (map[string]InstanceConfig, string, error) {
	cfg, err := Load()
	if err != nil {
		return nil, "", err
	}
	return cfg.Instances, cfg.Default, nil
}

// GetInstance returns a specific instance by name, or nil if not found.
func GetInstance(name string) (*InstanceConfig, error) {
	cfg, err := Load()
	if err != nil {
		return nil, err
	}
	inst, ok := cfg.Instances[name]
	if !ok {
		return nil, nil
	}
	return &inst, nil
}

// GetDefaultInstance returns the default instance name and config.
// Returns empty strings if no default is set.
func GetDefaultInstance() (string, *InstanceConfig, error) {
	cfg, err := Load()
	if err != nil {
		return "", nil, err
	}

	if cfg.Default == "" {
		return "", nil, nil
	}

	inst, ok := cfg.Instances[cfg.Default]
	if !ok {
		return "", nil, nil
	}

	return cfg.Default, &inst, nil
}

// SetDefaultInstance sets the default instance by name.
// Returns false if the instance does not exist.
func SetDefaultInstance(name string) (bool, error) {
	cfg, err := Load()
	if err != nil {
		return false, err
	}

	if _, ok := cfg.Instances[name]; !ok {
		return false, nil
	}

	cfg.Default = name
	return true, Save(cfg)
}

// NormalizeBaseURL trims trailing slashes and ensures the URL ends with /api.
func NormalizeBaseURL(rawURL string) string {
	u := strings.TrimRight(rawURL, "/")
	if !strings.HasSuffix(u, "/api") {
		u += "/api"
	}
	return u
}

// MaskToken masks a token for display, showing first 4 and last 4 characters.
func MaskToken(token string) string {
	if len(token) <= 12 {
		return "****"
	}
	return token[:4] + "..." + token[len(token)-4:]
}

// ResolveCredentials resolves credentials using a 3-tier priority:
// 1. CLI flags (--url + --token)
// 2. Environment variables (TROKKY_URL, TROKKY_TOKEN)
// 3. Config file (--instance flag, TROKKY_INSTANCE env, or default)
func ResolveCredentials(opts ResolveOptions) (*ResolvedCredentials, error) {
	// Priority 1: CLI flags
	if opts.URL != "" && opts.Token != "" {
		return &ResolvedCredentials{
			URL:    opts.URL,
			Token:  opts.Token,
			Source: "cli",
		}, nil
	}

	// Priority 2: Environment variables
	envURL := os.Getenv(EnvURL)
	envToken := os.Getenv(EnvToken)
	if envURL != "" && envToken != "" {
		return &ResolvedCredentials{
			URL:    envURL,
			Token:  envToken,
			Source: "env",
		}, nil
	}

	// Priority 3: Config file
	cfg, err := Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	envInstance := os.Getenv(EnvInstance)
	instanceName := opts.Instance
	if instanceName == "" {
		instanceName = envInstance
	}
	if instanceName == "" {
		instanceName = cfg.Default
	}

	if instanceName == "" {
		return nil, nil
	}

	inst, ok := cfg.Instances[instanceName]
	if !ok {
		return nil, nil
	}

	return &ResolvedCredentials{
		URL:          inst.URL,
		Token:        inst.Token,
		Source:       "config",
		InstanceName: instanceName,
		Instance:     &inst,
	}, nil
}

// RequireCredentials resolves credentials or returns a helpful error.
func RequireCredentials(opts ResolveOptions) (*ResolvedCredentials, error) {
	creds, err := ResolveCredentials(opts)
	if err != nil {
		return nil, err
	}
	if creds != nil {
		return creds, nil
	}

	msg := "No credentials found.\n\n" +
		"You can provide credentials in several ways:\n\n" +
		"1. CLI flags (highest priority):\n" +
		"   trokky <command> --url <url> --token <token>\n\n" +
		"2. Environment variables:\n" +
		"   export " + EnvURL + "=https://cms.example.com/api\n" +
		"   export " + EnvToken + "=your-api-token\n\n" +
		"3. Configure an instance (recommended):\n" +
		"   trokky config add <name>\n" +
		"   trokky config use <name>\n"

	if opts.Instance != "" {
		msg += fmt.Sprintf("\nNote: Instance %q was not found.\n"+
			"Run `trokky config list` to see available instances.\n", opts.Instance)
	}

	return nil, fmt.Errorf("%s", msg)
}
