package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	cfgFile string
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

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default $HOME/.trokky/config.json)")
	rootCmd.PersistentFlags().String("instance", "", "Trokky instance URL (overrides active instance)")
	rootCmd.PersistentFlags().String("token", "", "API token (overrides stored token)")
}
