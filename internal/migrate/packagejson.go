package migrate

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// TargetVersion is the version range the consolidated v2 packages are pinned to.
const TargetVersion = "^2.0.0"

// consolidatedPackages are the packages that survive into v2 and must be moved
// to TargetVersion wherever they appear.
var consolidatedPackages = map[string]bool{
	"@trokky/trokky": true,
	"@trokky/studio": true,
	"@trokky/client": true,
}

var depSections = []string{"dependencies", "devDependencies", "peerDependencies"}

var (
	// e.g. `  "dependencies": {`
	sectionOpenRe = regexp.MustCompile(`^(\s*)"([A-Za-z]+)"\s*:\s*\{\s*$`)
	// e.g. `    "@trokky/core": "^0.1.4",`
	depEntryRe = regexp.MustCompile(`^(\s*)"([^"]+)"\s*:\s*"([^"]*)"\s*,?\s*$`)
	// e.g. `  },`
	sectionCloseRe = regexp.MustCompile(`^\s*\}\s*,?\s*$`)
)

func isOldPackage(name string) bool {
	for _, m := range Mappings {
		if name == m.From {
			return true
		}
	}
	return false
}

// RewritePackageJSON upgrades the dependency sections of a package.json: old
// split packages are removed, @trokky/trokky is added where they were removed
// from (see below), and the surviving @trokky packages are pinned to
// TargetVersion.
//
// @trokky/trokky is added at most once, and only when an old package was
// actually removed: into "dependencies" if anything was removed there, else
// into "devDependencies", never into "peerDependencies", and never when the
// file already lists @trokky/trokky. A frontend-only workspace that just
// depends on @trokky/client therefore only gets its version bumped.
//
// Key order, the file's own indentation and its trailing newline are preserved;
// nothing outside the touched dependency lines is reformatted. If the file has
// no @trokky dependencies at all, src is returned byte-identical with no
// changes.
func RewritePackageJSON(src []byte) (out []byte, changes []string, err error) {
	var probe any
	if err := json.Unmarshal(src, &probe); err != nil {
		return nil, nil, fmt.Errorf("parse package.json: %w", err)
	}

	lines := strings.Split(string(src), "\n")
	sections, err := findSections(lines)
	if err != nil {
		return nil, nil, err
	}

	// The section scan is line-based, so a dependency block written on a single line
	// (or otherwise compacted) matches nothing. Left alone that returns the file
	// byte-identical with no error: the sources get rewritten, the manifest keeps asking
	// for packages that no longer exist, and the next install fails on the deploy host.
	// Cross-check against the parsed JSON and refuse rather than silently do nothing.
	if err := verifySectionsSeen(src, lines, sections); err != nil {
		return nil, nil, err
	}

	// Nothing to do unless some section mentions a @trokky package.
	any, hasTrokky := false, false
	removals := map[string]bool{}
	for _, s := range sections {
		for _, e := range collectEntries(lines, s) {
			if strings.HasPrefix(e.key, "@trokky/") {
				any = true
			}
			if e.key == "@trokky/trokky" {
				hasTrokky = true
			}
			if isOldPackage(e.key) {
				removals[s.name] = true
			}
		}
	}
	changes = append(changes, pinNotes(src)...)

	if !any {
		// Stale pins still have to reach the report even when no dependency changed.
		return src, changes, nil
	}

	// @trokky/trokky replaces the packages we remove, so it is only added
	// where something was actually removed: "dependencies" when anything went
	// from there, otherwise "devDependencies" (a types-only dependency), and
	// never into "peerDependencies". A file that already has it keeps its one
	// entry wherever it lives.
	addTo := ""
	switch {
	case hasTrokky:
	case removals["dependencies"]:
		addTo = "dependencies"
	case removals["devDependencies"]:
		addTo = "devDependencies"
	}

	// Rewrite bottom-up so earlier section line ranges stay valid.
	for i := len(sections) - 1; i >= 0; i-- {
		s := sections[i]
		newLines, secChanges := rewriteSection(lines, s, addTo == s.name)
		lines = append(append(append([]string{}, lines[:s.start]...), newLines...), lines[s.end+1:]...)
		changes = append(secChanges, changes...)
	}

	out = []byte(strings.Join(lines, "\n"))
	if err := json.Unmarshal(out, &probe); err != nil {
		return nil, nil, fmt.Errorf("rewritten package.json is not valid JSON: %w", err)
	}
	return out, changes, nil
}

// section is a dependency block: lines[start] is the `"deps": {` line and
// lines[end] is the closing `}` line.
type section struct {
	name       string
	start, end int
}

type entry struct {
	line    int
	indent  string
	key     string
	version string
}

func findSections(lines []string) ([]section, error) {
	var found []section
	wanted := map[string]bool{}
	for _, n := range depSections {
		wanted[n] = true
	}
	for i, l := range lines {
		m := sectionOpenRe.FindStringSubmatch(l)
		if m == nil || !wanted[m[2]] {
			continue
		}
		end := -1
		for j := i + 1; j < len(lines); j++ {
			if sectionCloseRe.MatchString(lines[j]) {
				end = j
				break
			}
			if !depEntryRe.MatchString(lines[j]) && strings.TrimSpace(lines[j]) != "" {
				return nil, fmt.Errorf(`unsupported %q formatting at line %d: %q`, m[2], j+1, lines[j])
			}
		}
		if end < 0 {
			return nil, fmt.Errorf(`unterminated %q object`, m[2])
		}
		found = append(found, section{name: m[2], start: i, end: end})
	}
	return found, nil
}

