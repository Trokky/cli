package migrate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanInternalImports(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		wantSpec string // "" means no warning expected
		wantLine int
	}{
		{
			name:     "bare src subpath",
			src:      "import x from 'y'\nimport { toHTML } from '@trokky/fields/src/definitions/RichTextField/format-converter.js'\n",
			wantSpec: "@trokky/fields/src/definitions/RichTextField/format-converter.js",
			wantLine: 2,
		},
		{
			name:     "relative node_modules path",
			src:      "import { toHTML } from '../node_modules/@trokky/fields/src/definitions/RichTextField/format-converter.js'\n",
			wantSpec: "../node_modules/@trokky/fields/src/definitions/RichTextField/format-converter.js",
			wantLine: 1,
		},
		{
			name:     "dist subpath is internal too",
			src:      "const m = require(\"@trokky/core/dist/index.js\")\n",
			wantSpec: "@trokky/core/dist/index.js",
			wantLine: 1,
		},
		{name: "plain package", src: "import { defineField } from '@trokky/fields'\n"},
		{name: "public v2 subpath", src: "import fs from '@trokky/trokky/adapters/filesystem-data'\n"},
		{name: "unrelated src path", src: "import a from './src/lib/a.js'\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ScanInternalImports("scripts/a.ts", tc.src)
			if tc.wantSpec == "" {
				if len(got) != 0 {
					t.Fatalf("expected no warnings, got %+v", got)
				}
				return
			}
			if len(got) != 1 {
				t.Fatalf("expected 1 warning, got %+v", got)
			}
			w := got[0]
			if w.Kind != "internal-import" {
				t.Errorf("kind = %q", w.Kind)
			}
			if w.File != "scripts/a.ts" {
				t.Errorf("file = %q", w.File)
			}
			if w.Line != tc.wantLine {
				t.Errorf("line = %d, want %d", w.Line, tc.wantLine)
			}
			if !strings.Contains(w.Message, tc.wantSpec) {
				t.Errorf("message %q does not mention %q", w.Message, tc.wantSpec)
			}
			if !strings.Contains(w.Message, "rewrite by hand") {
				t.Errorf("message %q is not actionable", w.Message)
			}
		})
	}
}

func TestScanAstroEnv(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		wantLines []int
	}{
		{"dot form", "const u = import.meta.env.TROKKY_API_URL\n", []int{1}},
		{"bracket single quotes", "const u = import.meta.env['TROKKY_API_URL']\n", []int{1}},
		{"bracket double quotes", "\nconst u = import.meta.env[\"TROKKY_API_URL\"]\n", []int{2}},
		{"other var", "const u = import.meta.env.PUBLIC_SITE\n", nil},
		{"process env untouched", "const u = process.env.TROKKY_API_URL\n", nil},
		{"two hits", "a(import.meta.env.TROKKY_API_URL)\nb(import.meta.env.TROKKY_API_URL)\n", []int{1, 2}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ScanAstroEnv("src/pages/index.astro", tc.src)
			if len(got) != len(tc.wantLines) {
				t.Fatalf("got %d warnings, want %d: %+v", len(got), len(tc.wantLines), got)
			}
			for i, want := range tc.wantLines {
				if got[i].Kind != "astro-env" {
					t.Errorf("kind = %q", got[i].Kind)
				}
				if got[i].Line != want {
					t.Errorf("line[%d] = %d, want %d", i, got[i].Line, want)
				}
				if !strings.Contains(got[i].Message, "BUILD time") {
					t.Errorf("message = %q", got[i].Message)
				}
			}
		})
	}
}

// warnWriteTree writes a fixture tree under a fresh temp dir and returns it.
func warnWriteTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		p := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func warnKinds(ws []Warning, kind string) []Warning {
	var out []Warning
	for _, w := range ws {
		if w.Kind == kind {
			out = append(out, w)
		}
	}
	return out
}

func TestIsAstroProject(t *testing.T) {
	tests := []struct {
		name  string
		files map[string]string
		want  bool
	}{
		{"astro config mjs", map[string]string{"astro.config.mjs": "export default {}\n"}, true},
		{"astro config ts", map[string]string{"astro.config.ts": "export default {}\n"}, true},
		{"astro file", map[string]string{"src/pages/index.astro": "---\n---\n<h1/>\n"}, true},
		{"package.json dep", map[string]string{"package.json": `{"dependencies":{"astro":"^4.0.0"}}`}, true},
		{"package.json devDep", map[string]string{"package.json": `{"devDependencies":{"astro":"^4.0.0"}}`}, true},
		{"plain node project", map[string]string{"package.json": `{"dependencies":{"trokky":"^2.0.0"}}`, "server.ts": "//\n"}, false},
		{"astro only in node_modules", map[string]string{"node_modules/x/a.astro": "x"}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsAstroProject(warnWriteTree(t, tc.files)); got != tc.want {
				t.Errorf("IsAstroProject = %v, want %v", got, tc.want)
			}
		})
	}
}

