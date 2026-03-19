package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/trokky/cli/internal/config"
	"golang.org/x/term"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage Trokky instance configurations",
	Long:  `Add, remove, list, and switch between configured Trokky instances.`,
}

// --- config add ---

var configAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a new Trokky instance configuration",
	Long: `Add a new instance configuration with a name, URL, and token.
If --url or --token are not provided, you will be prompted interactively.

Example:
  trokky config add production --url https://cms.example.com/api --token my-token
  trokky config add staging`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		url, _ := cmd.Flags().GetString("url")
		token, _ := cmd.Flags().GetString("token")
		description, _ := cmd.Flags().GetString("description")
		setDefault, _ := cmd.Flags().GetBool("default")

		reader := bufio.NewReader(os.Stdin)

		if url == "" {
			fmt.Print("Trokky instance URL: ")
			input, err := reader.ReadString('\n')
			if err != nil {
				return fmt.Errorf("failed to read URL: %w", err)
			}
			url = strings.TrimSpace(input)
			if url == "" {
				return fmt.Errorf("URL is required")
			}
		}

		if token == "" {
			fmt.Print("Authentication token: ")
			tokenBytes, err := term.ReadPassword(int(syscall.Stdin))
			if err != nil {
				return fmt.Errorf("failed to read token: %w", err)
			}
			fmt.Println()
			token = string(tokenBytes)
			if token == "" {
				return fmt.Errorf("token is required")
			}
		}

		err := config.AddInstance(name, config.InstanceConfig{
			URL:         url,
			Token:       token,
			AuthType:    config.AuthTypeAPIToken,
			Description: description,
		}, setDefault)
		if err != nil {
			return fmt.Errorf("failed to add instance: %w", err)
		}

		fmt.Printf("✓ Instance %q added successfully\n", name)
		fmt.Printf("  URL:   %s\n", url)
		fmt.Printf("  Token: %s\n", config.MaskToken(token))
		if setDefault {
			fmt.Printf("  Set as default\n")
		}
		fmt.Printf("\n  Config saved to: %s\n", config.ConfigPath())
		return nil
	},
}

// --- config remove ---

var configRemoveCmd = &cobra.Command{
	Use:   "remove <name>",
	Short: "Remove a Trokky instance configuration",
	Long: `Remove a configured instance by name.

Example:
  trokky config remove staging
  trokky config remove staging --force`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		force, _ := cmd.Flags().GetBool("force")

		if !force {
			ok, err := confirmPrompt(fmt.Sprintf("Remove instance %q?", name))
			if err != nil {
				return err
			}
			if !ok {
				fmt.Println("Cancelled")
				return nil
			}
		}

		removed, err := config.RemoveInstance(name)
		if err != nil {
			return err
		}
		if !removed {
			return fmt.Errorf("instance %q not found", name)
		}

		fmt.Printf("✓ Instance %q removed\n", name)
		return nil
	},
}

// --- config list ---

var configListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all configured Trokky instances",
	RunE: func(cmd *cobra.Command, args []string) error {
		instances, defaultName, err := config.ListInstances()
		if err != nil {
			return err
		}

		if len(instances) == 0 {
			fmt.Println("No instances configured yet.")
			fmt.Println("Run `trokky config add <name>` to add one.")
			return nil
		}

		fmt.Print("\nConfigured Instances\n\n")

		for name, inst := range instances {
			marker := "  "
			if name == defaultName {
				marker = "* "
			}

			fmt.Printf("  %s%s\n", marker, name)
			fmt.Printf("      URL:   %s\n", inst.URL)
			fmt.Printf("      Token: %s\n", config.MaskToken(inst.Token))
			if inst.Description != "" {
				fmt.Printf("      Desc:  %s\n", inst.Description)
			}
			fmt.Println()
		}

		if defaultName != "" {
			fmt.Println("  * = default instance")
		}
		fmt.Println()
		return nil
	},
}

// --- config use ---

var configUseCmd = &cobra.Command{
	Use:   "use <name>",
	Short: "Set the default Trokky instance",
	Long: `Set which configured instance is used by default.

Example:
  trokky config use production`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		ok, err := config.SetDefaultInstance(name)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Printf("Instance %q not found.\n", name)
			fmt.Println("Run `trokky config list` to see available instances.")
			return fmt.Errorf("instance %q not found", name)
		}

		fmt.Printf("✓ Now using %q as default\n", name)
		return nil
	},
}

// --- config path ---

var configPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Show the config file path",
	Run: func(cmd *cobra.Command, args []string) {
		path := config.ConfigPath()
		exists := config.ConfigExists()

		fmt.Print("\nConfig File Location\n\n")
		fmt.Printf("  %s\n", path)
		status := "not created yet"
		if exists {
			status = "exists"
		}
		fmt.Printf("  Status: %s\n\n", status)
	},
}

func init() {
	// config add flags
	configAddCmd.Flags().String("url", "", "Trokky instance URL")
	configAddCmd.Flags().String("token", "", "Authentication token")
	configAddCmd.Flags().String("description", "", "Description for this instance")
	configAddCmd.Flags().Bool("default", false, "Set as the default instance")

	// config remove flags
	configRemoveCmd.Flags().Bool("force", false, "Skip confirmation prompt")

	// Register subcommands
	configCmd.AddCommand(configAddCmd)
	configCmd.AddCommand(configRemoveCmd)
	configCmd.AddCommand(configListCmd)
	configCmd.AddCommand(configUseCmd)
	configCmd.AddCommand(configPathCmd)

	rootCmd.AddCommand(configCmd)
}