func collectEntries(lines []string, s section) []entry {
	var es []entry
	for i := s.start + 1; i < s.end; i++ {
		if m := depEntryRe.FindStringSubmatch(lines[i]); m != nil {
			es = append(es, entry{line: i, indent: m[1], key: m[2], version: m[3]})
		}
	}
	return es
}

// rewriteSection returns the replacement lines for a whole section, including
// its opening and closing lines, plus a human-readable change list. addTrokky
// asks for a @trokky/trokky entry to be inserted in this section; the caller
// decides which section that is.
func rewriteSection(lines []string, s section, addTrokky bool) ([]string, []string) {
	entries := collectEntries(lines, s)

	indent := "  "
	if len(entries) > 0 {
		indent = entries[0].indent
	}
	format := func(key, version string) string {
		return fmt.Sprintf("%s%q: %q", indent, key, version)
	}

	var (
		body      []string
		changes   []string
		isEntry   []bool
		inserted  bool
		addedLine = format("@trokky/trokky", TargetVersion)
	)
	emit := func(line string, entryLine bool) {
		body = append(body, line)
		isEntry = append(isEntry, entryLine)
	}

	for i := s.start + 1; i < s.end; i++ {
		m := depEntryRe.FindStringSubmatch(lines[i])
		if m == nil {
			emit(lines[i], false)
			continue
		}
		key, version := m[2], m[3]
		if isOldPackage(key) {
			changes = append(changes, "removed "+key)
			if addTrokky && !inserted {
				emit(addedLine, true)
				changes = append(changes, "added @trokky/trokky "+TargetVersion)
				inserted = true
			}
			continue
		}
		if consolidatedPackages[key] && version != TargetVersion {
			changes = append(changes, key+" -> "+TargetVersion)
			version = TargetVersion
		}
		emit(format(key, version), true)
	}
	if addTrokky && !inserted {
		emit(addedLine, true)
		changes = append(changes, "added @trokky/trokky "+TargetVersion)
	}

	// Re-apply commas: every entry but the last one in the object gets one.
	last := -1
	for i, e := range isEntry {
		if e {
			last = i
		}
	}
	for i, e := range isEntry {
		if e && i != last {
			body[i] += ","
		}
	}

	// An emptied section collapses onto one line rather than leaving a bare
	// `{` / `}` pair behind. The opening line's indentation and the closing
	// line's trailing comma are kept.
	if last < 0 && len(entries) > 0 {
		open := sectionOpenRe.FindStringSubmatch(lines[s.start])
		collapsed := fmt.Sprintf("%s%q: {}", open[1], s.name)
		if strings.HasSuffix(strings.TrimSpace(lines[s.end]), ",") {
			collapsed += ","
		}
		return []string{collapsed}, changes
	}

	return append(append([]string{lines[s.start]}, body...), lines[s.end]), changes
}

// pinSections are the blocks npm, yarn and pnpm use to force a version. The codemod does
// not rewrite them (their semantics differ per package manager), but a stale pin on a
// package that no longer exists breaks installs, so it must not pass unmentioned.
var pinSections = []string{"resolutions", "overrides"}

// verifySectionsSeen reports an error when the parsed JSON holds @trokky entries in a
// dependency section that the line scanner did not pick up.
func verifySectionsSeen(src []byte, lines []string, sections []section) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(src, &raw); err != nil {
		return nil // not an object; the caller already validated it parses
	}

	seen := map[string]int{}
	for _, s := range sections {
		for _, e := range collectEntries(lines, s) {
			if strings.HasPrefix(e.key, "@trokky/") {
				seen[s.name]++
			}
		}
	}

	for _, name := range depSections {
		blob, ok := raw[name]
		if !ok {
			continue
		}
		var deps map[string]string
		if json.Unmarshal(blob, &deps) != nil {
			continue
		}
		count := 0
		for key := range deps {
			if strings.HasPrefix(key, "@trokky/") {
				count++
			}
		}
		if count > seen[name] {
			return fmt.Errorf(
				"%q holds %d @trokky dependencies but only %d could be read: this package.json is not "+
					"formatted one entry per line, so it cannot be rewritten safely — reformat it "+
					"(npm pkg fix, or a JSON formatter) and run again",
				name, count, seen[name])
		}
	}
	return nil
}

// pinNotes returns a note for every stale @trokky pin in resolutions/overrides, which the
// codemod deliberately does not rewrite.
func pinNotes(src []byte) []string {
	var raw map[string]json.RawMessage
	if json.Unmarshal(src, &raw) != nil {
		return nil
	}
	var notes []string
	for _, name := range pinSections {
		blob, ok := raw[name]
		if !ok {
			continue
		}
		var pins map[string]json.RawMessage
		if json.Unmarshal(blob, &pins) != nil {
			continue
		}
		for _, key := range sortedKeys(pins) {
			if isOldPackage(key) {
				notes = append(notes, name+" still pins "+key+" (not rewritten, edit by hand)")
			}
		}
	}
	return notes
}

func sortedKeys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