const warnAstroPage = "---\nconst url = import.meta.env.TROKKY_API_URL\n---\n<img src={url} />\n"

func TestScanWarningsAstroEnvOnlyForAstroProjects(t *testing.T) {
	astroRoot := warnWriteTree(t, map[string]string{
		"astro.config.mjs":      "export default {}\n",
		"src/pages/index.astro": warnAstroPage,
	})
	got, err := ScanWarnings(astroRoot)
	if err != nil {
		t.Fatal(err)
	}
	env := warnKinds(got, "astro-env")
	if len(env) != 1 {
		t.Fatalf("astro project: got %+v", got)
	}
	if env[0].File != "src/pages/index.astro" || env[0].Line != 2 {
		t.Errorf("got %+v", env[0])
	}

	// Same source, no astro markers anywhere: not our problem.
	plainRoot := warnWriteTree(t, map[string]string{
		"package.json": `{"dependencies":{"trokky":"^2.0.0"}}`,
		"src/page.ts":  warnAstroPage,
	})
	got, err = ScanWarnings(plainRoot)
	if err != nil {
		t.Fatal(err)
	}
	if env := warnKinds(got, "astro-env"); len(env) != 0 {
		t.Fatalf("non-astro project: got %+v", env)
	}
}

func TestScanWarningsSkipsIgnoredDirs(t *testing.T) {
	root := warnWriteTree(t, map[string]string{
		"node_modules/@trokky/fields/src/index.ts": "import x from '@trokky/core/src/a.js'\n",
		"dist/bundle.js":     "import x from '@trokky/core/src/a.js'\n",
		"scripts/convert.ts": "import { toHTML } from '../node_modules/@trokky/fields/src/definitions/RichTextField/format-converter.js'\n",
	})
	got, err := ScanWarnings(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].File != "scripts/convert.ts" {
		t.Fatalf("got %+v", got)
	}
}

// warnSingletonFixture builds a project with one structure entry for
// 'homepage' plus the given schema body.
func warnSingletonFixture(schema string) map[string]string {
	return map[string]string{
		"structure.ts": "export const structure = [\n" +
			"  { type: 'singleton', title: 'Home', schemaType: 'homepage', documentId: 'home' },\n" +
			"]\n",
		"schemas/homepage.ts": schema,
	}
}

func TestCheckSingletonsViaScanWarnings(t *testing.T) {
	tests := []struct {
		name          string
		files         map[string]string
		wantDivergent []string // collection names, in order
		wantNearMiss  int
	}{
		{
			name: "schema missing the flag",
			files: warnSingletonFixture(
				"export default {\n  name: 'homepage',\n  type: 'document',\n  fields: { title: { type: 'string' } },\n}\n"),
			wantDivergent: []string{"homepage"},
		},
		{
			name: "schema sets singleton: true",
			files: warnSingletonFixture(
				"export default {\n  name: 'homepage',\n  type: 'document',\n  singleton: true,\n  fields: { title: { type: 'string' } },\n}\n"),
		},
		{
			name: "schema sets type: 'singleton'",
			files: warnSingletonFixture(
				"export default {\n  name: 'homepage',\n  type: 'singleton',\n  fields: { title: { type: 'string' } },\n}\n"),
		},
		{
			name: "schema uses isSingleton",
			files: warnSingletonFixture(
				"export default {\n  name: 'homepage',\n  type: 'document',\n  isSingleton: true,\n  fields: { title: { type: 'string' } },\n}\n"),
			wantDivergent: []string{"homepage"},
			wantNearMiss:  1,
		},
		{
			name: "no schema declares the name",
			files: map[string]string{
				"structure.ts": "export const structure = [\n" +
					"  { type: 'singleton', title: 'Home', schemaType: 'homepage' },\n]\n",
				"schemas/post.ts": "export default {\n  name: 'post',\n  type: 'document',\n  fields: {},\n}\n",
			},
			wantDivergent: []string{"homepage"},
		},
		{
			name: "multi-line entry with schemaType before type",
			files: map[string]string{
				"structure.ts":        "export const structure = [\n  {\n    title: 'Home',\n    schemaType: 'homepage',\n    type: 'singleton',\n  },\n]\n",
				"schemas/homepage.ts": "export default {\n  name: 'homepage',\n  type: 'document',\n  fields: {},\n}\n",
			},
			wantDivergent: []string{"homepage"},
		},
		{
			name: "two singletons, only one missing the flag",
			files: map[string]string{
				"structure.ts": "export const structure = [\n" +
					"  { type: 'singleton', schemaType: 'homepage' },\n" +
					"  { type: 'singleton', schemaType: 'settings' },\n]\n",
				"schemas/homepage.ts": "export default {\n  name: 'homepage',\n  type: 'document',\n  singleton: true,\n  fields: {},\n}\n",
				"schemas/settings.ts": "export default {\n  name: 'settings',\n  type: 'document',\n  fields: {},\n}\n",
			},
			wantDivergent: []string{"settings"},
		},
		{
			name: "structure inline in trokky.config.ts",
			files: map[string]string{
				"trokky.config.ts": "export default {\n  structure: [\n    { schemaType: 'homepage', type: 'singleton' },\n  ],\n}\n",
				"schemas.ts":       "export const schemas = [{ name: 'homepage', type: 'document', fields: {} }]\n",
			},
			wantDivergent: []string{"homepage"},
		},
		{
			name: "nested field keys do not satisfy the flag",
			files: warnSingletonFixture(
				"export default {\n  name: 'homepage',\n  type: 'document',\n  fields: {\n    layout: { name: 'layout', type: 'singleton', singleton: true },\n  },\n}\n"),
			wantDivergent: []string{"homepage"},
		},
		{
			name: "isSingleton in a structure file is not a near miss",
			files: map[string]string{
				"structure.ts":        "export const structure = [\n  { type: 'singleton', schemaType: 'homepage', isSingleton: true },\n]\n",
				"schemas/homepage.ts": "export default {\n  name: 'homepage',\n  type: 'document',\n  singleton: true,\n  fields: {},\n}\n",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ScanWarnings(warnWriteTree(t, tc.files))
			if err != nil {
				t.Fatal(err)
			}
			div := warnKinds(got, "singleton-divergence")
			if len(div) != len(tc.wantDivergent) {
				t.Fatalf("got %d divergence warnings, want %d: %+v", len(div), len(tc.wantDivergent), got)
			}
			for i, name := range tc.wantDivergent {
				if !strings.Contains(div[i].Message, "'"+name+"'") {
					t.Errorf("warning %d = %q, want it to name %q", i, div[i].Message, name)
				}
			}
			if n := len(warnKinds(got, "singleton-near-miss")); n != tc.wantNearMiss {
				t.Errorf("got %d near-miss warnings, want %d: %+v", n, tc.wantNearMiss, got)
			}
		})
	}
}

