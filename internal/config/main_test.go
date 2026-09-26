package config

import (
	"os"
	"testing"
)

// Every test in this package runs against a throwaway HOME. RefreshAccessToken reads and
// writes ~/.trokky/config.yaml under a lock, and a test that forgot overrideHome once
// rewrote a developer's real config; this makes forgetting harmless.
func TestMain(m *testing.M) {
	home, err := os.MkdirTemp("", "trokky-config-test-home-")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", home)
	os.Setenv("USERPROFILE", home)
	code := m.Run()
	os.RemoveAll(home)
	os.Exit(code)
}
