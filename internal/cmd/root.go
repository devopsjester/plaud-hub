// Package cmd implements the CLI commands for plaud-downloader.
package cmd

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/devopsjester/plaud-downloader/internal/config"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgFile string
	verbose bool
)

// rootCmd is the base command for plaud-downloader.
var rootCmd = &cobra.Command{
	Use:   "plaud-downloader",
	Short: "Download Plaud Note transcripts and summaries",
	Long: `plaud-downloader fetches transcripts and/or AI summaries from
your Plaud AI account and saves them as Markdown files with YAML front matter.

Requires a Plaud API token. Run 'plaud-downloader auth setup' to configure.`,
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)

	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ./plaud-downloader.yaml or ~/.config/plaud-downloader/config.yaml)")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable debug logging")
	rootCmd.PersistentFlags().String("token", "", "Plaud API token (overrides config/env)")

	_ = viper.BindPFlag("token", rootCmd.PersistentFlags().Lookup("token"))
}

func initConfig() {
	if err := config.Setup(cfgFile); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}
}

// newLogger creates a slog.Logger based on the verbose flag.
func newLogger() *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	}))
}
