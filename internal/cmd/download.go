package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/devopsjester/plaud-hub/internal/api"
	"github.com/devopsjester/plaud-hub/internal/config"
	"github.com/devopsjester/plaud-hub/internal/download"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download transcripts and/or summaries from Plaud",
	Long: `Download transcripts and/or AI summaries for your Plaud recordings
and save them as Markdown files with YAML front matter.

By default, downloads both transcripts and summaries. Use --type to filter.
Existing files are skipped unless --force is specified.`,
	RunE: runDownload,
}

func init() {
	rootCmd.AddCommand(downloadCmd)

	downloadCmd.Flags().String("from", "", "start date (inclusive, YYYY-MM-DD)")
	downloadCmd.Flags().String("to", "", "end date (inclusive, YYYY-MM-DD)")
	downloadCmd.Flags().String("type", config.DefaultType, "content type: transcript, summary, or all")
	downloadCmd.Flags().String("output-dir", config.DefaultOutputDir, "output directory for downloaded files")
	downloadCmd.Flags().Bool("force", false, "re-download even if files already exist")
	downloadCmd.Flags().Int("concurrency", config.DefaultConcurrency, "number of concurrent downloads")

	_ = viper.BindPFlag("output_dir", downloadCmd.Flags().Lookup("output-dir"))
	_ = viper.BindPFlag("concurrency", downloadCmd.Flags().Lookup("concurrency"))
	_ = viper.BindPFlag("type", downloadCmd.Flags().Lookup("type"))
}

func runDownload(cmd *cobra.Command, _ []string) error {
	logger := newLogger()

	token, err := config.Token()
	if err != nil {
		return err
	}

	// Parse date flags.
	var from, to time.Time
	if f, _ := cmd.Flags().GetString("from"); f != "" {
		from, err = time.Parse("2006-01-02", f)
		if err != nil {
			return fmt.Errorf("invalid --from date %q: expected YYYY-MM-DD", f)
		}
	}
	if t, _ := cmd.Flags().GetString("to"); t != "" {
		to, err = time.Parse("2006-01-02", t)
		if err != nil {
			return fmt.Errorf("invalid --to date %q: expected YYYY-MM-DD", t)
		}
		// Make "to" inclusive by setting it to end of day.
		to = to.Add(24*time.Hour - time.Nanosecond)
	}

	dlType := viper.GetString("type")
	if dlType != "all" && dlType != "transcript" && dlType != "summary" {
		return fmt.Errorf("invalid --type %q: must be transcript, summary, or all", dlType)
	}

	force, _ := cmd.Flags().GetBool("force")
	outputDir := viper.GetString("output_dir")
	concurrency := viper.GetInt("concurrency")
	if concurrency < 1 {
		concurrency = 1
	}

	client := api.NewClient(token, logger)

	// Set up signal handling.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	logger.Info("fetching recording list...",
		"from", from.Format("2006-01-02"),
		"to", to.Format("2006-01-02"),
	)

	recordings, err := client.ListRecordingsInRange(ctx, from, to)
	if err != nil {
		return fmt.Errorf("list recordings: %w", err)
	}

	logger.Info("found recordings", "count", len(recordings))

	if len(recordings) == 0 {
		fmt.Println("No recordings found in the specified date range.")
		return nil
	}

	opts := download.Options{
		OutputDir:   outputDir,
		Type:        dlType,
		Force:       force,
		Concurrency: concurrency,
	}

	result, err := download.Run(ctx, client, recordings, opts, logger)
	if err != nil {
		return err
	}

	fmt.Println(result.FormatResult())
	return nil
}
