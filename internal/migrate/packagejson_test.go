package migrate

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func mustValidJSON(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, b)
	}
	return v
}

func TestRewritePackageJSONBasic(t *testing.T) {
	src := []byte(`{
  "name": "my-site",
  "version": "1.0.0",
  "type": "module",
  "scripts": {
    "dev": "tsx server.ts"
  },
  "dependencies": {
    "express": "^4.19.2",
    "@trokky/core": "^0.1.4",
    "@trokky/express": "^0.1.4",
    "@trokky/studio": "^0.1.4",
    "zod": "^3.23.8"
  },
  "devDependencies": {
    "@trokky/types": "^0.1.4",
    "typescript": "^5.5.0"
  }
}
`)
	want := `{
  "name": "my-site",
  "version": "1.0.0",
  "type": "module",
  "scripts": {
    "dev": "tsx server.ts"
  },
  "dependencies": {
    "express": "^4.19.2",
    "@trokky/trokky": "^2.0.0",
    "@trokky/studio": "^2.0.0",
    "zod": "^3.23.8"
  },
  "devDependencies": {
    "typescript": "^5.5.0"
  }
}
`
	out, changes, err := RewritePackageJSON(src)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
	mustValidJSON(t, out)

	wantChanges := []string{
		"removed @trokky/core",
		"added @trokky/trokky ^2.0.0",
		"removed @trokky/express",
		"@trokky/studio -> ^2.0.0",
		"removed @trokky/types",
	}
	if len(changes) != len(wantChanges) {
		t.Fatalf("changes = %v, want %v", changes, wantChanges)
	}
	for i := range changes {
		if changes[i] != wantChanges[i] {
			t.Errorf("changes[%d] = %q, want %q", i, changes[i], wantChanges[i])
		}
	}
}

func TestRewritePackageJSONInsertsAtFirstRemoval(t *testing.T) {
	src := []byte(`{
  "dependencies": {
    "a": "1",
    "@trokky/mail-adapter-console": "^0.1.0",
    "b": "2",
    "@trokky/core": "^0.1.0",
    "c": "3"
  }
}
`)
	out, _, err := RewritePackageJSON(src)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "dependencies": {
    "a": "1",
    "@trokky/trokky": "^2.0.0",
    "b": "2",
    "c": "3"
  }
}
`
	if string(out) != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestRewritePackageJSONNoAddWhenNothingRemoved(t *testing.T) {
	// A frontend-only workspace: @trokky/client is bumped, but nothing was
	// removed, so @trokky/trokky must not appear.
	src := []byte(`{
  "name": "web",
  "dependencies": {
    "@trokky/client": "^0.1.4",
    "astro": "^4.15.0"
  }
}
`)
	out, changes, err := RewritePackageJSON(src)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "name": "web",
  "dependencies": {
    "@trokky/client": "^2.0.0",
    "astro": "^4.15.0"
  }
}
`
	if string(out) != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
	if len(changes) != 1 || changes[0] != "@trokky/client -> ^2.0.0" {
		t.Errorf("changes = %v, want only the client bump", changes)
	}
	if strings.Contains(string(out), "@trokky/trokky") {
		t.Error("@trokky/trokky must not be added when nothing was removed")
	}
}

func TestRewritePackageJSONAddsToDevDependenciesOnly(t *testing.T) {
	// The only old package is a types-only devDependency, so the replacement
	// belongs in devDependencies, not dependencies.
	src := []byte(`{
  "dependencies": {
    "@trokky/client": "^0.1.4",
    "astro": "^4.15.0"
  },
  "devDependencies": {
    "@trokky/types": "^0.1.4",
    "typescript": "^5.5.0"
  }
}
`)
	out, changes, err := RewritePackageJSON(src)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "dependencies": {
    "@trokky/client": "^2.0.0",
    "astro": "^4.15.0"
  },
  "devDependencies": {
    "@trokky/trokky": "^2.0.0",
    "typescript": "^5.5.0"
  }
}
`
	if string(out) != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
	wantChanges := []string{"@trokky/client -> ^2.0.0", "removed @trokky/types", "added @trokky/trokky ^2.0.0"}
	if len(changes) != len(wantChanges) {
		t.Fatalf("changes = %v, want %v", changes, wantChanges)
	}
	for i := range wantChanges {
		if changes[i] != wantChanges[i] {
			t.Errorf("changes[%d] = %q, want %q", i, changes[i], wantChanges[i])
		}
	}
}

func TestRewritePackageJSONAddsOnceWhenBothSectionsRemove(t *testing.T) {
	src := []byte(`{
  "dependencies": {
    "@trokky/core": "^0.1.4",
    "express": "^4.19.2"
  },
  "devDependencies": {
    "@trokky/types": "^0.1.4",
    "typescript": "^5.5.0"
  }
}
`)
	out, _, err := RewritePackageJSON(src)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "dependencies": {
    "@trokky/trokky": "^2.0.0",
    "express": "^4.19.2"
  },
  "devDependencies": {
    "typescript": "^5.5.0"
  }
}
`
	if string(out) != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
	if n := strings.Count(string(out), `"@trokky/trokky"`); n != 1 {
		t.Errorf("@trokky/trokky appears %d times, want 1", n)
	}
}

