package cmd

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/trokky/cli/internal/migrate"
)

func newMigrateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Upgrade a Trokky project from v0.1.x packages to v2",
		Long: `Rewrite a Trokky site project from the old split v0.1.x npm packages to the
consolidated v2 packages.

The command works purely on the files in --path: it needs no running instance,
no URL and no token. It rewrites every quoted @trokky/* module specifier in the
project's JS/TS/Astro sources (@trokky/core -> @trokky/trokky,
@trokky/express -> @trokky/trokky/express, @trokky/adapter-* ->
@trokky/trokky/adapters/*, @trokky/mail-adapter-* -> @trokky/trokky/mail/*,
@trokky/fields -> @trokky/studio, ...) and updates the dependency sections of
every package.json, pinning @trokky/trokky, @trokky/studio and @trokky/client
to ^2.0.0. node_modules, dist, build, .git, .astro and symlinks are never
touched.

It also reports what it cannot fix by itself: imports of package internals,
Astro's build-time inlining of import.meta.env.TROKKY_API_URL, and — most
importantly — structure entries marked type: 'singleton' whose schema does not
set singleton: true. That divergence is a data-loss trap: 'trokky restore'
POSTs such a document instead of doing an id-preserving PUT, which regenerates
the document id and orphans the structure entry. Fix those warnings before you
restore into the migrated site.

The default is a dry run. Nothing is written until you pass --write.

Examples:
  trokky migrate --dry-run
  trokky migrate --write
  trokky migrate --path ./site --write --yes`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path, _ := cmd.Flags().GetString("path")
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			write, _ := cmd.Flags().GetBool("write")
			yes, _ := cmd.Flags().GetBool("yes")
			force, _ := cmd.Flags().GetBool("force")

			if dryRun && write {
				return errors.New("--dry-run and --write are mutually exclusive")
			}

			root, err := filepath.Abs(path)
			if err != nil {
				return fmt.Errorf("invalid --path: %w", err)
			}

			out := cmd.OutOrStdout()

			// Before touching anything, refuse to write over a dirty working
			// tree so the user can always diff and revert the migration.
			if write {
				isRepo, dirty, err := gitDirty(root)
				if err != nil {
					return err
				}
				if isRepo && dirty && !force {
					return fmt.Errorf("refusing to write: %s has uncommitted changes; "+
						"commit or stash them so you can diff and revert the migration, or pass --force", root)
				}
			}

			// Always start with a dry run: it produces the report and the file
			// count used by the confirmation prompt.
			res, err := migrate.Run(migrate.Options{Root: root})
			if err != nil {
				return err
			}
			res.Report(out, !write)

			if !write {
				fmt.Fprintf(out, "%d file(s) would change.\n", len(res.Files))
				fmt.Fprintln(out, "Dry run: no files were written. Re-run with --write to apply.")
				return nil
			}

			if len(res.Files) == 0 {
				fmt.Fprintln(out, "Nothing to do.")
				return nil
			}

			if !yes {
				ok, err := confirmPrompt(fmt.Sprintf("Rewrite %d file(s) under %s?", len(res.Files), root))
				if err != nil {
					return err
				}
				if !ok {
					fmt.Fprintln(out, "Aborted.")
					return nil
				}
			}

			written, err := migrate.Run(migrate.Options{Root: root, Write: true})
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "%d file(s) changed.\n", len(written.Files))
			return nil
		},
	}

	cmd.Flags().String("path", ".", "Project directory to migrate")
	cmd.Flags().Bool("dry-run", false, "Report what would change without writing (the default)")
	cmd.Flags().Bool("write", false, "Rewrite the files in place")
	cmd.Flags().BoolP("yes", "y", false, "Skip the confirmation prompt")
	cmd.Flags().Bool("force", false, "Write even when the git working tree is dirty")

	return cmd
}

// gitDirty reports whether dir is inside a git repository and, if so, whether
// that repository's working tree has uncommitted changes (untracked files
// included). A missing git binary is not an error: the directory is simply
// treated as not being a repository.
func gitDirty(dir string) (isRepo bool, dirty bool, err error) {
	if _, err := exec.LookPath("git"); err != nil {
		return false, false, nil
	}

	inside, err := exec.Command("git", "-C", dir, "rev-parse", "--is-inside-work-tree").Output()
	if err != nil {
		// Not a repository (or git refused to look): nothing to protect.
		return false, false, nil
	}
	if strings.TrimSpace(string(inside)) != "true" {
		return false, false, nil
	}

	status, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	if err != nil {
		return true, false, fmt.Errorf("git status failed in %s: %w", dir, err)
	}
	return true, strings.TrimSpace(string(status)) != "", nil
}

func init() {
	rootCmd.AddCommand(newMigrateCmd())
}
