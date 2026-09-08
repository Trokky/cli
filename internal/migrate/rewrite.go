// Package migrate implements the codemod that upgrades a Trokky site project
// from the old split npm packages (v0.1.x) to the consolidated v2 packages.
package migrate

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Mapping is a single old-package -> new-subpath rewrite rule.
type Mapping struct{ From, To string }

// Mappings is ordered longest From first, so that e.g.
// @trokky/mail-adapter-console wins over @trokky/mail and
// @trokky/adapter-filesystem-media over @trokky/adapter-filesystem.
//
// @trokky/client and @trokky/studio keep their names and are absent here.
var Mappings = []Mapping{
	{"@trokky/adapter-filesystem-media", "@trokky/trokky/adapters/filesystem-media"},
	{"@trokky/adapter-filesystem-data", "@trokky/trokky/adapters/filesystem-data"},
	{"@trokky/adapter-postgres-data", "@trokky/trokky/adapters/postgres-data"},
	{"@trokky/mail-adapter-console", "@trokky/trokky/mail/console"},
	{"@trokky/mail-adapter-resend", "@trokky/trokky/mail/resend"},
	{"@trokky/adapter-filesystem", "@trokky/trokky/adapters/filesystem-data"},
	{"@trokky/mail-adapter-smtp", "@trokky/trokky/mail/smtp"},
	{"@trokky/structure", "@trokky/trokky/structure"},
	{"@trokky/express", "@trokky/trokky/express"},
	{"@trokky/routes", "@trokky/trokky"},
	{"@trokky/fields", "@trokky/studio"},
	{"@trokky/types", "@trokky/trokky/types"},
	{"@trokky/core", "@trokky/trokky"},
	{"@trokky/mail", "@trokky/trokky/mail"},
	{"@trokky/i18n", "@trokky/trokky/i18n"},
}

// RewriteSpecifier rewrites a bare module specifier if it equals a mapped
// package exactly or is a subpath of one. The longest match wins, so
// @trokky/mail-adapter-console never matches @trokky/mail first. It reports
// whether a rewrite happened.
func RewriteSpecifier(spec string) (string, bool) {
	for _, m := range Mappings {
		if spec == m.From {
			return m.To, true
		}
		if strings.HasPrefix(spec, m.From+"/") {
			return m.To + spec[len(m.From):], true
		}
	}
	return spec, false
}

// matchedMapping returns the From key that RewriteSpecifier would use, or "".
func matchedMapping(spec string) string {
	for _, m := range Mappings {
		if spec == m.From || strings.HasPrefix(spec, m.From+"/") {
			return m.From
		}
	}
	return ""
}

// quotedSpecifier matches a single- or double-quoted string literal whose
// content starts with @trokky/. Like codemod.mjs, we rewrite any such literal:
// in site code a "@trokky/..." string is essentially always a specifier, so
// this covers static/side-effect imports, re-exports, require(), dynamic
// import() and `declare module` in one pass while leaving unquoted prose and
// comments alone.
var quotedSpecifier = regexp.MustCompile(`'(@trokky/[^'\n]*)'|"(@trokky/[^"\n]*)"`)

// RewriteSource rewrites every quoted @trokky/* specifier in source text.
// hits counts the rewrites keyed by the old (From) package name.
func RewriteSource(src string) (out string, hits map[string]int) {
	hits = map[string]int{}
	out = quotedSpecifier.ReplaceAllStringFunc(src, func(lit string) string {
		quote := lit[:1]
		spec := lit[1 : len(lit)-1]
		from := matchedMapping(spec)
		if from == "" {
			return lit
		}
		rewritten, _ := RewriteSpecifier(spec)
		hits[from]++
		return quote + rewritten + quote
	})
	return out, hits
}

var sourceExts = map[string]bool{
	".ts":    true,
	".tsx":   true,
	".js":    true,
	".jsx":   true,
	".mjs":   true,
	".cjs":   true,
	".astro": true,
}

// IsSourceFile reports whether name looks like a JS/TS/Astro source file.
func IsSourceFile(name string) bool {
	return sourceExts[strings.ToLower(filepath.Ext(name))]
}

var skippedDirs = map[string]bool{
	"node_modules": true,
	".git":         true,
	"dist":         true,
	"build":        true,
	".astro":       true,
}

// SkipDir reports whether a directory with this base name should not be walked.
func SkipDir(name string) bool { return skippedDirs[name] }