func TestRewritePackageJSONNoAddWhenTrokkyAlreadyPresentElsewhere(t *testing.T) {
	src := []byte(`{
  "dependencies": {
    "@trokky/core": "^0.1.4"
  },
  "devDependencies": {
    "@trokky/trokky": "^1.9.0"
  }
}
`)
	out, _, err := RewritePackageJSON(src)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "dependencies": {},
  "devDependencies": {
    "@trokky/trokky": "^2.0.0"
  }
}
`
	if string(out) != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestRewritePackageJSONNeverAddsToPeerDependencies(t *testing.T) {
	src := []byte(`{
  "peerDependencies": {
    "@trokky/core": "^0.1.4",
    "react": "^18.0.0"
  }
}
`)
	out, _, err := RewritePackageJSON(src)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "peerDependencies": {
    "react": "^18.0.0"
  }
}
`
	if string(out) != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

func TestRewritePackageJSONCollapsesEmptiedSection(t *testing.T) {
	src := []byte(`{
  "name": "site",
  "dependencies": {
    "@trokky/core": "^0.1.4"
  },
  "devDependencies": {
    "@trokky/types": "^0.1.4",
    "@trokky/fields": "^0.1.4"
  },
  "private": true
}
`)
	out, _, err := RewritePackageJSON(src)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "name": "site",
  "dependencies": {
    "@trokky/trokky": "^2.0.0"
  },
  "devDependencies": {},
  "private": true
}
`
	if string(out) != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
	mustValidJSON(t, out)
}

func TestRewritePackageJSONCollapsesLastSectionWithoutComma(t *testing.T) {
	// The emptied section is the last key of the object, so its closing line
	// has no trailing comma and neither may the collapsed line. 4-space
	// indentation is preserved.
	src := []byte("{\n" +
		"    \"dependencies\": {\n" +
		"        \"@trokky/core\": \"^0.1.4\"\n" +
		"    },\n" +
		"    \"peerDependencies\": {\n" +
		"        \"@trokky/fields\": \"^0.1.4\"\n" +
		"    }\n" +
		"}\n")
	out, _, err := RewritePackageJSON(src)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n" +
		"    \"dependencies\": {\n" +
		"        \"@trokky/trokky\": \"^2.0.0\"\n" +
		"    },\n" +
		"    \"peerDependencies\": {}\n" +
		"}\n"
	if string(out) != want {
		t.Errorf("got:\n%q\nwant:\n%q", out, want)
	}
	mustValidJSON(t, out)
}

func TestRewritePackageJSONExistingTrokkyBumped(t *testing.T) {
	src := []byte(`{
  "dependencies": {
    "@trokky/trokky": "^1.9.0",
    "@trokky/core": "^0.1.0"
  }
}
`)
	out, changes, err := RewritePackageJSON(src)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "dependencies": {
    "@trokky/trokky": "^2.0.0"
  }
}
`
	if string(out) != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
	if len(changes) != 2 || changes[0] != "@trokky/trokky -> ^2.0.0" || changes[1] != "removed @trokky/core" {
		t.Errorf("changes = %v", changes)
	}
}

