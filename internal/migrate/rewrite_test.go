package migrate

import (
	"strings"
	"testing"
)

func TestMappingsLongestFirst(t *testing.T) {
	for i := 1; i < len(Mappings); i++ {
		prev, cur := Mappings[i-1].From, Mappings[i].From
		if len(cur) > len(prev) {
			t.Fatalf("Mappings not sorted longest-From-first: %q (len %d) after %q (len %d)",
				cur, len(cur), prev, len(prev))
		}
	}
}

func TestMappingsUnique(t *testing.T) {
	seen := map[string]bool{}
	for _, m := range Mappings {
		if seen[m.From] {
			t.Errorf("duplicate mapping for %q", m.From)
		}
		seen[m.From] = true
	}
	for _, keep := range []string{"@trokky/client", "@trokky/studio", "@trokky/trokky"} {
		if seen[keep] {
			t.Errorf("%q must not be mapped away", keep)
		}
	}
}

func TestRewriteSpecifier(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		// one row per mapping, exact...
		{"@trokky/adapter-filesystem-media", "@trokky/trokky/adapters/filesystem-media", true},
		{"@trokky/adapter-filesystem-data", "@trokky/trokky/adapters/filesystem-data", true},
		{"@trokky/adapter-postgres-data", "@trokky/trokky/adapters/postgres-data", true},
		{"@trokky/adapter-filesystem", "@trokky/trokky/adapters/filesystem-data", true},
		{"@trokky/mail-adapter-console", "@trokky/trokky/mail/console", true},
		{"@trokky/mail-adapter-resend", "@trokky/trokky/mail/resend", true},
		{"@trokky/mail-adapter-smtp", "@trokky/trokky/mail/smtp", true},
		{"@trokky/structure", "@trokky/trokky/structure", true},
		{"@trokky/express", "@trokky/trokky/express", true},
		{"@trokky/routes", "@trokky/trokky", true},
		{"@trokky/types", "@trokky/trokky/types", true},
		{"@trokky/core", "@trokky/trokky", true},
		{"@trokky/mail", "@trokky/trokky/mail", true},
		{"@trokky/i18n", "@trokky/trokky/i18n", true},
		{"@trokky/fields", "@trokky/studio", true},

		// ...and with a /sub suffix.
		{"@trokky/adapter-filesystem-media/foo", "@trokky/trokky/adapters/filesystem-media/foo", true},
		{"@trokky/adapter-filesystem-data/foo", "@trokky/trokky/adapters/filesystem-data/foo", true},
		{"@trokky/adapter-postgres-data/foo", "@trokky/trokky/adapters/postgres-data/foo", true},
		{"@trokky/adapter-filesystem/foo", "@trokky/trokky/adapters/filesystem-data/foo", true},
		{"@trokky/mail-adapter-console/foo", "@trokky/trokky/mail/console/foo", true},
		{"@trokky/mail-adapter-resend/foo", "@trokky/trokky/mail/resend/foo", true},
		{"@trokky/mail-adapter-smtp/foo", "@trokky/trokky/mail/smtp/foo", true},
		{"@trokky/structure/foo", "@trokky/trokky/structure/foo", true},
		{"@trokky/express/foo", "@trokky/trokky/express/foo", true},
		{"@trokky/routes/foo", "@trokky/trokky/foo", true},
		{"@trokky/types/foo", "@trokky/trokky/types/foo", true},
		{"@trokky/core/foo", "@trokky/trokky/foo", true},
		{"@trokky/mail/foo", "@trokky/trokky/mail/foo", true},
		{"@trokky/i18n/foo", "@trokky/trokky/i18n/foo", true},
		{"@trokky/fields/foo", "@trokky/studio/foo", true},

		// longest match wins, not the shorter prefix package
		{"@trokky/mail-adapter-console", "@trokky/trokky/mail/console", true},

		// substrings of longer names are untouched
		{"@trokky/mailer", "@trokky/mailer", false},
		{"@trokky/core-extra", "@trokky/core-extra", false},
		{"@trokky/typesx", "@trokky/typesx", false},
		{"@trokky/expressive", "@trokky/expressive", false},

		// unchanged packages
		{"@trokky/client", "@trokky/client", false},
		{"@trokky/studio", "@trokky/studio", false},
		{"@trokky/studio/react", "@trokky/studio/react", false},
		{"@trokky/trokky", "@trokky/trokky", false},
		{"express", "express", false},
	}
	for _, c := range cases {
		got, ok := RewriteSpecifier(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("RewriteSpecifier(%q) = (%q, %v), want (%q, %v)", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestRewriteSourceMixed(t *testing.T) {
	src := `import { defineConfig } from '@trokky/core'
import express from "@trokky/express"
export { thing } from '@trokky/types/schema'
import '@trokky/core/register'
const routes = require('@trokky/routes')
const mail = await import("@trokky/mail-adapter-console")
// @trokky/core in a comment without quotes stays
/* prose about @trokky/mail here */
const untouched = '@trokky/mailer'
const keep = '@trokky/client'
`
	want := `import { defineConfig } from '@trokky/trokky'
import express from "@trokky/trokky/express"
export { thing } from '@trokky/trokky/types/schema'
import '@trokky/trokky/register'
const routes = require('@trokky/trokky')
const mail = await import("@trokky/trokky/mail/console")
// @trokky/core in a comment without quotes stays
/* prose about @trokky/mail here */
const untouched = '@trokky/mailer'
const keep = '@trokky/client'
`
	out, hits := RewriteSource(src)
	if out != want {
		t.Errorf("RewriteSource mismatch:\n got:\n%s\nwant:\n%s", out, want)
	}
	wantHits := map[string]int{
		"@trokky/core":                 2,
		"@trokky/express":              1,
		"@trokky/types":                1,
		"@trokky/routes":               1,
		"@trokky/mail-adapter-console": 1,
	}
	if len(hits) != len(wantHits) {
		t.Fatalf("hits = %v, want %v", hits, wantHits)
	}
	for k, v := range wantHits {
		if hits[k] != v {
			t.Errorf("hits[%q] = %d, want %d (all: %v)", k, hits[k], v, hits)
		}
	}
	if strings.Contains(out, "@trokky/trokky/mail-adapter-console") {
		t.Error("mail-adapter-console was matched by the shorter @trokky/mail mapping")
	}
}

func TestRewriteSourceDeclareModule(t *testing.T) {
	src := "declare module '@trokky/core' {\n  export const x: number\n}\n"
	want := "declare module '@trokky/trokky' {\n  export const x: number\n}\n"
	out, hits := RewriteSource(src)
	if out != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if hits["@trokky/core"] != 1 {
		t.Errorf("hits = %v", hits)
	}
}

func TestRewriteSourceNoChange(t *testing.T) {
	src := "import { c } from '@trokky/client'\n// see @trokky/core docs\n"
	out, hits := RewriteSource(src)
	if out != src {
		t.Errorf("source changed unexpectedly: %q", out)
	}
	if len(hits) != 0 {
		t.Errorf("hits = %v, want empty", hits)
	}
}

func TestIsSourceFile(t *testing.T) {
	yes := []string{"a.ts", "a.tsx", "a.js", "a.jsx", "a.mjs", "a.cjs", "a.astro", "types.d.ts"}
	no := []string{"package.json", "README.md", "a.css", "a", "a.tsxx"}
	for _, n := range yes {
		if !IsSourceFile(n) {
			t.Errorf("IsSourceFile(%q) = false, want true", n)
		}
	}
	for _, n := range no {
		if IsSourceFile(n) {
			t.Errorf("IsSourceFile(%q) = true, want false", n)
		}
	}
}

func TestSkipDir(t *testing.T) {
	for _, n := range []string{"node_modules", ".git", "dist", "build", ".astro"} {
		if !SkipDir(n) {
			t.Errorf("SkipDir(%q) = false, want true", n)
		}
	}
	for _, n := range []string{"src", "public", "distinct", ""} {
		if SkipDir(n) {
			t.Errorf("SkipDir(%q) = true, want false", n)
		}
	}
}
