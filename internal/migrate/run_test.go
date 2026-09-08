package migrate

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const fixturePkg = `{
  "name": "site",
  "dependencies": {
    "@trokky/core": "^0.1.4",
    "@trokky/express": "^0.1.4",
    "@trokky/studio": "^0.1.4",
    "express": "^4.19.2"
  }
}
`

const fixtureServer = `import { createTrokky } from '@trokky/core'
import { trokkyExpress } from '@trokky/express'
import { filesystemData } from '@trokky/adapter-filesystem-data'
import { console as mailConsole } from '@trokky/mail-adapter-console'
// @trokky/core mentioned in prose stays
export const app = createTrokky({ adapter: filesystemData(), mail: mailConsole() })
`

const fixtureVendored = "require('@trokky/core')\n"

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), fixturePkg)
	writeFile(t, filepath.Join(root, "src", "server.ts"), fixtureServer)
	writeFile(t, filepath.Join(root, "src", "styles.css"), "body { color: '@trokky/core' }\n")
	writeFile(t, filepath.Join(root, "node_modules", "@trokky", "core", "index.js"), fixtureVendored)
	writeFile(t, filepath.Join(root, "node_modules", "@trokky", "core", "package.json"), fixturePkg)
	writeFile(t, filepath.Join(root, "dist", "server.js"), fixtureVendored)
	writeFile(t, filepath.Join(root, "packages", "ui", "package.json"), fixturePkg)
	return root
}