func TestRewritePackageJSONPeerDependencies(t *testing.T) {
	src := []byte(`{
  "peerDependencies": {
    "@trokky/fields": "^0.1.0",
    "react": "^18.0.0"
  }
}
`)
	out, changes, err := RewritePackageJSON(src)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "peerDependencies": {
    "react": "^18.0.0"
  }
}
`
	if string(out) != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
	if len(changes) != 1 || changes[0] != "removed @trokky/fields" {
		t.Errorf("changes = %v", changes)
	}
}

func TestRewritePackageJSONPreservesIndentation(t *testing.T) {
	t.Run("four spaces", func(t *testing.T) {
		src := []byte("{\n" +
			"    \"name\": \"x\",\n" +
			"    \"dependencies\": {\n" +
			"        \"@trokky/core\": \"^0.1.0\",\n" +
			"        \"express\": \"^4.19.2\"\n" +
			"    },\n" +
			"    \"private\": true\n" +
			"}\n")
		want := "{\n" +
			"    \"name\": \"x\",\n" +
			"    \"dependencies\": {\n" +
			"        \"@trokky/trokky\": \"^2.0.0\",\n" +
			"        \"express\": \"^4.19.2\"\n" +
			"    },\n" +
			"    \"private\": true\n" +
			"}\n"
		out, _, err := RewritePackageJSON(src)
		if err != nil {
			t.Fatal(err)
		}
		if string(out) != want {
			t.Errorf("got:\n%q\nwant:\n%q", out, want)
		}
		mustValidJSON(t, out)
	})

	t.Run("tabs", func(t *testing.T) {
		src := []byte("{\n" +
			"\t\"name\": \"x\",\n" +
			"\t\"dependencies\": {\n" +
			"\t\t\"@trokky/core\": \"^0.1.0\",\n" +
			"\t\t\"express\": \"^4.19.2\"\n" +
			"\t},\n" +
			"\t\"private\": true\n" +
			"}\n")
		want := "{\n" +
			"\t\"name\": \"x\",\n" +
			"\t\"dependencies\": {\n" +
			"\t\t\"@trokky/trokky\": \"^2.0.0\",\n" +
			"\t\t\"express\": \"^4.19.2\"\n" +
			"\t},\n" +
			"\t\"private\": true\n" +
			"}\n"
		out, _, err := RewritePackageJSON(src)
		if err != nil {
			t.Fatal(err)
		}
		if string(out) != want {
			t.Errorf("got:\n%q\nwant:\n%q", out, want)
		}
		mustValidJSON(t, out)
	})
}

func TestRewritePackageJSONKeyOrderPreserved(t *testing.T) {
	src := []byte(`{
  "name": "z-first",
  "version": "1.0.0",
  "dependencies": {
    "zod": "^3.23.8",
    "@trokky/core": "^0.1.0",
    "express": "^4.19.2",
    "aaa": "1.0.0"
  },
  "license": "MIT"
}
`)
	out, _, err := RewritePackageJSON(src)
	if err != nil {
		t.Fatal(err)
	}
	mustValidJSON(t, out)
	got := string(out)
	for _, pair := range [][2]string{
		{`"name"`, `"version"`},
		{`"version"`, `"dependencies"`},
		{`"zod"`, `"@trokky/trokky"`},
		{`"@trokky/trokky"`, `"express"`},
		{`"express"`, `"aaa"`},
		{`"aaa"`, `"license"`},
	} {
		if strings.Index(got, pair[0]) > strings.Index(got, pair[1]) {
			t.Errorf("key order broken: %s should precede %s\n%s", pair[0], pair[1], got)
		}
	}
}

func TestRewritePackageJSONNoTrokkyDepsUnchanged(t *testing.T) {
	src := []byte(`{
  "name": "plain",
  "dependencies": {
    "express": "^4.19.2"
  },
  "devDependencies": {
    "typescript": "^5.5.0"
  }
}
`)
	out, changes, err := RewritePackageJSON(src)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, src) {
		t.Errorf("file changed:\n%s", out)
	}
	if len(changes) != 0 {
		t.Errorf("changes = %v, want none", changes)
	}
}

func TestRewritePackageJSONNoDepsAtAll(t *testing.T) {
	src := []byte("{\n  \"name\": \"plain\"\n}\n")
	out, changes, err := RewritePackageJSON(src)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, src) || len(changes) != 0 {
		t.Errorf("out=%q changes=%v", out, changes)
	}
}

func TestRewritePackageJSONNoTrailingNewline(t *testing.T) {
	src := []byte("{\n  \"dependencies\": {\n    \"@trokky/core\": \"^0.1.0\"\n  }\n}")
	out, _, err := RewritePackageJSON(src)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"dependencies\": {\n    \"@trokky/trokky\": \"^2.0.0\"\n  }\n}"
	if string(out) != want {
		t.Errorf("got %q, want %q", out, want)
	}
}

func TestRewritePackageJSONInvalid(t *testing.T) {
	if _, _, err := RewritePackageJSON([]byte("{ not json")); err == nil {
		t.Fatal("expected an error for invalid JSON")
	}
}
