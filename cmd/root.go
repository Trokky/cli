package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
)

var rootCmd = &cobra.Command{
	Use:   "trokky",
	Short: "Trokky CLI — manage your Trokky CMS instances",
	Long: `Trokky CLI is a command-line tool for managing Trokky CMS instances.

  Login, backup, restore, export/import content, and generate TypeScript types
  from any Trokky instance.`,
	Version: version,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// confirmPrompt asks the user for y/N confirmation. Returns true if confirmed.
func confirmPrompt(message string) (bool, error) {
	fmt.Printf("%s [y/N] ", message)
	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	answer := strings.TrimSpace(strings.ToLower(input))
	return answer == "y" || answer == "yes", nil
}

func init() {
	rootCmd.PersistentFlags().String("url", "", "Trokky instance URL (or use TROKKY_URL env var)")
	rootCmd.PersistentFlags().String("token", "", "API token (or use TROKKY_TOKEN env var)")
	rootCmd.PersistentFlags().String("instance", "", "Use a specific configured instance")
	rootCmd.PersistentFlags().BoolP("quiet", "q", false, "suppress informational output")
}
