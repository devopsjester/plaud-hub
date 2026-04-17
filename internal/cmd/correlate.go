package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/devopsjester/plaud-hub/internal/calendar"
	googlecal "github.com/devopsjester/plaud-hub/internal/calendar/google"
	reclaimcal "github.com/devopsjester/plaud-hub/internal/calendar/reclaim"
	"github.com/devopsjester/plaud-hub/internal/config"
	"github.com/devopsjester/plaud-hub/internal/customer"
	"github.com/devopsjester/plaud-hub/internal/llm"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// calendarClient is the common interface both Google and Reclaim clients satisfy.
type calendarClient interface {
	ListEvents(ctx context.Context, from, to time.Time) ([]calendar.CalendarEvent, error)
}

var correlateCmd = &cobra.Command{
	Use:   "correlate",
	Short: "Organize downloaded files into per-customer folders",
	Long: `Scans the output directory for downloaded Plaud Markdown files, identifies
which customer(s) each recording relates to using a customer registry YAML file,
and moves the files into output/customers/{CustomerName}/ subfolders by default.

Both the summary and transcript are searched for customer names. When
--calendar reclaim or --calendar google is specified, calendar events are fetched
for each recording's date; attendee email domains are matched against the
customers file to confirm or add customer matches.

Transcript files in downloaded/ are deleted after correlation by default — they
are staging artifacts used for matching signal only. Use --keep-transcripts to
retain them (e.g. for audit purposes).

When a recording matches multiple customers, files are copied to each folder and
the original is removed. Use --keep to preserve originals in the output root.

When --split-llm is set, multi-customer summaries are split by an LLM so each
customer folder receives only their relevant content.`,
	RunE: runCorrelate,
}

func init() {
	rootCmd.AddCommand(correlateCmd)
	correlateCmd.Flags().String("output-dir", config.DefaultOutputDir, "root output directory (expects downloaded/ subdir as input)")
	correlateCmd.Flags().String("customers-file", "", "path to customer registry YAML file (required)")
	correlateCmd.Flags().Bool("keep", false, "keep originals in output root (default is to move)")
	correlateCmd.Flags().String("min-confidence", customer.ConfidenceMedium, "minimum confidence level to act on: high, medium, or low")
	correlateCmd.Flags().String("calendar", "", "calendar provider to use: reclaim or google (default from config: reclaim)")
	correlateCmd.Flags().Duration("calendar-tolerance", 15*time.Minute, "time window around recording start to search for a matching calendar event")
	correlateCmd.Flags().String("split-llm", "", "use LLM to split multi-customer summaries: github")
	correlateCmd.Flags().Bool("keep-transcripts", false, "keep transcript files in downloaded/ after correlation (default is to delete them)")

	_ = correlateCmd.MarkFlagRequired("customers-file")
	_ = viper.BindPFlag("output_dir", correlateCmd.Flags().Lookup("output-dir"))
	_ = viper.BindPFlag("calendar_provider", correlateCmd.Flags().Lookup("calendar"))
}

