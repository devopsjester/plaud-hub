package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/devopsjester/plaud-hub/internal/config"
	"github.com/devopsjester/plaud-hub/internal/customer"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var correlateCmd = &cobra.Command{
	Use:   "correlate",
	Short: "Organize downloaded files into per-customer folders",
	Long: `Scans the output directory for downloaded Plaud Markdown files, identifies
which customer(s) each recording relates to using a customer registry YAML file,
and copies (or moves) the files into output/customers/{CustomerName}/ subfolders.

When a recording mentions multiple customers, files are copied to each customer
folder. Use --move to remove the originals from the output root after copying.

Example registry file (customers.yaml):

  customers:
    - name: McDonalds
      aliases: ["McDonald's", "mcd"]
    - name: Nextera
      aliases: ["NextEra Energy"]
    - name: SLB
      aliases: ["Schlumberger"]`,
	RunE: runCorrelate,
}

func init() {
	rootCmd.AddCommand(correlateCmd)
	correlateCmd.Flags().String("output-dir", config.DefaultOutputDir, "directory containing downloaded files")
	correlateCmd.Flags().String("customers-file", "", "path to customer registry YAML file (required)")
	correlateCmd.Flags().Bool("move", false, "move files instead of copying (removes originals from output root)")
	correlateCmd.Flags().String("min-confidence", customer.ConfidenceMedium, "minimum confidence level to act on: high, medium, or low")

	_ = correlateCmd.MarkFlagRequired("customers-file")
	_ = viper.BindPFlag("output_dir", correlateCmd.Flags().Lookup("output-dir"))
}

func runCorrelate(cmd *cobra.Command, _ []string) error {
	logger := newLogger()

	outputDir := viper.GetString("output_dir")
	customersFile, _ := cmd.Flags().GetString("customers-file")
	moveFiles, _ := cmd.Flags().GetBool("move")
	minConf, _ := cmd.Flags().GetString("min-confidence")

	if customer.ConfidenceRank(minConf) == 0 {
		return fmt.Errorf("invalid --min-confidence %q: must be high, medium, or low", minConf)
	}

	registry, err := customer.LoadRegistry(customersFile)
	if err != nil {
		return err
	}
	if len(registry.Customers) == 0 {
		return fmt.Errorf("customers file %q contains no customers", customersFile)
	}

	// Gather all summary files in the output root (not in subdirs).
	summaries, err := filepath.Glob(filepath.Join(outputDir, "*_summary.md"))
	if err != nil {
		return fmt.Errorf("scan output dir: %w", err)
	}
	if len(summaries) == 0 {
		logger.Info("no summary files found", "dir", outputDir)
		return nil
	}

	minRank := customer.ConfidenceRank(minConf)
	var placed, skipped int

	for _, summaryPath := range summaries {
		matches, err := customer.CorrelateFile(summaryPath, registry)
		if err != nil {
			logger.Warn("skipping (parse error)", "file", filepath.Base(summaryPath), "err", err)
			skipped++
			continue
		}

		// Filter to eligible matches.
		eligible := make([]customer.Match, 0, len(matches))
		for _, m := range matches {
			if customer.ConfidenceRank(m.Confidence) >= minRank {
				eligible = append(eligible, m)
			}
		}
		if len(eligible) == 0 {
			logger.Debug("no customer match", "file", filepath.Base(summaryPath))
			skipped++
			continue
		}

		// Derive the paired transcript path.
		transcriptPath := strings.TrimSuffix(summaryPath, "_summary.md") + "_transcript.md"
		_, transcriptErr := os.Stat(transcriptPath)
		hasTranscript := transcriptErr == nil

		// Copy (or move) to every matched customer folder.
		// When moving and multi-customer, we copy to all then remove originals.
		for _, m := range eligible {
			destDir := customer.CustomerOutputDir(outputDir, m.Customer.Name)
			if err := os.MkdirAll(destDir, 0o755); err != nil {
				return fmt.Errorf("create customer dir %q: %w", destDir, err)
			}

			// For single-customer + --move, we can use rename. For multi-customer
			// we always copy first and remove originals after the loop.
			useRename := moveFiles && len(eligible) == 1

			summaryDest := filepath.Join(destDir, filepath.Base(summaryPath))
			if err := copyOrMoveFile(summaryPath, summaryDest, useRename); err != nil {
				logger.Warn("failed to place summary", "file", filepath.Base(summaryPath), "customer", m.Customer.Name, "err", err)
				continue
			}

			if hasTranscript {
				transcriptDest := filepath.Join(destDir, filepath.Base(transcriptPath))
				if err := copyOrMoveFile(transcriptPath, transcriptDest, useRename); err != nil {
					logger.Warn("failed to place transcript", "file", filepath.Base(transcriptPath), "customer", m.Customer.Name, "err", err)
				}
			}

			logger.Info("placed",
				"file", filepath.Base(summaryPath),
				"customer", m.Customer.Name,
				"confidence", m.Confidence,
			)
			placed++
		}

		// Multi-customer + --move: remove originals after all copies succeeded.
		if moveFiles && len(eligible) > 1 {
			_ = os.Remove(summaryPath)
			if hasTranscript {
				_ = os.Remove(transcriptPath)
			}
		}
	}

	fmt.Printf("\nCorrelation complete: %d recording(s) placed, %d skipped (no match)\n", placed, skipped)
	return nil
}

// copyOrMoveFile copies src to dst, or renames if move is true.
func copyOrMoveFile(src, dst string, move bool) error {
	if move {
		return os.Rename(src, dst)
	}
	return copyFile(src, dst)
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %s → %s: %w", src, dst, err)
	}
	return out.Sync()
}
