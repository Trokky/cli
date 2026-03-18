package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type InstanceConfig struct {
	Token string `json:"token"`
}

type Config struct {
	ActiveInstance string                    `json:"activeInstance"`
	Instances      map[string]InstanceConfig `json:"instances"`
	path           string
}

func New() *Config {
	return &Config{
		Instances: make(map[string]InstanceConfig),
	}
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".trokky", "config.json"), nil
}

func Load() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("no config found (run `trokky login` first)")
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid config file: %w", err)
	}

	if cfg.Instances == nil {
		cfg.Instances = make(map[string]InstanceConfig)
	}

	cfg.path = path
	return &cfg, nil
}

func (c *Config) Save() error {
	path := c.path
	if path == "" {
		var err error
		path, err = configPath()
		if err != nil {
			return err
		}
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, data, 0600)
}

func (c *Config) SetInstance(url, token string) {
	c.Instances[url] = InstanceConfig{Token: token}
}

func (c *Config) GetActiveToken() (string, string, error) {
	if c.ActiveInstance == "" {
		return "", "", fmt.Errorf("no active instance (run `trokky login` first)")
	}

	inst, ok := c.Instances[c.ActiveInstance]
	if !ok {
		return "", "", fmt.Errorf("no credentials for %s", c.ActiveInstance)
	}

	return c.ActiveInstance, inst.Token, nil
}