func runCorrelate(cmd *cobra.Command, _ []string) error {
	logger := newLogger()

	outputDir := viper.GetString("output_dir")
	customersFile, _ := cmd.Flags().GetString("customers-file")
	keepFiles, _ := cmd.Flags().GetBool("keep")
	moveFiles := !keepFiles
	minConf, _ := cmd.Flags().GetString("min-confidence")
	// Resolve calendar provider: explicit flag > config file > default (reclaim).
	calProvider := viper.GetString("calendar_provider")
	if flagVal, _ := cmd.Flags().GetString("calendar"); cmd.Flags().Changed("calendar") {
		calProvider = flagVal
	}
	calTolerance, _ := cmd.Flags().GetDuration("calendar-tolerance")
	splitLLM, _ := cmd.Flags().GetString("split-llm")
	keepTranscripts, _ := cmd.Flags().GetBool("keep-transcripts")

	if customer.ConfidenceRank(minConf) == 0 {
		return fmt.Errorf("invalid --min-confidence %q: must be high, medium, or low", minConf)
	}
	if calProvider != "" && calProvider != "google" && calProvider != "reclaim" {
		return fmt.Errorf("invalid --calendar %q: must be \"google\" or \"reclaim\"", calProvider)
	}
	if splitLLM != "" && splitLLM != "github" {
		return fmt.Errorf("invalid --split-llm %q: must be \"github\"", splitLLM)
	}

	registry, err := customer.LoadRegistry(customersFile)
	if err != nil {
		return err
	}
	if len(registry.Customers) == 0 {
		return fmt.Errorf("customers file %q contains no customers", customersFile)
	}

	// Build the calendar client if requested.
	var calClient calendarClient
	switch calProvider {
	case "reclaim":
		apiKey, err := config.LoadReclaimKey()
		if err != nil {
			return fmt.Errorf("load Reclaim API key: %w", err)
		}
		if apiKey == "" {
			return fmt.Errorf("no Reclaim API key found — run: plaud-hub auth setup-reclaim")
		}
		calClient = reclaimcal.NewClient(apiKey)
		logger.Info("Reclaim.ai calendar enabled", "tolerance", calTolerance)
	case "google":
		accessToken, _, err := config.LoadCalendarToken("google")
		if err != nil {
			return fmt.Errorf("load Google Calendar token: %w", err)
		}
		if accessToken == "" {
			return fmt.Errorf("no Google Calendar token found — run: plaud-hub auth setup-google")
		}
		calClient = googlecal.NewClient(accessToken)
		logger.Info("Google Calendar enabled", "tolerance", calTolerance)
	}

	// Build the LLM client if requested.
	var llmClient customer.LLMSplitter
	if splitLLM == "github" {
		ghToken, err := config.LoadGitHubToken()
		if err != nil {
			return fmt.Errorf("load GitHub token: %w", err)
		}
		if ghToken == "" {
			return fmt.Errorf("no GitHub token found — set github_token in config file")
		}
		llmClient = llm.NewGitHubClient(ghToken, "")
	}

	// Gather all summary files from the downloaded/ subdir.
	downloadedDir := customer.DownloadedDir(outputDir)
	summaries, err := filepath.Glob(filepath.Join(downloadedDir, "*_summary.md"))
	if err != nil {
		return fmt.Errorf("scan downloaded dir: %w", err)
	}
	if len(summaries) == 0 {
		logger.Info("no summary files found", "dir", downloadedDir)
		return nil
	}

	minRank := customer.ConfidenceRank(minConf)
	var placed, skipped int

	for _, summaryPath := range summaries {
		base := filepath.Base(summaryPath)

		// Parse recording info early — we need the timestamp for output dirs
		// and the body for LLM splitting. Falls back gracefully if parsing fails.
		recInfo, _ := customer.ParseRecordingInfo(summaryPath)
		recTime := recInfo.Start
		if recTime.IsZero() {
			recTime = time.Now()
		}

		var matches []customer.Match

		if calClient != nil {
			// Calendar path: time-window + attendee domain (high) merged with
			// body text (medium). calendarMatches handles both passes.
			calMatches, _, err := calendarMatches(cmd.Context(), calClient, registry, summaryPath, calTolerance, logger)
			if err != nil {
				logger.Warn("calendar lookup failed — falling back to text matching", "file", base, "err", err)
				// Non-fatal: fall back to text-only (use both summary and transcript).
				matches, err = customer.CorrelateFileCombined(summaryPath, registry)
				if err != nil {
					logger.Warn("skipping (parse error)", "file", base, "err", err)
					skipped++
					continue
				}
			} else {
				matches = calMatches
			}
		} else {
			// No calendar: text-only matching — use both summary and transcript.
			var err error
			matches, err = customer.CorrelateFileCombined(summaryPath, registry)
			if err != nil {
				logger.Warn("skipping (parse error)", "file", base, "err", err)
				skipped++
				continue
			}
		}

		// Filter to eligible matches.
		eligible := make([]customer.Match, 0, len(matches))
		for _, m := range matches {
			if customer.ConfidenceRank(m.Confidence) >= minRank {
				eligible = append(eligible, m)
			}
		}
		if len(eligible) == 0 {
			// No customer match — route to processed/unmatched/YYYY-MM/.
			logger.Debug("no customer match", "file", base)
			unmatchedDir := customer.UnmatchedOutputDir(outputDir, recTime)
			if mkErr := os.MkdirAll(unmatchedDir, 0o755); mkErr != nil {
				return fmt.Errorf("create unmatched dir %q: %w", unmatchedDir, mkErr)
			}
			unmatchedDest := filepath.Join(unmatchedDir, base)
			if cpErr := copyOrMoveFile(summaryPath, unmatchedDest, moveFiles); cpErr != nil {
				logger.Warn("failed to move unmatched file", "file", base, "err", cpErr)
			} else {
				logger.Info("unmatched", "file", base, "dest", unmatchedDest)
			}
			skipped++
			continue
		}

		// LLM-split path: multiple customers with an LLM configured.
		if llmClient != nil && len(eligible) > 1 {
			handled := false
			if recInfo.Body != "" {
				splits, otherBody, splitErr := customer.SplitByLLM(cmd.Context(), llmClient, recInfo.Body, eligible)
				if splitErr == nil {
					for _, sr := range splits {
						destDir := customer.CustomerOutputDir(outputDir, sr.CustomerName, recTime)
						if err := os.MkdirAll(destDir, 0o755); err != nil {
							return fmt.Errorf("create customer dir %q: %w", destDir, err)
						}

						noSuffix := strings.TrimSuffix(filepath.Base(summaryPath), "_summary.md")
						newName := fmt.Sprintf("%s_%s_summary.md", noSuffix, sr.CustomerName)
						destPath := filepath.Join(destDir, newName)

						content, buildErr := buildSplitContent(summaryPath, sr.CustomerName, sr.Body)
						if buildErr != nil {
							logger.Warn("failed to build split content", "file", base, "customer", sr.CustomerName, "err", buildErr)
							continue
						}

						if writeErr := writeTempThenRename(destPath, content); writeErr != nil {
							logger.Warn("failed to write split file", "file", base, "customer", sr.CustomerName, "err", writeErr)
							continue
						}

						logger.Info("split placed",
							"file", base,
							"customer", sr.CustomerName,
							"dest", newName,
						)
						placed++
					}

					// Write "other" (non-customer) content to processed/internal/YYYY-MM/.
					if strings.TrimSpace(otherBody) != "" {
						internalDir := customer.InternalOutputDir(outputDir, recTime)
						if mkErr := os.MkdirAll(internalDir, 0o755); mkErr != nil {
							return fmt.Errorf("create internal dir %q: %w", internalDir, mkErr)
						}
						internalDest := filepath.Join(internalDir, base)
						content, buildErr := buildLeftoverContent(summaryPath, otherBody)
						if buildErr == nil {
							if writeErr := writeTempThenRename(internalDest, content); writeErr != nil {
								logger.Warn("failed to write internal content", "file", base, "err", writeErr)
							} else {
								logger.Info("internal content placed", "file", base, "dest", internalDest)
							}
						}
					}
					if moveFiles {
						_ = os.Remove(summaryPath)
					}
					handled = true
				} else {
					logger.Warn("LLM split failed — falling back to full copy", "file", base, "err", splitErr)
				}
			} else {
				logger.Warn("failed to parse recording body for LLM split — falling back", "file", base)
			}
			if handled {
				continue
			}
		}

		// Copy (or move) summary to every matched customer folder.
		for _, m := range eligible {
			destDir := customer.CustomerOutputDir(outputDir, m.Customer.Name, recTime)
			if err := os.MkdirAll(destDir, 0o755); err != nil {
				return fmt.Errorf("create customer dir %q: %w", destDir, err)
			}

			useRename := moveFiles && len(eligible) == 1

			summaryDest := filepath.Join(destDir, filepath.Base(summaryPath))
			if err := copyOrMoveFile(summaryPath, summaryDest, useRename); err != nil {
				logger.Warn("failed to place summary", "file", base, "customer", m.Customer.Name, "err", err)
				continue
			}

			logger.Info("placed",
				"file", base,
				"customer", m.Customer.Name,
				"confidence", m.Confidence,
			)
			placed++
		}

		// Multi-customer + move: remove original after all copies.
		if moveFiles && len(eligible) > 1 {
			_ = os.Remove(summaryPath)
		}
	}

	// Clean up transcript files from downloaded/ unless the caller opted to keep them.
	if !keepTranscripts {
		transcripts, _ := filepath.Glob(filepath.Join(downloadedDir, "*_transcript.md"))
		removed := 0
		for _, tp := range transcripts {
			if err := os.Remove(tp); err != nil {
				logger.Warn("failed to remove transcript file", "file", filepath.Base(tp), "err", err)
			} else {
				removed++
			}
		}
		if removed > 0 {
			logger.Info("removed transcript files from downloaded/", "count", removed)
		}
	}

	fmt.Printf("\nCorrelation complete: %d recording(s) placed, %d skipped (no match)\n", placed, skipped)
	return nil
}

