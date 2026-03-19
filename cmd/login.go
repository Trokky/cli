package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/spf13/cobra"
	"github.com/trokky/cli/internal/auth"
	"github.com/trokky/cli/internal/config"
)

var loginCmd = &cobra.Command{
	Use:   "login <url>",
	Short: "Login to a Trokky instance using browser authentication",
	Long: `Authenticate with a Trokky instance using the OAuth2 device flow.
A browser window will open for you to authorize access.

Example:
  trokky login https://cms.example.com
  trokky login https://cms.example.com --name production`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		rawURL := args[0]

		baseURL := config.NormalizeBaseURL(rawURL)

		name, _ := cmd.Flags().GetString("name")
		if name == "" {
			name = auth.DeriveInstanceName(baseURL)
		}
		setDefault, _ := cmd.Flags().GetBool("set-default")

		fmt.Println()
		fmt.Println("Trokky CLI Login")
		fmt.Println("────────────────────────────────────────")
		fmt.Println()

		// Start device authorization
		s := spinner.New(spinner.CharSets[14], 100*time.Millisecond, spinner.WithWriter(os.Stderr))
		s.Suffix = " Starting device authorization..."
		s.Start()
		deviceAuth, err := auth.StartDeviceAuth(baseURL)
		s.Stop()
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "✓ Device authorization started")

		fmt.Println()
		fmt.Println("To complete login:")
		fmt.Println()
		fmt.Printf("  1. Open this URL in your browser:\n")
		fmt.Printf("     %s\n", deviceAuth.VerificationURI)
		fmt.Println()
		fmt.Printf("  2. Enter this code when prompted:\n")
		fmt.Printf("     %s\n", deviceAuth.UserCode)
		fmt.Println()

		if deviceAuth.VerificationURIComplete != "" {
			fmt.Printf("  Or open the complete URL directly:\n")
			fmt.Printf("     %s\n", deviceAuth.VerificationURIComplete)
			fmt.Println()
		}

		fmt.Printf("  Code expires in %d minutes\n", deviceAuth.ExpiresIn/60)
		fmt.Println()

		// Try to open browser
		if deviceAuth.VerificationURIComplete != "" {
			if err := auth.OpenBrowser(deviceAuth.VerificationURIComplete); err == nil {
				fmt.Println("  Browser opened automatically")
				fmt.Println()
			}
		}

		// Poll for token
		s.Suffix = " Waiting for authorization..."
		s.Start()
		tokenResp, err := auth.PollForToken(baseURL, deviceAuth.DeviceCode, deviceAuth.Interval, deviceAuth.ExpiresIn)
		s.Stop()
		if err != nil {
			return err
		}
		fmt.Fprintln(os.Stderr, "✓ Authorization successful")

		// Save to config
		err = config.AddInstance(name, config.InstanceConfig{
			URL:            baseURL,
			Token:          tokenResp.AccessToken,
			RefreshToken:   tokenResp.RefreshToken,
			AuthType:       config.AuthTypeOAuth2,
			TokenExpiresAt: auth.ExpiresAtFromNow(tokenResp.ExpiresIn),
			Description:    "Logged in via OAuth2 device flow",
		}, setDefault)
		if err != nil {
			return fmt.Errorf("failed to save config: %w", err)
		}

		fmt.Println()
		fmt.Println("Login successful!")
		fmt.Println()
		fmt.Printf("  Instance: %s\n", name)
		fmt.Printf("  URL:      %s\n", baseURL)
		if tokenResp.Scope != "" {
			fmt.Printf("  Scopes:   %s\n", tokenResp.Scope)
		}
		if setDefault {
			fmt.Printf("  Default:  Yes\n")
		}
		fmt.Println()
		fmt.Println("You can now use trokky commands without --url and --token flags.")
		fmt.Println()

		return nil
	},
}

func init() {
	loginCmd.Flags().String("name", "", "instance name (default: derived from URL hostname)")
	loginCmd.Flags().Bool("set-default", true, "set this instance as the default")
	rootCmd.AddCommand(loginCmd)
}
