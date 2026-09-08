package migrate

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
)

// Options configures a migration run.
type Options struct {
	// Root is the project directory to walk.
	Root string
	// Write makes the run rewrite files in place. When false the run is a
	// dry run and no file is ever touched.
	Write bool
}

// FileChange is one file the migration would change (or did change).
type FileChange struct {
	// Path is relative to Options.Root and slash-separated.
	Path string
	// Kind is "source" or "package.json".
	Kind string
	// Hits counts rewritten specifiers by old package name (source files).
	Hits map[string]int
	// Changes is the human-readable change list (package.json files).
	Changes []string
}

// Result is the summary of a migration run.
type Result struct {
	// Files are the changed files in deterministic (lexical walk) order.
	Files []FileChange
	// Hits totals the rewritten specifiers by old package name.
	Hits map[string]int
	// Scanned is the number of files examined.
	Scanned int
	// Warnings are the things the codemod cannot fix by itself. They are
	// collected before any file is written, so their messages quote the
	// original specifiers.
	Warnings []Warning
}

// Kinds of FileChange.
const (
	KindSource      = "source"
	KindPackageJSON = "package.json"
)

// Run walks opts.Root and applies the v0.1 -> v2 codemod to every source file
// and package.json it finds, skipping node_modules, dist, build, .git, .astro
// and symlinks. Files are only written back when opts.Write is true.
func Run(opts Options) (*Result, error) {
	res := &Result{Hits: map[string]int{}}

	// Scan for warnings first: once files are rewritten the sources no longer
	// mention the old specifiers the messages point at.
	warnings, err := ScanWarnings(opts.Root)
	if err != nil {
		return nil, err
	}
	res.Warnings = warnings

	err = filepath.WalkDir(opts.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != opts.Root && SkipDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		// Never follow or rewrite symlinks or other non-regular files.
		if !d.Type().IsRegular() {
			return nil
		}
		name := d.Name()
		isPkg := name == "package.json"
		if !isPkg && !IsSourceFile(name) {
			return nil
		}
		res.Scanned++

		before, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}

		var (
			after   []byte
			change  FileChange
			changed bool
		)
		if isPkg {
			out, changes, err := RewritePackageJSON(before)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			after = out
			change = FileChange{Kind: KindPackageJSON, Changes: changes}
			changed = !bytes.Equal(before, after)
		} else {
			out, hits := RewriteSource(string(before))
			after = []byte(out)
			change = FileChange{Kind: KindSource, Hits: hits}
			changed = !bytes.Equal(before, after)
		}
		if !changed {
			return nil
		}

		rel, err := filepath.Rel(opts.Root, path)
		if err != nil {
			return err
		}
		change.Path = filepath.ToSlash(rel)
		res.Files = append(res.Files, change)
		for from, n := range change.Hits {
			res.Hits[from] += n
		}

		if opts.Write {
			info, err := d.Info()
			if err != nil {
				return fmt.Errorf("stat %s: %w", path, err)
			}
			if err := os.WriteFile(path, after, info.Mode().Perm()); err != nil {
				return fmt.Errorf("write %s: %w", path, err)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// Report writes the human-readable migration report: the changed files with
// what changed in each, the per-mapping rewrite counts, and the warnings.
// dryRun only affects the wording.
func (r *Result) Report(w io.Writer, dryRun bool) {
	header := "Files changed"
	if dryRun {
		header = "Files that would change"
	}
	fmt.Fprintf(w, "%s (%d):\n", header, len(r.Files))

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, f := range r.Files {
		fmt.Fprintf(tw, "  %s\t%s\n", f.Path, f.detail())
	}
	_ = tw.Flush()

	var rows []Mapping
	for _, m := range Mappings {
		if r.Hits[m.From] > 0 {
			rows = append(rows, m)
		}
	}
	if len(rows) > 0 {
		fmt.Fprintln(w, "Specifiers rewritten by mapping:")
		tw = tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		for _, m := range rows {
			fmt.Fprintf(tw, "  %s\t-> %s\t%d\n", m.From, m.To, r.Hits[m.From])
		}
		_ = tw.Flush()
	}

	if len(r.Warnings) == 0 {
		fmt.Fprintln(w, "No warnings.")
		return
	}

	fmt.Fprintf(w, "WARNINGS (%d):\n", len(r.Warnings))
	divergence := false
	for _, warn := range r.Warnings {
		if warn.Kind == "singleton-divergence" {
			divergence = true
		}
		loc := warn.File
		if warn.Line > 0 {
			loc = fmt.Sprintf("%s:%d", warn.File, warn.Line)
		}
		if loc != "" {
			loc += ": "
		}
		fmt.Fprintf(w, "  [%s] %s%s\n", warn.Kind, loc, warn.Message)
	}
	if divergence {
		fmt.Fprintln(w, "Fix the singleton warnings BEFORE running `trokky restore` against the migrated site.")
	}
}

// detail describes one changed file in a single line.
func (f FileChange) detail() string {
	if f.Kind == KindPackageJSON {
		return strings.Join(f.Changes, ", ")
	}
	var parts []string
	for _, m := range Mappings {
		if n := f.Hits[m.From]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s x%d", m.From, n))
		}
	}
	return strings.Join(parts, ", ")
}

// Summary renders a one-line description of what a run found.
func (r *Result) Summary() string {
	var parts []string
	for _, m := range Mappings {
		if n := r.Hits[m.From]; n > 0 {
			parts = append(parts, fmt.Sprintf("%s x%d", m.From, n))
		}
	}
	s := fmt.Sprintf("%d file(s) changed of %d scanned", len(r.Files), r.Scanned)
	if len(parts) > 0 {
		s += ": " + strings.Join(parts, ", ")
	}
	return s
}
