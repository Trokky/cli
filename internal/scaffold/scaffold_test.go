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
			if !strings.Contains(pkg, `"@trokky/trokky": "`+TrokkyVersion+`"`) {
				t.Errorf("package.json missing trokky %s:\n%s", TrokkyVersion, pkg)
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
				"from '@trokky/trokky/express'",
				"import '@trokky/trokky/adapters/filesystem-media'",
				"TrokkyExpress.create(",
				"trokky.mount(app",
				"getMountedPaths()",
			} {
				if !strings.Contains(server, want) {
					t.Errorf("server.ts missing %q:\n%s", want, server)
				}
			}

			wantData := "import '@trokky/trokky/adapters/filesystem-data'"
			unwantData := "import '@trokky/trokky/adapters/postgres-data'"
			if cfg.DataAdapter == DataPostgres {
				wantData, unwantData = unwantData, wantData
			}
			if !strings.Contains(server, wantData) {
				t.Errorf("server.ts missing data adapter import %q:\n%s", wantData, server)
			}
			if strings.Contains(server, unwantData) {
				t.Errorf("server.ts has wrong data adapter import %q:\n%s", unwantData, server)
			}

			// Since 3.0 the site mounts Studio itself; the server never takes a studioPath.
			if strings.Contains(server, "studioPath:") || strings.Contains(server, "studio.enabled") {
				t.Errorf("server.ts uses the pre-3.0 studio mounting:\n%s", server)
			}
			hasRouter := strings.Contains(server, "from '@trokky/studio/express'") && strings.Contains(server, "app.use(studioPath, studioRouter(")
			if cfg.Studio == StudioEmbedded && !hasRouter {
				t.Errorf("server.ts does not mount studioRouter for an embedded studio:\n%s", server)
			}
			if cfg.Studio != StudioEmbedded && hasRouter {
				t.Errorf("server.ts mounts studioRouter for studio=%s:\n%s", cfg.Studio, server)
			}
			if cfg.Studio != StudioNone && !strings.Contains(files["scaffold:trokky.config.ts"], "url: process.env.STUDIO_URL") {
				t.Errorf("trokky.config.ts studio block lacks url: process.env.STUDIO_URL:\n%s", files["scaffold:trokky.config.ts"])
			}
			if cfg.Studio != StudioNone && !strings.Contains(server, "structure: trokkyConfig.structure") {
				t.Errorf("server.ts does not forward the top-level structure:\n%s", server)
			}
			// Only the studio block itself may not carry the removed keys; other
			// sections legitimately use "enabled:".
			if cfgText := files["scaffold:trokky.config.ts"]; strings.Contains(cfgText, "studio: {") {
				block := cfgText[strings.Index(cfgText, "studio: {"):]
				block = block[:strings.Index(block, "},")+2]
				for _, old := range []string{"enabled:", "path:", "requireAuth:", "apiUrl:", "structure,"} {
					if strings.Contains(block, old) {
						t.Errorf("trokky.config.ts studio block still has pre-3.0 key %q:\n%s", old, block)
					}
				}
			}

			// trokky.config.ts uses the v2 type import
			cfgFile := files["scaffold:trokky.config.ts"]
			if !strings.Contains(cfgFile, "import type { ContentSchema } from '@trokky/trokky'") {
				t.Errorf("trokky.config.ts missing ContentSchema import:\n%s", cfgFile)
			}
			if !strings.Contains(cfgFile, "as ContentSchema[]") {
				t.Errorf("trokky.config.ts schemas not typed as ContentSchema[]:\n%s", cfgFile)
			}
			if cfg.Mail != MailNone && !strings.Contains(cfgFile, "TROKKY_MAIL_ENABLED") {
				t.Errorf("trokky.config.ts missing mail config:\n%s", cfgFile)
			}

			// (d) no .npmrc: the packages are on the public npm registry, so a
			// generated project must install with no registry config and no token
			if npmrc, ok := files["scaffold:.npmrc"]; ok {
				t.Errorf("scaffold wrote an .npmrc; a generated project needs no registry config:\n%s", npmrc)
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
			if out.Dependencies["@trokky/trokky"] != TrokkyVersion {
				t.Errorf("dependencies.@trokky/trokky = %q, want %s", out.Dependencies["@trokky/trokky"], TrokkyVersion)
			}
			if out.Dependencies["@trokky/client"] != TrokkyVersion {
				t.Errorf("dependencies[@trokky/client] = %q, want %s", out.Dependencies["@trokky/client"], TrokkyVersion)
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

// A scaffolded project must not carry a secret anyone who has read this code knows: a fallback
// JWT key lets them sign tokens, a default admin password lets them sign in.
func TestScaffoldShipsNoKnownSecrets(t *testing.T) {
	for name, cfg := range testConfigs() {
		t.Run(name, func(t *testing.T) {
			config := GenerateTrokkyConfig(cfg)
			env := GenerateEnvExample(cfg)
			for _, known := range []string{"change-me", "change-in-production", "your-secret", "admin123"} {
				if strings.Contains(config, known) || strings.Contains(env, known) {
					t.Errorf("generated project contains the known value %q", known)
				}
			}
			if !strings.Contains(config, "jwtSecret: process.env.TROKKY_JWT_SECRET,") {
				t.Errorf("jwtSecret should read TROKKY_JWT_SECRET with no fallback:\n%s", config)
			}
			if !strings.Contains(config, "adminUser: process.env.ADMIN_PASSWORD ? {") {
				t.Errorf("the admin user should only be created when ADMIN_PASSWORD is set:\n%s", config)
			}
			for _, line := range []string{"TROKKY_JWT_SECRET=\n", "TROKKY_CLAIM_SECRET=\n", "ADMIN_PASSWORD=\n"} {
				if !strings.Contains(env, line) {
					t.Errorf(".env.example should leave %q empty, to be generated:\n%s", strings.TrimSpace(line), env)
				}
			}
		})
	}
}

func TestScaffoldWritesADotEnvWithGeneratedSecrets(t *testing.T) {
	for name, cfg := range testConfigs() {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "site")
			if err := Scaffold(cfg, dir); err != nil {
				t.Fatal(err)
			}
			env, err := os.ReadFile(filepath.Join(dir, ".env"))
			if err != nil {
				t.Fatalf(".env was not written: %v", err)
			}
			secrets := map[string]string{}
			for _, line := range strings.Split(string(env), "\n") {
				for _, key := range []string{"TROKKY_JWT_SECRET", "TROKKY_CLAIM_SECRET"} {
					if strings.HasPrefix(line, key+"=") {
						secrets[key] = strings.TrimPrefix(line, key+"=")
					}
				}
			}
			for _, key := range []string{"TROKKY_JWT_SECRET", "TROKKY_CLAIM_SECRET"} {
				if len(secrets[key]) < 60 {
					t.Errorf("%s = %q, want a generated value", key, secrets[key])
				}
			}
			if secrets["TROKKY_JWT_SECRET"] == secrets["TROKKY_CLAIM_SECRET"] {
				t.Error("the two secrets must differ")
			}
			info, _ := os.Stat(filepath.Join(dir, ".env"))
			if info.Mode().Perm() != 0o600 {
				t.Errorf(".env mode = %v, want 0600", info.Mode().Perm())
			}
			example, _ := os.ReadFile(filepath.Join(dir, ".env.example"))
			if !strings.Contains(string(example), "\nTROKKY_JWT_SECRET=\n") {
				t.Error(".env.example must keep the secret empty")
			}
		})
	}
}

func TestScaffoldKeepsAnExistingDotEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("MINE=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Scaffold(TemplateDefaults[TemplateMinimal], dir); err != nil {
		t.Fatal(err)
	}
	if env, _ := os.ReadFile(filepath.Join(dir, ".env")); string(env) != "MINE=1\n" {
		t.Errorf(".env was overwritten: %q", env)
	}
}

func TestScaffoldShipsNoPlaceholderThatSwitchesAFeatureOn(t *testing.T) {
	cfg := TemplateDefaults[TemplateFull]
	env := GenerateEnvExample(cfg)
	for _, placeholder := range []string{"your-client-id", "your-client-secret", "re_xxxxx", "your-site-key"} {
		if strings.Contains(env, placeholder) {
			t.Errorf(".env.example contains the placeholder %q", placeholder)
		}
	}
}

func TestClaimInstructionsMatchTheTemplate(t *testing.T) {
	if got := claimWhere(TemplateDefaults[TemplateAPIOnly]); !strings.Contains(got, "POST /api/auth/claim") {
		t.Errorf("api-only: %q", got)
	}
	if got := claimWhere(TemplateDefaults[TemplateMinimal]); !strings.Contains(got, "Studio") {
		t.Errorf("minimal: %q", got)
	}
}
