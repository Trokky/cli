package migrate

import (
	"encoding/json"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Warning is one thing the codemod cannot fix by itself and the user has to
// look at. Everything here comes from a text scan of the sources: we never
// execute or type-check the project.
type Warning struct {
	// Kind is one of "internal-import", "astro-env", "singleton-divergence"
	// or "singleton-near-miss".
	Kind string
	// File is relative to the scanned root, slash-separated. It is empty when
	// the warning is not tied to a file.
	File string
	// Line is 1-based, or 0 when unknown.
	Line int
	// Message is one actionable human-readable line.
	Message string
}

// ---------------------------------------------------------------------------
// shared scanning helpers
// ---------------------------------------------------------------------------

// The warning scan visits exactly the files and directories the codemod does,
// so it shares IsSourceFile and SkipDir with rewrite.go.

// warnLineAt returns the 1-based line number of byte offset idx in src.
func warnLineAt(src string, idx int) int {
	if idx < 0 || idx > len(src) {
		return 0
	}
	return 1 + strings.Count(src[:idx], "\n")
}

// warnEnclosingObject returns the byte range of the object literal that
// directly contains idx: it scans backwards to the nearest unmatched '{' and
// forwards to that brace's matching '}'. ok is false when idx is not inside a
// balanced object literal. Braces inside strings or comments are not
// recognised as such; site schema/structure files are regular enough that this
// is fine in practice.
func warnEnclosingObject(src string, idx int) (start, end int, ok bool) {
	depth := 0
	start = -1
	for i := idx - 1; i >= 0; i-- {
		switch src[i] {
		case '}':
			depth++
		case '{':
			if depth == 0 {
				start = i
			} else {
				depth--
			}
		}
		if start >= 0 {
			break
		}
	}
	if start < 0 {
		return 0, 0, false
	}
	depth = 0
	for i := start; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return start, i + 1, true
			}
		}
	}
	return 0, 0, false
}

// warnSpan is a half-open [start,end) byte range in a source file.
type warnSpan struct{ start, end int }

func (s warnSpan) contains(idx int) bool { return idx >= s.start && idx < s.end }

var warnFieldsKey = regexp.MustCompile(`\bfields\s*:\s*\{`)

