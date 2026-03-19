package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/trokky/cli/internal/scaffold"
)

var createCmd = &cobra.Command{
	Use:   "create <project-name>",
	Short: "Scaffold a new Trokky project",
	Long: `Create a new Trokky project with interactive configuration.

You can choose a template (minimal, full, api-only) and then optionally
customize each option: data adapter, media adapter, mail, auth, studio,
captcha, and i18n.

Examples:
  trokky create my-site
  trokky create my-site --template full
  trokky create my-site --template minimal -y
  trokky create my-site --data postgres --media s3 --mail resend`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		yes, _ := cmd.Flags().GetBool("yes")

		targetDir, err := filepath.Abs(name)
		if err != nil {
			return fmt.Errorf("invalid project name: %w", err)
		}

		// Check if directory already exists
		if info, err := os.Stat(targetDir); err == nil && info.IsDir() {
			entries, _ := os.ReadDir(targetDir)
			if len(entries) > 0 {
				return fmt.Errorf("directory %q already exists and is not empty", name)
			}
		}

		// Determine template
		templateFlag, _ := cmd.Flags().GetString("template")

		var cfg scaffold.ProjectConfig

		if templateFlag != "" {
			tmpl := scaffold.Template(templateFlag)
			defaults, ok := scaffold.TemplateDefaults[tmpl]
			if !ok {
				return fmt.Errorf("unknown template %q (valid: minimal, full, api-only)", templateFlag)
			}
			cfg = defaults
		} else if yes {
			cfg = scaffold.TemplateDefaults[scaffold.TemplateMinimal]
		} else {
			cfg = scaffold.TemplateDefaults[scaffold.TemplateMinimal]
		}

		cfg.Name = filepath.Base(targetDir)

		// Apply any explicit flags (these override template defaults)
		applyFlags(cmd, &cfg)

		// Interactive prompts if not --yes and no --template flag
		if !yes && templateFlag == "" {
			reader := bufio.NewReader(os.Stdin)

			fmt.Println()
			fmt.Println("  Create a new Trokky project")
			fmt.Println()

			// Template selection
			tmpl, err := promptTemplate(reader)
			if err != nil {
				return err
			}
			cfg = scaffold.TemplateDefaults[tmpl]
			cfg.Name = filepath.Base(targetDir)

			// Ask if they want to customize
			fmt.Print("  Customize options? [y/N]: ")
			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(strings.ToLower(input))

			if input == "y" || input == "yes" {
				if err := promptCustomize(reader, &cfg); err != nil {
					return err
				}
			}

			// Apply explicit flags last (override interactive choices)
			applyFlags(cmd, &cfg)
		}

		// Show summary
		fmt.Println()
		fmt.Println("  Project configuration:")
		fmt.Println()
		fmt.Printf("    Name:      %s\n", cfg.Name)
		fmt.Printf("    Template:  %s\n", cfg.Template)
		fmt.Printf("    Data:      %s\n", cfg.DataAdapter)
		fmt.Printf("    Media:     %s\n", cfg.MediaAdapter)
		fmt.Printf("    Mail:      %s\n", cfg.Mail)
		fmt.Printf("    Auth:      %s\n", cfg.Auth)
		fmt.Printf("    Studio:    %s\n", cfg.Studio)
		fmt.Printf("    Captcha:   %s\n", cfg.Captcha)
		fmt.Printf("    I18n:      %s\n", cfg.I18n)
		fmt.Printf("    Examples:  %v\n", cfg.IncludeExamples)
		fmt.Println()

		if !yes {
			ok, err := confirmPrompt("  Proceed with scaffolding?")
			if err != nil {
				return err
			}
			if !ok {
				fmt.Println("  Cancelled.")
				return nil
			}
		}

		// Validate config
		if err := cfg.Validate(); err != nil {
			return err
		}

		// Scaffold
		if err := scaffold.Scaffold(cfg, targetDir); err != nil {
			return fmt.Errorf("scaffolding failed: %w", err)
		}

		// Success output
		fmt.Println()
		fmt.Printf("  Project %q created successfully!\n", name)
		fmt.Println()
		fmt.Println("  Next steps:")
		fmt.Println()
		fmt.Printf("    cd %s\n", name)
		fmt.Println("    cp .env.example .env     # Configure your environment")
		fmt.Println("    npm install               # Install dependencies")
		fmt.Println("    npm run dev               # Start development server")
		fmt.Println()

		return nil
	},
}

