package migrate

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

// A realistic v0.1.x site: the split packages in package.json, every import
// style in one server file, an internal import, an Astro page reading the API
// URL at build time, and a structure/schemas pair with two singleton traps.

const e2ePackageJSON = `{
  "name": "acme-site",
  "type": "module",
  "scripts": {
    "dev": "tsx server.ts"
  },
  "dependencies": {
    "@trokky/core": "^0.1.4",
    "@trokky/express": "^0.1.4",
    "@trokky/adapter-filesystem-data": "^0.1.4",
    "@trokky/adapter-filesystem-media": "^0.1.4",
    "@trokky/mail-adapter-console": "^0.1.4",
    "@trokky/studio": "^0.1.4",
    "@trokky/client": "^0.1.4",
    "express": "^4.19.0"
  },
  "devDependencies": {
    "typescript": "^5.4.0"
  }
}
`

const e2eServer = `import express from 'express'
import { trokkyExpress } from '@trokky/express'
import { createTrokky } from '@trokky/core'
import '@trokky/adapter-filesystem-data'
import '@trokky/adapter-filesystem-media'
import { ConsoleMailAdapter } from '@trokky/mail-adapter-console'

const mail = require("@trokky/mail")
const i18n = await import('@trokky/i18n')

// see @trokky/core docs
export const app = express()
app.use(trokkyExpress(createTrokky({ mail: new ConsoleMailAdapter(), i18n })))
`

const e2eConvert = `import { toHTML } from '../node_modules/@trokky/fields/src/definitions/RichTextField/format-converter.js'
import { defineField } from '@trokky/fields'

export { toHTML, defineField }
`

const e2eStructure = `import { defineStructure } from '@trokky/structure'

export default defineStructure([
  { type: 'singleton', title: 'Home', schemaType: 'homepage', documentId: 'home' },
  { type: 'singleton', title: 'Settings', schemaType: 'settings' },
  { type: 'singleton', title: 'Legal', schemaType: 'legal' },
  { type: 'list', title: 'Articles', schemaType: 'article' },
])
`

const e2eIndexAstro = `---
const apiUrl = import.meta.env.TROKKY_API_URL
---
<a href={apiUrl}>api</a>
`

const e2eVendored = "module.exports = require('@trokky/core')\n"

func e2eFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"package.json":                       e2ePackageJSON,
		"server.ts":                          e2eServer,
		"scripts/convert.ts":                 e2eConvert,
		"structure.ts":                       e2eStructure,
		"schemas/homepage.ts":                "export default {\n  name: 'homepage',\n  type: 'document',\n  fields: { title: { name: 'title', type: 'string' } },\n}\n",
		"schemas/settings.ts":                "export default {\n  name: 'settings',\n  type: 'document',\n  singleton: true,\n  fields: { siteName: { name: 'siteName', type: 'string' } },\n}\n",
		"schemas/legal.ts":                   "export default {\n  name: 'legal',\n  type: 'document',\n  isSingleton: true,\n  fields: { body: { name: 'body', type: 'richtext' } },\n}\n",
		"schemas/article.ts":                 "export default {\n  name: 'article',\n  type: 'document',\n  fields: { title: { name: 'title', type: 'string' } },\n}\n",
		"src/pages/index.astro":              e2eIndexAstro,
		"astro.config.mjs":                   "export default { output: 'static' }\n",
		"node_modules/@trokky/core/index.js": e2eVendored,
		"dist/out.js":                        e2eVendored,
	}
	for rel, content := range files {
		writeFile(t, filepath.Join(root, filepath.FromSlash(rel)), content)
	}
	return root
}

func e2eWarningsOfKind(ws []Warning, kind string) []Warning {
	var out []Warning
	for _, w := range ws {
		if w.Kind == kind {
			out = append(out, w)
		}
	}
	return out
}