// warnFieldsSpans returns the ranges covered by every `fields: { ... }` block
// in src, so that keys belonging to individual field definitions are not
// mistaken for keys of the schema object itself. Nested blocks are subsumed by
// their outermost enclosing block.
func warnFieldsSpans(src string) []warnSpan {
	var spans []warnSpan
	for _, m := range warnFieldsKey.FindAllStringIndex(src, -1) {
		open := m[1] - 1 // index of the '{'
		covered := false
		for _, s := range spans {
			if s.contains(open) {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		depth := 0
		for i := open; i < len(src); i++ {
			if src[i] == '{' {
				depth++
			} else if src[i] == '}' {
				depth--
				if depth == 0 {
					spans = append(spans, warnSpan{m[0], i + 1})
					break
				}
			}
		}
	}
	return spans
}

func warnInAnySpan(spans []warnSpan, idx int) bool {
	for _, s := range spans {
		if s.contains(idx) {
			return true
		}
	}
	return false
}

// warnStripSpans removes the parts of src[start:end] that fall inside spans.
func warnStripSpans(src string, start, end int, spans []warnSpan) string {
	var b strings.Builder
	for i := start; i < end; i++ {
		skipped := false
		for _, s := range spans {
			if s.contains(i) {
				i = s.end - 1
				skipped = true
				break
			}
		}
		if !skipped {
			b.WriteByte(src[i])
		}
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// 1. internal imports
// ---------------------------------------------------------------------------

var warnQuotedLiteral = regexp.MustCompile(`'([^'\n]*)'|"([^"\n]*)"`)

var (
	warnDirectInternal    = regexp.MustCompile(`^@trokky/[^/'"]+/(?:src|dist)/`)
	warnNodeModulesIntern = regexp.MustCompile(`(?:^|/)node_modules/@trokky/[^/'"]+/(?:src|dist)/`)
)

// warnIsInternalSpec reports whether spec reaches into a @trokky package's
// src/ or dist/ tree, either as a bare specifier or through a relative
// node_modules path.
func warnIsInternalSpec(spec string) bool {
	return warnDirectInternal.MatchString(spec) || warnNodeModulesIntern.MatchString(spec)
}

// ScanInternalImports flags string literals that import @trokky package
// internals. relPath is only used to label the returned warnings.
func ScanInternalImports(relPath, src string) []Warning {
	var out []Warning
	for _, m := range warnQuotedLiteral.FindAllStringSubmatchIndex(src, -1) {
		lo, hi := m[2], m[3]
		if lo < 0 {
			lo, hi = m[4], m[5]
		}
		if lo < 0 {
			continue
		}
		spec := src[lo:hi]
		if !warnIsInternalSpec(spec) {
			continue
		}
		out = append(out, Warning{
			Kind:    "internal-import",
			File:    relPath,
			Line:    warnLineAt(src, m[0]),
			Message: "imports package internals (" + spec + "); v2 does not ship src/ paths — rewrite by hand",
		})
	}
	return out
}

// ---------------------------------------------------------------------------
// 2. Astro build-time env
// ---------------------------------------------------------------------------

var warnAstroEnvRe = regexp.MustCompile(`import\.meta\.env\s*(?:\.\s*TROKKY_API_URL\b|\[\s*['"]TROKKY_API_URL['"]\s*\])`)

const warnAstroEnvMessage = "import.meta.env.TROKKY_API_URL is inlined at BUILD time by Astro — " +
	"set TROKKY_API_URL for the site build, not only for the server process, or every media request 404s at runtime"

// ScanAstroEnv flags build-time reads of TROKKY_API_URL. The caller decides
// whether the project is actually an Astro project (see IsAstroProject).
func ScanAstroEnv(relPath, src string) []Warning {
	var out []Warning
	for _, m := range warnAstroEnvRe.FindAllStringIndex(src, -1) {
		out = append(out, Warning{
			Kind:    "astro-env",
			File:    relPath,
			Line:    warnLineAt(src, m[0]),
			Message: warnAstroEnvMessage,
		})
	}
	return out
}

// IsAstroProject reports whether root looks like an Astro project: an
// astro.config.* at the root, any *.astro file in the tree, or an "astro"
// entry in package.json dependencies/devDependencies.
func IsAstroProject(root string) bool {
	for _, name := range []string{"astro.config.mjs", "astro.config.ts", "astro.config.js", "astro.config.cjs"} {
		if st, err := os.Stat(filepath.Join(root, name)); err == nil && !st.IsDir() {
			return true
		}
	}

	if data, err := os.ReadFile(filepath.Join(root, "package.json")); err == nil {
		var pkg struct {
			Dependencies    map[string]json.RawMessage `json:"dependencies"`
			DevDependencies map[string]json.RawMessage `json:"devDependencies"`
		}
		if json.Unmarshal(data, &pkg) == nil {
			if _, ok := pkg.Dependencies["astro"]; ok {
				return true
			}
			if _, ok := pkg.DevDependencies["astro"]; ok {
				return true
			}
		}
	}

	found := false
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if p != root && SkipDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if strings.EqualFold(filepath.Ext(d.Name()), ".astro") {
			found = true
			return fs.SkipAll
		}
		return nil
	})
	return found
}

// ---------------------------------------------------------------------------
// 3 + 4. singletons
// ---------------------------------------------------------------------------

var (
	warnSingletonType = regexp.MustCompile(`\btype\s*:\s*['"]singleton['"]`)
	warnSchemaTypeKey = regexp.MustCompile(`\bschemaType\s*:\s*['"]([^'"]+)['"]`)
	warnNameKey       = regexp.MustCompile(`\bname\s*:\s*['"]([^'"]+)['"]`)
	warnSingletonFlag = regexp.MustCompile(`\bsingleton\s*:\s*true\b`)
	warnIsSingleton   = regexp.MustCompile(`\bisSingleton\s*:\s*true\b`)
)

// warnStructEntry is one `type: 'singleton'` entry found in a structure file.
type warnStructEntry struct {
	name string
	file string
	line int
}

// scanStructureSingletons extracts the singleton entries of one structure file.
func scanStructureSingletons(relPath, src string) []warnStructEntry {
	var out []warnStructEntry
	for _, m := range warnSingletonType.FindAllStringIndex(src, -1) {
		start, end, ok := warnEnclosingObject(src, m[0])
		if !ok {
			continue
		}
		sm := warnSchemaTypeKey.FindStringSubmatch(src[start:end])
		if sm == nil {
			continue
		}
		out = append(out, warnStructEntry{name: sm[1], file: relPath, line: warnLineAt(src, start)})
	}
	return out
}

// warnSchemaDecl is one schema object declaring a document name.
type warnSchemaDecl struct {
	name    string
	file    string
	line    int
	hasFlag bool // singleton: true or type: 'singleton' on the schema object
}

// scanSchemaDecls extracts the schema objects of one schema file, together
// with whether each of them carries a singleton flag. Keys nested inside a
// `fields: { ... }` block belong to a field, not to the schema, and are
// ignored on both counts.
func scanSchemaDecls(relPath, src string) []warnSchemaDecl {
	fields := warnFieldsSpans(src)
	var out []warnSchemaDecl
	for _, m := range warnNameKey.FindAllStringSubmatchIndex(src, -1) {
		if warnInAnySpan(fields, m[0]) {
			continue
		}
		start, end, ok := warnEnclosingObject(src, m[0])
		if !ok {
			continue
		}
		body := warnStripSpans(src, start, end, fields)
		out = append(out, warnSchemaDecl{
			name:    src[m[2]:m[3]],
			file:    relPath,
			line:    warnLineAt(src, m[0]),
			hasFlag: warnSingletonFlag.MatchString(body) || warnSingletonType.MatchString(body),
		})
	}
	return out
}

// scanSchemaNearMisses flags `isSingleton: true` on schema objects: that key
// is a structure navigation key and is silently ignored on a schema.
func scanSchemaNearMisses(relPath, src string) []Warning {
	fields := warnFieldsSpans(src)
	var out []Warning
	for _, m := range warnIsSingleton.FindAllStringIndex(src, -1) {
		if warnInAnySpan(fields, m[0]) {
			continue
		}
		name := "?"
		if start, end, ok := warnEnclosingObject(src, m[0]); ok {
			body := warnStripSpans(src, start, end, fields)
			if nm := warnNameKey.FindStringSubmatch(body); nm != nil {
				name = nm[1]
			}
		}
		out = append(out, Warning{
			Kind:    "singleton-near-miss",
			File:    relPath,
			Line:    warnLineAt(src, m[0]),
			Message: "schema '" + name + "' uses isSingleton: true, which is ignored on schemas — use singleton: true",
		})
	}
	return out
}

// CheckSingletons cross-checks the singleton entries of the structure against
// the schemas they point at. Both maps are keyed by a path relative to the
// project root; the values are the file contents.
//
// A structure entry marked `type: 'singleton'` whose schema does not set
// `singleton: true` is the data-loss trap this whole command exists for, so it
// is reported once per collection name.
func CheckSingletons(structureSources map[string]string, schemaSources map[string]string) []Warning {
	var entries []warnStructEntry
	for _, rel := range warnSortedKeys(structureSources) {
		entries = append(entries, scanStructureSingletons(rel, structureSources[rel])...)
	}

	decls := map[string][]warnSchemaDecl{}
	var out []Warning
	for _, rel := range warnSortedKeys(schemaSources) {
		src := schemaSources[rel]
		for _, d := range scanSchemaDecls(rel, src) {
			decls[d.name] = append(decls[d.name], d)
		}
		out = append(out, scanSchemaNearMisses(rel, src)...)
	}

	seen := map[string]bool{}
	for _, e := range entries {
		if seen[e.name] {
			continue
		}
		seen[e.name] = true

		matches := decls[e.name]
		if len(matches) == 0 {
			out = append(out, Warning{
				Kind:    "singleton-divergence",
				File:    e.file,
				Line:    e.line,
				Message: "structure declares '" + e.name + "' as a singleton but no schema declaring name '" + e.name + "' was found",
			})
			continue
		}
		flagged := false
		for _, d := range matches {
			if d.hasFlag {
				flagged = true
				break
			}
		}
		if flagged {
			continue
		}
		out = append(out, Warning{
			Kind: "singleton-divergence",
			File: e.file,
			Line: e.line,
			Message: "structure declares '" + e.name + "' as a singleton but schema " + matches[0].file +
				" does not set singleton: true — `trokky restore` will POST instead of an id-preserving PUT, " +
				"regenerating the document id and orphaning the structure entry (data-loss trap). " +
				"Add `singleton: true` to the schema.",
		})
	}

	warnSort(out)
	return out
}

func warnSortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func warnSort(ws []Warning) {
	sort.SliceStable(ws, func(i, j int) bool {
		if ws[i].File != ws[j].File {
			return ws[i].File < ws[j].File
		}
		if ws[i].Line != ws[j].Line {
			return ws[i].Line < ws[j].Line
		}
		return ws[i].Kind < ws[j].Kind
	})
}

// ---------------------------------------------------------------------------
// walker
// ---------------------------------------------------------------------------

var warnStructureFiles = map[string]bool{
	"structure.ts":      true,
	"structure.js":      true,
	"structure.mjs":     true,
	"trokky.config.ts":  true,
	"trokky.config.js":  true,
	"trokky.config.mjs": true,
}

// warnIsStructureFile reports whether a file may hold the structure definition.
//
// The conventional basenames are matched first, but a project is free to split a long
// structure across a directory (structure/index.ts, structure/pages.ts) or to build it
// somewhere else entirely. Missing one of those files means the singleton check silently
// examines nothing and reports "no warnings", which is indistinguishable from a clean
// project and is exactly the case that loses documents on restore. So any source file
// that actually contains a singleton entry counts, wherever it lives.
func warnIsStructureFile(base, src string) bool {
	if warnStructureFiles[base] {
		return true
	}
	return strings.Contains(src, `type: 'singleton'`) || strings.Contains(src, `type: "singleton"`)
}

// warnIsSchemaFile reports whether a file holds document schemas: anything
// under a schemas/ or schema/ directory, a schemas.ts/schemas.js, or any file
// that mentions a document type.
func warnIsSchemaFile(rel, src string) bool {
	ext := strings.ToLower(path.Ext(rel))
	if ext != ".ts" && ext != ".js" && ext != ".mjs" {
		return false
	}
	dir, base := path.Split(rel)
	for _, seg := range strings.Split(strings.Trim(dir, "/"), "/") {
		if seg == "schemas" || seg == "schema" {
			return true
		}
	}
	if base == "schemas.ts" || base == "schemas.js" {
		return true
	}
	return strings.Contains(src, `type: 'document'`) || strings.Contains(src, `type: "document"`)
}

// ScanWarnings walks root and returns every warning found, sorted by file and
// line. Skipped directories (node_modules, .git, dist, build, .astro) and
// symlinks are not visited.
// ScanSummary records what the singleton check actually examined, so that a clean run can
// be told apart from a run that found nothing to look at. Reporting only "no warnings" makes
// those two identical, and the second one loses documents on restore.
type ScanSummary struct {
	StructureFiles   int
	SchemaFiles      int
	SingletonEntries int
}

// ScanWarnings walks root and returns every warning found.
func ScanWarnings(root string) ([]Warning, error) {
	w, _, err := ScanWarningsSummary(root)
	return w, err
}

func ScanWarningsSummary(root string) ([]Warning, ScanSummary, error) {
	astro := IsAstroProject(root)

	var out []Warning
	structures := map[string]string{}
	schemas := map[string]string{}

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != root && SkipDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 || !d.Type().IsRegular() {
			return nil
		}
		if !IsSourceFile(d.Name()) {
			return nil
		}
		rel, relErr := filepath.Rel(root, p)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)

		data, readErr := os.ReadFile(p)
		if readErr != nil {
			return readErr
		}
		src := string(data)

		out = append(out, ScanInternalImports(rel, src)...)
		if astro {
			out = append(out, ScanAstroEnv(rel, src)...)
		}

		// Not a switch: a schema file may itself use the `type: 'singleton'` spelling, and a
		// config file may hold both the structure and the schemas. Classifying it as only one
		// would hide the other from the check.
		if warnIsStructureFile(d.Name(), src) {
			structures[rel] = src
		}
		if warnIsSchemaFile(rel, src) {
			schemas[rel] = src
		}
		return nil
	})
	if err != nil {
		return nil, ScanSummary{}, err
	}

	out = append(out, CheckSingletons(structures, schemas)...)
	warnSort(out)

	summary := ScanSummary{StructureFiles: len(structures), SchemaFiles: len(schemas)}
	for rel, src := range structures {
		summary.SingletonEntries += len(scanStructureSingletons(rel, src))
	}
	return out, summary, nil
}
