package scaffold

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// legacyMarkers are v1 package references that must never appear in a v2 scaffold.
var legacyMarkers = []string{"@trokky/core", "@trokky/express", "@trokky/adapter-"}

func testConfigs() map[string]ProjectConfig {
	cases := map[string]ProjectConfig{}

	for tmpl, cfg := range TemplateDefaults {
		cfg.Name = "tmpl-" + string(tmpl)
		cases["template-"+string(tmpl)] = cfg
	}

	base := TemplateDefaults[TemplateMinimal]
	base.Name = "combo-project"

	postgres := base
	postgres.DataAdapter = DataPostgres
	cases["data-postgres"] = postgres

	mailResend := base
	mailResend.Mail = MailResend
	cases["mail-resend"] = mailResend

	mailConsole := base
	mailConsole.Mail = MailConsole
	cases["mail-console"] = mailConsole

	studioSeparate := base
	studioSeparate.Studio = StudioSeparate
	cases["studio-separate"] = studioSeparate

	studioNone := base
	studioNone.Studio = StudioNone
	cases["studio-none"] = studioNone

	examples := base
	examples.IncludeExamples = true
	cases["examples-on"] = examples

	return cases
}

// generated returns every file the scaffold produces for cfg, keyed by a label,
// combining the direct Generate* calls with the files Scaffold writes to disk.
func generated(t *testing.T, cfg ProjectConfig) map[string]string {
	t.Helper()

	files := map[string]string{
		"gen:package.json":     GeneratePackageJSON(cfg),
		"gen:server.ts":        GenerateServerTS(cfg),
		"gen:trokky.config.ts": GenerateTrokkyConfig(cfg),
		"gen:tsconfig.json":    GenerateTsConfig(),
		"gen:nodemon.json":     GenerateNodemonConfig(),
		"gen:.env.example":     GenerateEnvExample(cfg),
		"gen:.gitignore":       GenerateGitignore(),
		"gen:structure.ts":     GenerateStructureTS(cfg),
		"gen:schemas/index.ts": GenerateSchemaIndex(cfg),
		"gen:schemas/article":  GenerateExampleArticleSchema(),
		"gen:schemas/page":     GenerateExamplePageSchema(),
	}

	dir := t.TempDir()
	if err := Scaffold(cfg, dir); err != nil {
		t.Fatalf("Scaffold: %v", err)
	}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files["scaffold:"+filepath.ToSlash(rel)] = string(data)
		return nil
	})
	if err != nil {
		t.Fatalf("walking scaffolded dir: %v", err)
	}

	return files
}

