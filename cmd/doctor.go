package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"
	"github.com/trokky/cli/internal/client"
)

// doctorReport is GET /system/doctor (Trokky 3.6 and later).
type doctorReport struct {
	Environment string        `json:"environment"`
	Status      string        `json:"status"`
	CheckedAt   string        `json:"checkedAt"`
	Checks      []doctorCheck `json:"checks"`
}

type doctorCheck struct {
	ID      string   `json:"id"`
	Title   string   `json:"title"`
	Status  string   `json:"status"`
	Details []string `json:"details"`
}

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check a site for problems that fail silently",
	Long: `Ask the site to check itself and print what it finds:

  - Signing key: whether a real JWT secret is set
  - Schemas: keys and field types the server silently ignores
  - Storage persistence: filesystem storage that a redeploy would wipe
  - Stored content: documents that fail the current schema rules (they cannot be
    saved until fixed) and documents with no draft/published status

Run it before an upgrade, after changing schemas, and after a restore. It needs
Trokky 3.6 or later on the server, and the settings:read permission (held by
admins): sign in as an admin (a sign-in from an older CLI lacks the system:read
scope, so run 'trokky login' again), or use an API token with settings:read.
The site reuses a report for up to a minute.

Exits non-zero when a check fails; with --strict, also on a warning.

Example:
  trokky doctor
  trokky doctor --json`,
	SilenceUsage: true,
	// Findings exit non-zero routinely: print the error once (Execute does), not twice
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		out := cmd.OutOrStdout()
		c, err := client.FromContext(cmd)
		if err != nil {
			return err
		}
		asJSON, _ := cmd.Flags().GetBool("json")
		strict, _ := cmd.Flags().GetBool("strict")

		data, err := c.Get("/system/doctor")
		if err != nil {
			return explainDoctorError(c, err)
		}
		var report doctorReport
		if err := json.Unmarshal(data, &report); err != nil || len(report.Checks) == 0 {
			return fmt.Errorf("the site's diagnostics response could not be read: %s", strings.TrimSpace(string(data)))
		}

		if asJSON {
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			if err := enc.Encode(report); err != nil {
				return err
			}
		} else {
			printDoctorReport(out, c.BaseURL, report)
		}

		switch {
		case report.Status == "fail":
			return fmt.Errorf("doctor found problems that need fixing")
		case strict && report.Status == "warn":
			return fmt.Errorf("doctor found warnings (--strict)")
		}
		return nil
	},
}

func explainDoctorError(c *client.Client, err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "HTTP 404"):
		// A wrong --url is a 404 too: only blame the server's version when a Trokky API answers
		// there.
		if !answersHealth(c) {
			return fmt.Errorf("no Trokky API answered at %s: check the URL (it usually ends in /api)", c.BaseURL)
		}
		return fmt.Errorf("this site has no diagnostics endpoint: trokky doctor needs Trokky 3.6 or later on the server")
	case strings.Contains(msg, "HTTP 403"):
		return fmt.Errorf("trokky doctor needs the settings:read permission (held by admins): sign in again as an admin with 'trokky login' and allow \"Read site diagnostics\", or use an API token that has settings:read")
	}
	return err
}

// answersHealth reports whether a Trokky health endpoint answers 200, under the API path
// (where Trokky serves it) or at the site root (where some setups expose it).
func answersHealth(c *client.Client) bool {
	urls := []string{c.BaseURL + "/health"}
	if root := strings.TrimSuffix(c.BaseURL, "/api"); root != c.BaseURL {
		urls = append(urls, root+"/health")
	}
	for _, u := range urls {
		resp, err := c.HTTPClient.Get(u)
		if err != nil {
			continue
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return true
		}
	}
	return false
}

func printDoctorReport(out io.Writer, site string, report doctorReport) {
	fmt.Fprintf(out, "Site: %s (%s), checked at %s\n", site, report.Environment, report.CheckedAt)
	fmt.Fprintln(out, "(The site reuses a report for up to a minute.)")
	fmt.Fprintln(out)
	labels := map[string]string{"ok": "ok  ", "warn": "WARN", "fail": "FAIL"}
	for _, check := range report.Checks {
		label, known := labels[check.Status]
		if !known {
			label = strings.ToUpper(check.Status)
		}
		fmt.Fprintf(out, "[%s] %s\n", label, check.Title)
		for _, detail := range check.Details {
			fmt.Fprintf(out, "       %s\n", detail)
		}
	}
	fmt.Fprintln(out)
	switch report.Status {
	case "fail":
		fmt.Fprintln(out, "Some checks failed.")
	case "warn":
		fmt.Fprintln(out, "No failures, some warnings.")
	default:
		fmt.Fprintln(out, "All checks passed.")
	}
}

func init() {
	doctorCmd.Flags().Bool("json", false, "print the report as JSON")
	doctorCmd.Flags().Bool("strict", false, "exit non-zero on warnings too")
	rootCmd.AddCommand(doctorCmd)
}