// buildSplitContent reads the original file's front matter, modifies the title
// to include the customer name with an em dash, adds a source_recording field,
// and returns the new file content with the provided split body appended.
func buildSplitContent(originalPath, customerName, splitBody string) ([]byte, error) {
	data, err := os.ReadFile(originalPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", originalPath, err)
	}

	lines := strings.Split(string(data), "\n")

	// No front matter: return split body as-is.
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return []byte(splitBody), nil
	}

	endFM := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			endFM = i
			break
		}
	}
	if endFM == -1 {
		return []byte(splitBody), nil
	}

	var sb strings.Builder
	sb.WriteString("---\n")
	for i := 1; i < endFM; i++ {
		line := lines[i]
		if strings.HasPrefix(line, "title:") {
			rest := strings.TrimPrefix(line, "title:")
			val := strings.TrimSpace(rest)
			// Strip surrounding double quotes if present.
			if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
				val = val[1 : len(val)-1]
			}
			if customerName != "" {
				fmt.Fprintf(&sb, "title: \"%s \u2014 %s\"\n", val, customerName)
			} else {
				fmt.Fprintf(&sb, "title: \"%s\"\n", val)
			}
		} else {
			sb.WriteString(line)
			sb.WriteByte('\n')
		}
	}
	fmt.Fprintf(&sb, "source_recording: %s\n", filepath.Base(originalPath))
	sb.WriteString("---\n")
	sb.WriteString(splitBody)

	return []byte(sb.String()), nil
}

