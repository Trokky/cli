package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitRepo makes a temp directory into a git repository with one empty commit.
// It skips the test when git is unavailable.
func gitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	run("init")
	run("-c", "user.email=t@t", "-c", "user.name=t", "commit", "--allow-empty", "-m", "init")
	return dir
}

func TestGitDirtyCleanRepo(t *testing.T) {
	dir := gitRepo(t)

	isRepo, dirty, err := gitDirty(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !isRepo {
		t.Fatal("isRepo = false, want true")
	}
	if dirty {
		t.Error("dirty = true for a fresh repo, want false")
	}
}

func TestGitDirtyUntrackedFile(t *testing.T) {
	dir := gitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "server.ts"), []byte("import '@trokky/core'\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	isRepo, dirty, err := gitDirty(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !isRepo || !dirty {
		t.Errorf("isRepo=%v dirty=%v, want true/true (untracked files count as dirty)", isRepo, dirty)
	}
}

func TestGitDirtyOutsideRepo(t *testing.T) {
	// t.TempDir() is under /var/folders on macOS and /tmp on Linux, neither of
	// which is a repository — unless the test itself is run from one, so only
	// assert that no error escapes.
	if _, _, err := gitDirty(t.TempDir()); err != nil {
		t.Fatalf("gitDirty outside a repo returned an error: %v", err)
	}
}

// runMigrate executes a fresh migrate command with args and returns its output.
func runMigrate(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := newMigrateCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestMigrateDryRunAndWriteAreMutuallyExclusive(t *testing.T) {
	_, err := runMigrate(t, "--path", t.TempDir(), "--dry-run", "--write")
	if err == nil {
		t.Fatal("expected an error when both --dry-run and --write are given")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error = %v, want it to mention mutual exclusion", err)
	}
}

func TestMigrateRefusesDirtyRepo(t *testing.T) {
	dir := gitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "server.ts"), []byte("import '@trokky/core'\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := runMigrate(t, "--path", dir, "--write", "--yes")
	if err == nil {
		t.Fatal("expected a refusal for a dirty working tree")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error = %v, want it to mention --force", err)
	}

	// The file must be untouched.
	got, readErr := os.ReadFile(filepath.Join(dir, "server.ts"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "import '@trokky/core'\n" {
		t.Errorf("server.ts was rewritten despite the refusal: %q", got)
	}
}

func TestMigrateForceWritesDirtyRepo(t *testing.T) {
	dir := gitRepo(t)
	src := filepath.Join(dir, "server.ts")
	if err := os.WriteFile(src, []byte("import '@trokky/core'\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runMigrate(t, "--path", dir, "--write", "--yes", "--force")
	if err != nil {
		t.Fatalf("migrate --force failed: %v\n%s", err, out)
	}
	got, readErr := os.ReadFile(src)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "import '@trokky/trokky'\n" {
		t.Errorf("server.ts = %q, want the rewritten specifier", got)
	}
	if !strings.Contains(out, "1 file(s) changed.") {
		t.Errorf("output = %q, want a file count", out)
	}
}

func TestMigrateDryRunIsDefault(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "server.ts")
	if err := os.WriteFile(src, []byte("import '@trokky/core'\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runMigrate(t, "--path", dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Files that would change (1):",
		"1 file(s) would change.",
		"Dry run: no files were written. Re-run with --write to apply.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output is missing %q:\n%s", want, out)
		}
	}
	got, readErr := os.ReadFile(src)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "import '@trokky/core'\n" {
		t.Errorf("the default run wrote to server.ts: %q", got)
	}
}