func promptTemplate(reader *bufio.Reader) (scaffold.Template, error) {
	fmt.Println("  Select a template:")
	fmt.Println()
	fmt.Println("    1) minimal  - Filesystem storage, basic auth, embedded studio")
	fmt.Println("    2) full     - Postgres, S3, OAuth, mail, captcha, i18n, examples")
	fmt.Println("    3) api-only - Filesystem storage, basic auth, no studio")
	fmt.Println()
	fmt.Print("  Template [1]: ")
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		input = "1"
	}

	switch input {
	case "1", "minimal":
		return scaffold.TemplateMinimal, nil
	case "2", "full":
		return scaffold.TemplateFull, nil
	case "3", "api-only":
		return scaffold.TemplateAPIOnly, nil
	default:
		return "", fmt.Errorf("invalid template choice: %q", input)
	}
}

func promptCustomize(reader *bufio.Reader, cfg *scaffold.ProjectConfig) error {
	fmt.Println()

	// Data adapter
	fmt.Println("  Data adapter:")
	fmt.Println("    1) filesystem  2) postgres  3) d1")
	fmt.Printf("  Choice [%s]: ", dataAdapterIndex(cfg.DataAdapter))
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input != "" {
		switch input {
		case "1", "filesystem":
			cfg.DataAdapter = scaffold.DataFilesystem
		case "2", "postgres":
			cfg.DataAdapter = scaffold.DataPostgres
		case "3", "d1":
			cfg.DataAdapter = scaffold.DataD1
		default:
			return fmt.Errorf("invalid data adapter choice: %q", input)
		}
	}

	// Media adapter
	fmt.Println("  Media adapter:")
	fmt.Println("    1) filesystem  2) r2  3) s3")
	fmt.Printf("  Choice [%s]: ", mediaAdapterIndex(cfg.MediaAdapter))
	input, _ = reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input != "" {
		switch input {
		case "1", "filesystem":
			cfg.MediaAdapter = scaffold.MediaFilesystem
		case "2", "r2":
			cfg.MediaAdapter = scaffold.MediaR2
		case "3", "s3":
			cfg.MediaAdapter = scaffold.MediaS3
		default:
			return fmt.Errorf("invalid media adapter choice: %q", input)
		}
	}

	// Mail
	fmt.Println("  Mail provider:")
	fmt.Println("    1) none  2) resend  3) console")
	fmt.Printf("  Choice [%s]: ", mailIndex(cfg.Mail))
	input, _ = reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input != "" {
		switch input {
		case "1", "none":
			cfg.Mail = scaffold.MailNone
		case "2", "resend":
			cfg.Mail = scaffold.MailResend
		case "3", "console":
			cfg.Mail = scaffold.MailConsole
		default:
			return fmt.Errorf("invalid mail choice: %q", input)
		}
	}

	// Auth
	fmt.Println("  Auth mode:")
	fmt.Println("    1) basic  2) oauth  3) none")
	fmt.Printf("  Choice [%s]: ", authIndex(cfg.Auth))
	input, _ = reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input != "" {
		switch input {
		case "1", "basic":
			cfg.Auth = scaffold.AuthBasic
		case "2", "oauth":
			cfg.Auth = scaffold.AuthOAuth
		case "3", "none":
			cfg.Auth = scaffold.AuthNone
		default:
			return fmt.Errorf("invalid auth choice: %q", input)
		}
	}

	// Studio
	fmt.Println("  Studio mode:")
	fmt.Println("    1) embedded  2) separate  3) none")
	fmt.Printf("  Choice [%s]: ", studioIndex(cfg.Studio))
	input, _ = reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input != "" {
		switch input {
		case "1", "embedded":
			cfg.Studio = scaffold.StudioEmbedded
		case "2", "separate":
			cfg.Studio = scaffold.StudioSeparate
		case "3", "none":
			cfg.Studio = scaffold.StudioNone
		default:
			return fmt.Errorf("invalid studio choice: %q", input)
		}
	}

	// Captcha
	fmt.Println("  Captcha provider:")
	fmt.Println("    1) none  2) turnstile  3) recaptcha")
	fmt.Printf("  Choice [%s]: ", captchaIndex(cfg.Captcha))
	input, _ = reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input != "" {
		switch input {
		case "1", "none":
			cfg.Captcha = scaffold.CaptchaNone
		case "2", "turnstile":
			cfg.Captcha = scaffold.CaptchaTurnstile
		case "3", "recaptcha":
			cfg.Captcha = scaffold.CaptchaRecaptcha
		default:
			return fmt.Errorf("invalid captcha choice: %q", input)
		}
	}

	// I18n
	fmt.Println("  I18n mode:")
	fmt.Println("    1) none  2) en  3) fr  4) en-fr")
	fmt.Printf("  Choice [%s]: ", i18nIndex(cfg.I18n))
	input, _ = reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input != "" {
		switch input {
		case "1", "none":
			cfg.I18n = scaffold.I18nNone
		case "2", "en":
			cfg.I18n = scaffold.I18nEn
		case "3", "fr":
			cfg.I18n = scaffold.I18nFr
		case "4", "en-fr":
			cfg.I18n = scaffold.I18nEnFr
		default:
			return fmt.Errorf("invalid i18n choice: %q", input)
		}
	}

	// Examples
	fmt.Print("  Include example schemas? [y/N]: ")
	input, _ = reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	cfg.IncludeExamples = input == "y" || input == "yes"

	return nil
}