// treeHash hashes every file's path and content under root.
func treeHash(t *testing.T, root string) string {
	t.Helper()
	h := sha256.New()
	var paths []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(root, p)
		h.Write([]byte(rel))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestRunDryRunTouchesNothing(t *testing.T) {
	root := fixture(t)
	before := treeHash(t, root)

	res, err := Run(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if after := treeHash(t, root); after != before {
		t.Fatal("dry run modified the tree")
	}

	// package.json, src/server.ts and packages/ui/package.json change;
	// node_modules and dist are skipped entirely.
	var paths []string
	for _, f := range res.Files {
		paths = append(paths, f.Path)
	}
	want := []string{"package.json", "packages/ui/package.json", "src/server.ts"}
	if len(paths) != len(want) {
		t.Fatalf("changed files = %v, want %v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Errorf("changed files = %v, want %v (order matters)", paths, want)
		}
	}
	if res.Scanned != 3 {
		t.Errorf("Scanned = %d, want 3", res.Scanned)
	}
	wantHits := map[string]int{
		"@trokky/core":                    1,
		"@trokky/express":                 1,
		"@trokky/adapter-filesystem-data": 1,
		"@trokky/mail-adapter-console":    1,
	}
	for k, v := range wantHits {
		if res.Hits[k] != v {
			t.Errorf("Hits[%q] = %d, want %d (all: %v)", k, res.Hits[k], v, res.Hits)
		}
	}
	if len(res.Hits) != len(wantHits) {
		t.Errorf("Hits = %v, want %v", res.Hits, wantHits)
	}

	for _, f := range res.Files {
		switch f.Kind {
		case KindPackageJSON:
			if len(f.Changes) == 0 {
				t.Errorf("%s: no changes recorded", f.Path)
			}
		case KindSource:
			if len(f.Hits) == 0 {
				t.Errorf("%s: no hits recorded", f.Path)
			}
		default:
			t.Errorf("%s: unexpected kind %q", f.Path, f.Kind)
		}
	}
}

func TestRunWrite(t *testing.T) {
	root := fixture(t)
	vendored := filepath.Join(root, "node_modules", "@trokky", "core", "index.js")
	distFile := filepath.Join(root, "dist", "server.js")
	css := filepath.Join(root, "src", "styles.css")

	res, err := Run(Options{Root: root, Write: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Files) != 3 {
		t.Fatalf("changed %d files, want 3: %+v", len(res.Files), res.Files)
	}

	src := readFile(t, filepath.Join(root, "src", "server.ts"))
	wantSrc := `import { createTrokky } from '@trokky/trokky'
import { trokkyExpress } from '@trokky/trokky/express'
import { filesystemData } from '@trokky/trokky/adapters/filesystem-data'
import { console as mailConsole } from '@trokky/trokky/mail/console'
// @trokky/core mentioned in prose stays
export const app = createTrokky({ adapter: filesystemData(), mail: mailConsole() })
`
	if src != wantSrc {
		t.Errorf("src/server.ts:\ngot:\n%s\nwant:\n%s", src, wantSrc)
	}

	pkg := readFile(t, filepath.Join(root, "package.json"))
	wantPkg := `{
  "name": "site",
  "dependencies": {
    "@trokky/trokky": "^2.0.0",
    "@trokky/studio": "^2.0.0",
    "express": "^4.19.2"
  }
}
`
	if pkg != wantPkg {
		t.Errorf("package.json:\ngot:\n%s\nwant:\n%s", pkg, wantPkg)
	}

	for _, p := range []string{vendored, distFile} {
		if got := readFile(t, p); got != fixtureVendored {
			t.Errorf("%s was modified: %q", p, got)
		}
	}
	if got := readFile(t, css); got != "body { color: '@trokky/core' }\n" {
		t.Errorf("css was modified: %q", got)
	}

	// Running again is a no-op: everything is already migrated.
	res2, err := Run(Options{Root: root, Write: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(res2.Files) != 0 {
		t.Errorf("second run changed %+v, want none", res2.Files)
	}
}

func TestRunPreservesFileMode(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "server.ts")
	writeFile(t, p, "import '@trokky/core'\n")
	if err := os.Chmod(p, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(Options{Root: root, Write: true}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestRunSkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "real.ts")
	writeFile(t, target, "import '@trokky/core'\n")
	link := filepath.Join(root, "link.ts")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	res, err := Run(Options{Root: root, Write: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Scanned != 1 || len(res.Files) != 1 || res.Files[0].Path != "real.ts" {
		t.Errorf("scanned=%d files=%+v, want only real.ts", res.Scanned, res.Files)
	}
}

func TestRunBadPackageJSON(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "package.json"), "{ not json")
	if _, err := Run(Options{Root: root}); err == nil {
		t.Fatal("expected an error for an unparseable package.json")
	}
}

func TestRunMissingRoot(t *testing.T) {
	if _, err := Run(Options{Root: filepath.Join(t.TempDir(), "nope")}); err == nil {
		t.Fatal("expected an error for a missing root")
	}
}

func TestResultReport(t *testing.T) {
	res := &Result{
		Files: []FileChange{
			{Path: "src/server.ts", Kind: KindSource, Hits: map[string]int{"@trokky/core": 2, "@trokky/express": 1}},
			{Path: "package.json", Kind: KindPackageJSON, Changes: []string{"removed @trokky/core", "added @trokky/trokky ^2.0.0"}},
		},
		Hits:    map[string]int{"@trokky/core": 2, "@trokky/express": 1},
		Scanned: 2,
		Warnings: []Warning{
			{Kind: "internal-import", File: "scripts/convert.ts", Line: 3, Message: "imports package internals (…)"},
			{Kind: "singleton-divergence", File: "structure.ts", Line: 12, Message: "structure declares 'homepage' as a singleton but …"},
		},
	}

	var buf bytes.Buffer
	res.Report(&buf, false)
	got := buf.String()
	for _, want := range []string{
		"Files changed (2):",
		"src/server.ts",
		"@trokky/express x1, @trokky/core x2",
		"package.json",
		"removed @trokky/core, added @trokky/trokky ^2.0.0",
		"Specifiers rewritten by mapping:",
		"@trokky/express",
		"-> @trokky/trokky",
		"WARNINGS (2):",
		"[internal-import] scripts/convert.ts:3:",
		"[singleton-divergence] structure.ts:12:",
		"Fix the singleton warnings BEFORE running `trokky restore`",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("report is missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "@trokky/i18n") {
		t.Errorf("report lists mappings with no hits:\n%s", got)
	}

	buf.Reset()
	res.Report(&buf, true)
	if !strings.Contains(buf.String(), "Files that would change (2):") {
		t.Errorf("dry-run report:\n%s", buf.String())
	}

	buf.Reset()
	(&Result{Hits: map[string]int{}}).Report(&buf, false)
	got = buf.String()
	if !strings.Contains(got, "Files changed (0):") || !strings.Contains(got, "No warnings.") {
		t.Errorf("empty report:\n%s", got)
	}
	if strings.Contains(got, "Fix the singleton warnings") {
		t.Errorf("empty report has the singleton footer:\n%s", got)
	}
}

func TestResultSummary(t *testing.T) {
	root := fixture(t)
	res, err := Run(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if s := res.Summary(); s == "" {
		t.Error("empty summary")
	}
}