func TestE2EDryRunChangesNothing(t *testing.T) {
	root := e2eFixture(t)
	before := treeHash(t, root)

	res, err := Run(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if after := treeHash(t, root); after != before {
		t.Fatal("dry run modified the tree")
	}

	var got []string
	for _, f := range res.Files {
		got = append(got, f.Path)
	}
	want := []string{"package.json", "scripts/convert.ts", "server.ts", "structure.ts"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("changed files = %v, want %v", got, want)
	}
}

func TestE2EWrite(t *testing.T) {
	root := e2eFixture(t)

	res, err := Run(Options{Root: root, Write: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Files) != 4 {
		t.Fatalf("changed %d files, want 4: %+v", len(res.Files), res.Files)
	}

	// --- no old specifier survives outside node_modules/dist ---------------
	//
	// The relative node_modules path in scripts/convert.ts is deliberately not
	// a bare specifier, so the codemod leaves it alone and only warns about it.
	const internalImport = "'../node_modules/@trokky/fields/src/definitions/RichTextField/format-converter.js'"
	for _, rel := range []string{"server.ts", "scripts/convert.ts", "structure.ts", "package.json"} {
		src := readFile(t, filepath.Join(root, filepath.FromSlash(rel)))
		stripped := strings.ReplaceAll(src, internalImport, "")
		for _, m := range Mappings {
			for _, pat := range []string{"'" + m.From + "'", `"` + m.From + `"`, "'" + m.From + "/"} {
				if strings.Contains(stripped, pat) {
					t.Errorf("%s still contains %s", rel, pat)
				}
			}
		}
	}

	// --- server.ts ---------------------------------------------------------
	server := readFile(t, filepath.Join(root, "server.ts"))
	for _, want := range []string{
		"from '@trokky/trokky/express'",
		"from '@trokky/trokky'",
		"import '@trokky/trokky/adapters/filesystem-data'",
		"import '@trokky/trokky/adapters/filesystem-media'",
		"from '@trokky/trokky/mail/console'",
		`require("@trokky/trokky/mail")`,
		"await import('@trokky/trokky/i18n')",
		"// see @trokky/core docs", // prose is never rewritten
	} {
		if !strings.Contains(server, want) {
			t.Errorf("server.ts is missing %q:\n%s", want, server)
		}
	}

	// --- scripts/convert.ts ------------------------------------------------
	convert := readFile(t, filepath.Join(root, "scripts", "convert.ts"))
	if !strings.Contains(convert, "from '@trokky/studio'") {
		t.Errorf("convert.ts: bare @trokky/fields was not rewritten:\n%s", convert)
	}
	if !strings.Contains(convert, internalImport) {
		t.Errorf("convert.ts: the internal import should be left for the human:\n%s", convert)
	}

	// --- package.json ------------------------------------------------------
	pkgSrc := readFile(t, filepath.Join(root, "package.json"))
	var pkg struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal([]byte(pkgSrc), &pkg); err != nil {
		t.Fatalf("rewritten package.json is not valid JSON: %v\n%s", err, pkgSrc)
	}
	wantDeps := map[string]string{
		"@trokky/trokky": "^2.0.0",
		"@trokky/studio": "^2.0.0",
		"@trokky/client": "^2.0.0",
		"express":        "^4.19.0",
	}
	for k, v := range wantDeps {
		if pkg.Dependencies[k] != v {
			t.Errorf("dependencies[%q] = %q, want %q", k, pkg.Dependencies[k], v)
		}
	}
	if len(pkg.Dependencies) != len(wantDeps) {
		t.Errorf("dependencies = %v, want exactly %v", pkg.Dependencies, wantDeps)
	}
	for _, m := range Mappings {
		if _, ok := pkg.Dependencies[m.From]; ok {
			t.Errorf("dependencies still has the old package %q", m.From)
		}
	}
	if pkg.DevDependencies["typescript"] != "^5.4.0" {
		t.Errorf("devDependencies = %v, want typescript untouched", pkg.DevDependencies)
	}
	// Key order and the 2-space indentation are preserved.
	if i, j, k, d := strings.Index(pkgSrc, `"name"`), strings.Index(pkgSrc, `"type"`),
		strings.Index(pkgSrc, `"scripts"`), strings.Index(pkgSrc, `"dependencies"`); !(i < j && j < k && k < d) {
		t.Errorf("key order changed (name=%d type=%d scripts=%d dependencies=%d):\n%s", i, j, k, d, pkgSrc)
	}
	if !strings.Contains(pkgSrc, "\n    \"@trokky/trokky\": \"^2.0.0\",") {
		t.Errorf("2-space indentation was not preserved:\n%s", pkgSrc)
	}

	// --- untouched trees ---------------------------------------------------
	for _, rel := range []string{"node_modules/@trokky/core/index.js", "dist/out.js"} {
		if got := readFile(t, filepath.Join(root, filepath.FromSlash(rel))); got != e2eVendored {
			t.Errorf("%s was modified: %q", rel, got)
		}
	}

	// --- warnings ----------------------------------------------------------
	div := e2eWarningsOfKind(res.Warnings, "singleton-divergence")
	var divNames []string
	for _, w := range div {
		for _, name := range []string{"homepage", "settings", "legal", "article"} {
			if strings.Contains(w.Message, "'"+name+"'") {
				divNames = append(divNames, name)
				break
			}
		}
	}
	if strings.Join(divNames, ",") != "homepage,legal" {
		t.Errorf("singleton-divergence warnings = %v (%+v), want homepage and legal only", divNames, div)
	}

	near := e2eWarningsOfKind(res.Warnings, "singleton-near-miss")
	if len(near) != 1 || !strings.Contains(near[0].Message, "'legal'") || near[0].File != "schemas/legal.ts" {
		t.Errorf("singleton-near-miss = %+v, want exactly one for legal", near)
	}

	internal := e2eWarningsOfKind(res.Warnings, "internal-import")
	if len(internal) != 1 || internal[0].File != "scripts/convert.ts" || internal[0].Line != 1 {
		t.Fatalf("internal-import = %+v, want one at scripts/convert.ts:1", internal)
	}
	// The scan ran before any file was written, so it still quotes @trokky/fields.
	if !strings.Contains(internal[0].Message, "@trokky/fields") {
		t.Errorf("internal-import message = %q, want the original specifier", internal[0].Message)
	}

	env := e2eWarningsOfKind(res.Warnings, "astro-env")
	if len(env) != 1 || env[0].File != "src/pages/index.astro" {
		t.Errorf("astro-env = %+v, want one for src/pages/index.astro", env)
	}
}

func TestE2EIdempotent(t *testing.T) {
	root := e2eFixture(t)
	if _, err := Run(Options{Root: root, Write: true}); err != nil {
		t.Fatal(err)
	}
	after := treeHash(t, root)

	res, err := Run(Options{Root: root, Write: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Files) != 0 {
		t.Errorf("second run changed %+v, want none", res.Files)
	}
	if treeHash(t, root) != after {
		t.Error("second run modified the tree")
	}
}