func TestCheckSingletonsMessages(t *testing.T) {
	structure := map[string]string{
		"structure.ts": "export const structure = [\n  { type: 'singleton', schemaType: 'homepage' },\n  { type: 'singleton', schemaType: 'ghost' },\n]\n",
	}
	schemas := map[string]string{
		"schemas/homepage.ts": "export default {\n  name: 'homepage',\n  type: 'document',\n  isSingleton: true,\n  fields: {},\n}\n",
	}

	got := CheckSingletons(structure, schemas)
	if len(got) != 3 {
		t.Fatalf("got %+v", got)
	}

	// Sorted by file then line: structure.ts:2, structure.ts:3, then schemas/... .
	// "schemas/..." < "structure.ts" so the near miss comes first.
	if got[0].Kind != "singleton-near-miss" || got[0].File != "schemas/homepage.ts" || got[0].Line != 4 {
		t.Errorf("near miss = %+v", got[0])
	}
	if !strings.Contains(got[0].Message, "schema 'homepage' uses isSingleton: true") ||
		!strings.Contains(got[0].Message, "use singleton: true") {
		t.Errorf("near miss message = %q", got[0].Message)
	}

	if got[1].File != "structure.ts" || got[1].Line != 2 {
		t.Errorf("divergence = %+v", got[1])
	}
	if !strings.Contains(got[1].Message, "schemas/homepage.ts does not set singleton: true") ||
		!strings.Contains(got[1].Message, "data-loss trap") {
		t.Errorf("divergence message = %q", got[1].Message)
	}

	if got[2].File != "structure.ts" || got[2].Line != 3 {
		t.Errorf("missing schema warning = %+v", got[2])
	}
	if !strings.Contains(got[2].Message, "no schema declaring name 'ghost' was found") {
		t.Errorf("missing schema message = %q", got[2].Message)
	}
}

func TestScanWarningsSortedByFileThenLine(t *testing.T) {
	root := warnWriteTree(t, map[string]string{
		"astro.config.mjs": "export default {}\n",
		"src/b.ts":         "\n\nimport x from '@trokky/core/src/a.js'\n",
		"src/a.astro":      "---\nconst u = import.meta.env.TROKKY_API_URL\nimport y from '@trokky/fields/src/x.js'\n---\n",
	})
	got, err := ScanWarnings(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %+v", got)
	}
	want := []struct {
		file string
		line int
	}{{"src/a.astro", 2}, {"src/a.astro", 3}, {"src/b.ts", 3}}
	for i, w := range want {
		if got[i].File != w.file || got[i].Line != w.line {
			t.Errorf("warning %d = %s:%d, want %s:%d", i, got[i].File, got[i].Line, w.file, w.line)
		}
	}
}

func TestScanWarningsMissingRoot(t *testing.T) {
	if _, err := ScanWarnings(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("expected an error for a missing root")
	}
}
