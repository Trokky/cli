package scaffold

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Scaffold creates a new Trokky project in the given directory.
func Scaffold(cfg ProjectConfig, targetDir string) error {
	// Create directory structure
	dirs := []string{
		targetDir,
		filepath.Join(targetDir, "schemas"),
	}

	if cfg.DataAdapter == DataFilesystem {
		dirs = append(dirs, filepath.Join(targetDir, "data", "content"))
		dirs = append(dirs, filepath.Join(targetDir, "data", "users"))
	}
	if cfg.MediaAdapter == MediaFilesystem {
		dirs = append(dirs, filepath.Join(targetDir, "data", "media"))
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	// Files to generate
	type fileEntry struct {
		path    string
		content string
	}

	files := []fileEntry{
		{filepath.Join(targetDir, "package.json"), GeneratePackageJSON(cfg)},
		{filepath.Join(targetDir, "server.ts"), GenerateServerTS(cfg)},
		{filepath.Join(targetDir, "trokky.config.ts"), GenerateTrokkyConfig(cfg)},
		{filepath.Join(targetDir, "tsconfig.json"), GenerateTsConfig()},
		{filepath.Join(targetDir, "nodemon.json"), GenerateNodemonConfig()},
		{filepath.Join(targetDir, ".env.example"), GenerateEnvExample(cfg)},
		{filepath.Join(targetDir, ".gitignore"), GenerateGitignore()},
		{filepath.Join(targetDir, "schemas", "index.ts"), GenerateSchemaIndex(cfg)},
	}

	// Structure file (for Studio sidebar)
	if cfg.Studio != StudioNone {
		files = append(files, fileEntry{filepath.Join(targetDir, "structure.ts"), GenerateStructureTS(cfg)})
	}

	// Example schemas
	if cfg.IncludeExamples {
		files = append(files, fileEntry{filepath.Join(targetDir, "schemas", "article.ts"), GenerateExampleArticleSchema()})
		files = append(files, fileEntry{filepath.Join(targetDir, "schemas", "page.ts"), GenerateExamplePageSchema()})
	}

	for _, f := range files {
		if err := os.WriteFile(f.path, []byte(f.content), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", f.path, err)
		}
	}

	// A .env with generated secrets (gitignored), unless one is already there
	envPath := filepath.Join(targetDir, ".env")
	if _, err := os.Stat(envPath); errors.Is(err, fs.ErrNotExist) {
		if err := os.WriteFile(envPath, []byte(GenerateEnv(cfg, newSecret)), 0600); err != nil {
			return fmt.Errorf("failed to write %s: %w", envPath, err)
		}
	}

	return nil
}

// newSecret is 48 random bytes, base64: what `openssl rand -base64 48` prints
func newSecret() string {
	b := make([]byte, 48)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("no randomness for a secret: %v", err))
	}
	return base64.StdEncoding.EncodeToString(b)
}