// buildLeftoverContent rebuilds a file preserving its original front matter
// but replacing the body with the LLM's "other" content. Does not modify the
// title or add a source_recording field.
func buildLeftoverContent(originalPath, otherBody string) ([]byte, error) {
	data, err := os.ReadFile(originalPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", originalPath, err)
	}

	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return []byte(otherBody), nil
	}

	endFM := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			endFM = i
			break
		}
	}
	if endFM == -1 {
		return []byte(otherBody), nil
	}

	var sb strings.Builder
	for i := 0; i <= endFM; i++ {
		sb.WriteString(lines[i])
		sb.WriteByte('\n')
	}
	sb.WriteString(otherBody)
	return []byte(sb.String()), nil
}

// writeTempThenRename writes data to a temporary file in the same directory as
// dst, then atomically renames it to dst to avoid partial writes.
func writeTempThenRename(dst string, data []byte) error {
	dir := filepath.Dir(dst)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()

	if writeErr != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("write temp file: %w", writeErr)
	}
	if closeErr != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", closeErr)
	}
	if err := os.Rename(tmpName, dst); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename temp to %s: %w", dst, err)
	}
	return nil
}

// calendarMatches returns customer matches for a recording by correlating its
// timestamp window against calendar events, then boosting with body text.
//
// Confidence tiers:
//   - high:   recording window overlaps a non-all-day calendar event AND the
//     attendee's email domain maps to a known customer.
//   - medium: no calendar attendee match but the summary body text mentions
//     the customer name. Also used when a high match has a body
//     mention — the calendar result wins (already high), no change.
//
// The recording window is [date, date+duration]. A 2-minute grace period is
// applied at both ends to account for the Plaud button being pressed slightly
// before or after the calendar invite time.
//
// eventFound is true when at least one overlapping non-all-day event was found,
// even if no domain matched a known customer.
func calendarMatches(
	ctx context.Context,
	client calendarClient,
	registry *customer.Registry,
	summaryPath string,
	_ time.Duration, // tolerance flag kept for interface compatibility; grace is built-in
	logger interface {
		Warn(string, ...any)
		Debug(string, ...any)
	},
) (matches []customer.Match, eventFound bool, err error) {
	info, parseErr := customer.ParseRecordingInfo(summaryPath)
	if parseErr != nil || info.Start.IsZero() {
		return nil, false, nil
	}

	from := info.Start.Truncate(24 * time.Hour)
	to := from.Add(24 * time.Hour)

	events, err := client.ListEvents(ctx, from, to)
	if err != nil {
		return nil, false, err
	}

	const grace = 2 * time.Minute
	recStart := info.Start.UTC().Add(-grace)
	recEnd := info.End().UTC().Add(grace)

	// --- Pass 1: calendar attendee domain matching (high confidence) ---
	seenHigh := make(map[string]bool)
	for _, ev := range events {
		logger.Debug("considering event",
			"file", filepath.Base(summaryPath),
			"event", ev.Title,
			"allday", ev.AllDay,
			"ev_start", ev.Start.Format(time.RFC3339),
			"ev_end", ev.End.Format(time.RFC3339),
			"rec_start", recStart.Format(time.RFC3339),
			"rec_end", recEnd.Format(time.RFC3339),
			"attendees", len(ev.Attendees),
		)
		if ev.AllDay {
			continue
		}
		// Overlap: recording window must intersect the event window.
		if recEnd.Before(ev.Start.UTC()) || recStart.After(ev.End.UTC()) {
			continue
		}
		if len(ev.Attendees) < 2 {
			continue
		}
		eventFound = true
		logger.Debug("calendar event overlaps recording",
			"file", filepath.Base(summaryPath),
			"event", ev.Title,
			"event_start", ev.Start.Format(time.RFC3339),
			"event_end", ev.End.Format(time.RFC3339),
			"attendees", len(ev.Attendees),
		)
		for _, att := range ev.Attendees {
			domain := emailDomain(att.Email)
			if domain == "" {
				continue
			}
			c := registry.MatchDomain(domain)
			if c == nil || seenHigh[c.Name] {
				continue
			}
			seenHigh[c.Name] = true
			matches = append(matches, customer.Match{Customer: c, Confidence: customer.ConfidenceHigh})
			logger.Debug("calendar attendee match",
				"file", filepath.Base(summaryPath),
				"event", ev.Title,
				"domain", domain,
				"customer", c.Name,
			)
		}
	}

	// --- Pass 2: summary + transcript body text matching (medium confidence) ---
	// Any customer found in the body text that was NOT already matched at high
	// confidence from the calendar is added at medium confidence.
	// Include the transcript body (if present) for richer signal.
	transcriptBody := ""
	transcriptPath := strings.TrimSuffix(summaryPath, "_summary.md") + "_transcript.md"
	if tInfo, tErr := customer.ParseRecordingInfo(transcriptPath); tErr == nil {
		transcriptBody = tInfo.Body
	}
	textMatches := registry.MatchText(info.Title, info.Body+"\n"+transcriptBody)
	for _, tm := range textMatches {
		if seenHigh[tm.Customer.Name] {
			// Already captured at high; body text doesn't lower it.
			continue
		}
		logger.Debug("body text match",
			"file", filepath.Base(summaryPath),
			"customer", tm.Customer.Name,
			"confidence", tm.Confidence,
		)
		matches = append(matches, customer.Match{Customer: tm.Customer, Confidence: customer.ConfidenceMedium})
	}

	return matches, eventFound, nil
}