func TestScaffoldTemplates(t *testing.T) {
	for name, cfg := range testConfigs() {
		cfg := cfg
		t.Run(name, func(t *testing.T) {
			if err := cfg.Validate(); err != nil {
				t.Fatalf("Validate: %v", err)
			}

			files := generated(t, cfg)

			// (a) no legacy v1 packages anywhere
			for label, content := range files {
				for _, marker := range legacyMarkers {
					if strings.Contains(content, marker) {
						t.Errorf("%s contains legacy reference %q", label, marker)
					}
				}
			}

			// (b) package.json pins the v2 packages
			pkg := files["scaffold:package.json"]
			if !strings.Contains(pkg, `"trokky": "^2.0.0"`) {
				t.Errorf("package.json missing trokky ^2.0.0:\n%s", pkg)
			}
			if !strings.Contains(pkg, "@trokky/client") {
				t.Errorf("package.json missing @trokky/client:\n%s", pkg)
			}
			if cfg.DataAdapter == DataPostgres && !strings.Contains(pkg, `"pg"`) {
				t.Errorf("postgres package.json missing pg:\n%s", pkg)
			}
			if cfg.Studio != StudioNone && !strings.Contains(pkg, "@trokky/studio") {
				t.Errorf("package.json missing @trokky/studio:\n%s", pkg)
			}

			// (c) server.ts wiring
			server := files["scaffold:server.ts"]
			for _, want := range []string{
				"from 'trokky/express'",
				"import 'trokky/adapters/filesystem-media'",
				"TrokkyExpress.create(",
				"trokky.mount(app",
				"getMountedPaths()",
			} {
				if !strings.Contains(server, want) {
					t.Errorf("server.ts missing %q:\n%s", want, server)
				}
			}

			wantData := "import 'trokky/adapters/filesystem-data'"
			unwantData := "import 'trokky/adapters/postgres-data'"
			if cfg.DataAdapter == DataPostgres {
				wantData, unwantData = unwantData, wantData
			}
			if !strings.Contains(server, wantData) {
				t.Errorf("server.ts missing data adapter import %q:\n%s", wantData, server)
			}
			if strings.Contains(server, unwantData) {
				t.Errorf("server.ts has wrong data adapter import %q:\n%s", unwantData, server)
			}

			if cfg.Studio == StudioNone && strings.Contains(server, "studioPath:") {
				t.Errorf("server.ts mounts studio for studio=none:\n%s", server)
			}
			if cfg.Studio != StudioNone && !strings.Contains(server, "studioPath: '/studio'") {
				t.Errorf("server.ts missing studio mount:\n%s", server)
			}

			// trokky.config.ts uses the v2 type import
			cfgFile := files["scaffold:trokky.config.ts"]
			if !strings.Contains(cfgFile, "import type { ContentSchema } from 'trokky'") {
				t.Errorf("trokky.config.ts missing ContentSchema import:\n%s", cfgFile)
			}
			if !strings.Contains(cfgFile, "as ContentSchema[]") {
				t.Errorf("trokky.config.ts schemas not typed as ContentSchema[]:\n%s", cfgFile)
			}
			if cfg.Mail != MailNone && !strings.Contains(cfgFile, "TROKKY_MAIL_ENABLED") {
				t.Errorf("trokky.config.ts missing mail config:\n%s", cfgFile)
			}

			// (d) .npmrc is not part of a v2 scaffold
			if _, ok := files["scaffold:.npmrc"]; ok {
				t.Error(".npmrc was written to the scaffolded project")
			}

			// No d1/r2/s3 leftovers in generated env/config
			for _, label := range []string{"scaffold:.env.example", "scaffold:trokky.config.ts"} {
				for _, marker := range []string{"cloudflare-d1", "cloudflare-r2", "R2_BUCKET_NAME", "S3_BUCKET", "D1_DATABASE_NAME"} {
					if strings.Contains(files[label], marker) {
						t.Errorf("%s still references %q", label, marker)
					}
				}
			}
		})
	}
}

func TestGeneratePackageJSONIsValidJSON(t *testing.T) {
	for name, cfg := range testConfigs() {
		cfg := cfg
		t.Run(name, func(t *testing.T) {
			var out struct {
				Name         string            `json:"name"`
				Dependencies map[string]string `json:"dependencies"`
			}
			if err := json.Unmarshal([]byte(GeneratePackageJSON(cfg)), &out); err != nil {
				t.Fatalf("package.json is not valid JSON: %v", err)
			}
			if out.Name != cfg.Name {
				t.Errorf("package.json name = %q, want %q", out.Name, cfg.Name)
			}
			if out.Dependencies["trokky"] != "^2.0.0" {
				t.Errorf("dependencies.trokky = %q, want ^2.0.0", out.Dependencies["trokky"])
			}
			if out.Dependencies["@trokky/client"] != "^2.0.0" {
				t.Errorf("dependencies[@trokky/client] = %q, want ^2.0.0", out.Dependencies["@trokky/client"])
			}
		})
	}
}

func TestValidateRejectsRemovedAdapters(t *testing.T) {
	base := TemplateDefaults[TemplateMinimal]
	base.Name = "invalid-project"

	for _, v := range []DataAdapter{"d1", "r2", "s3"} {
		cfg := base
		cfg.DataAdapter = v
		if err := cfg.Validate(); err == nil {
			t.Errorf("Validate accepted removed data adapter %q", v)
		}
	}

	for _, v := range []MediaAdapter{"d1", "r2", "s3"} {
		cfg := base
		cfg.MediaAdapter = v
		if err := cfg.Validate(); err == nil {
			t.Errorf("Validate accepted removed media adapter %q", v)
		}
	}
}
