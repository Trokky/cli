package scaffold

import (
	"fmt"
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
		{filepath.Join(targetDir, ".npmrc"), GenerateNpmrc()},
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

	return nil
}