func applyFlags(cmd *cobra.Command, cfg *scaffold.ProjectConfig) {
	if cmd.Flags().Changed("data") {
		v, _ := cmd.Flags().GetString("data")
		cfg.DataAdapter = scaffold.DataAdapter(v)
	}
	if cmd.Flags().Changed("media") {
		v, _ := cmd.Flags().GetString("media")
		cfg.MediaAdapter = scaffold.MediaAdapter(v)
	}
	if cmd.Flags().Changed("mail") {
		v, _ := cmd.Flags().GetString("mail")
		cfg.Mail = scaffold.MailProvider(v)
	}
	if cmd.Flags().Changed("auth") {
		v, _ := cmd.Flags().GetString("auth")
		cfg.Auth = scaffold.AuthMode(v)
	}
	if cmd.Flags().Changed("studio") {
		v, _ := cmd.Flags().GetString("studio")
		cfg.Studio = scaffold.StudioMode(v)
	}
	if cmd.Flags().Changed("captcha") {
		v, _ := cmd.Flags().GetString("captcha")
		cfg.Captcha = scaffold.CaptchaProvider(v)
	}
	if cmd.Flags().Changed("i18n") {
		v, _ := cmd.Flags().GetString("i18n")
		cfg.I18n = scaffold.I18nMode(v)
	}
	if cmd.Flags().Changed("examples") {
		v, _ := cmd.Flags().GetBool("examples")
		cfg.IncludeExamples = v
	}
}

func dataAdapterIndex(v scaffold.DataAdapter) string {
	switch v {
	case scaffold.DataFilesystem:
		return "1"
	case scaffold.DataPostgres:
		return "2"
	case scaffold.DataD1:
		return "3"
	}
	return "1"
}

func mediaAdapterIndex(v scaffold.MediaAdapter) string {
	switch v {
	case scaffold.MediaFilesystem:
		return "1"
	case scaffold.MediaR2:
		return "2"
	case scaffold.MediaS3:
		return "3"
	}
	return "1"
}

func mailIndex(v scaffold.MailProvider) string {
	switch v {
	case scaffold.MailNone:
		return "1"
	case scaffold.MailResend:
		return "2"
	case scaffold.MailConsole:
		return "3"
	}
	return "1"
}

func authIndex(v scaffold.AuthMode) string {
	switch v {
	case scaffold.AuthBasic:
		return "1"
	case scaffold.AuthOAuth:
		return "2"
	case scaffold.AuthNone:
		return "3"
	}
	return "1"
}

func studioIndex(v scaffold.StudioMode) string {
	switch v {
	case scaffold.StudioEmbedded:
		return "1"
	case scaffold.StudioSeparate:
		return "2"
	case scaffold.StudioNone:
		return "3"
	}
	return "1"
}

func captchaIndex(v scaffold.CaptchaProvider) string {
	switch v {
	case scaffold.CaptchaNone:
		return "1"
	case scaffold.CaptchaTurnstile:
		return "2"
	case scaffold.CaptchaRecaptcha:
		return "3"
	}
	return "1"
}

func i18nIndex(v scaffold.I18nMode) string {
	switch v {
	case scaffold.I18nNone:
		return "1"
	case scaffold.I18nEn:
		return "2"
	case scaffold.I18nFr:
		return "3"
	case scaffold.I18nEnFr:
		return "4"
	}
	return "1"
}

func init() {
	createCmd.Flags().StringP("template", "t", "", "Project template (minimal, full, api-only)")
	createCmd.Flags().String("data", "", "Data adapter (filesystem, postgres, d1)")
	createCmd.Flags().String("media", "", "Media adapter (filesystem, r2, s3)")
	createCmd.Flags().String("mail", "", "Mail provider (none, resend, console)")
	createCmd.Flags().String("auth", "", "Auth mode (basic, oauth, none)")
	createCmd.Flags().String("studio", "", "Studio mode (embedded, separate, none)")
	createCmd.Flags().String("captcha", "", "Captcha provider (none, turnstile, recaptcha)")
	createCmd.Flags().String("i18n", "", "I18n mode (none, en, fr, en-fr)")
	createCmd.Flags().Bool("examples", false, "Include example schemas")
	createCmd.Flags().BoolP("yes", "y", false, "Skip prompts, use defaults")

	rootCmd.AddCommand(createCmd)
}