// recordingTitleFromPath extracts a normalized title string from the filename.
// e.g. "2026-02-24_Planning_Meeting_GitHub_Team_Governance_summary.md" → "planning meeting github team governance"
func recordingTitleFromPath(path string) string {
	base := filepath.Base(path)
	// Strip date prefix (YYYY-MM-DD_) and suffix (_summary.md or _transcript.md).
	base = strings.TrimSuffix(base, "_summary.md")
	base = strings.TrimSuffix(base, "_transcript.md")
	// Remove leading date portion if present.
	if len(base) > 10 && base[4] == '-' && base[7] == '-' {
		base = base[10:] // skip YYYY-MM-DD_
	}
	return strings.ToLower(strings.ReplaceAll(base, "_", " "))
}

// titlesOverlap returns true if the calendar event title shares at least one
// meaningful word (≥4 chars, not a stop word) with the recording title.
func titlesOverlap(recTitle, eventTitle string) bool {
	eventLower := strings.ToLower(eventTitle)
	stopWords := map[string]bool{
		"the": true, "and": true, "for": true, "with": true, "from": true,
		"this": true, "that": true, "have": true, "will": true, "your": true,
	}
	words := strings.FieldsFunc(recTitle, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for _, w := range words {
		if len(w) < 4 || stopWords[w] {
			continue
		}
		if strings.Contains(eventLower, w) {
			return true
		}
	}
	return false
}

// emailDomain extracts the domain part from an email address.
func emailDomain(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(parts[1]))
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
