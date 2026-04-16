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
and copies (or moves) the files into output/customers/{CustomerName}/ subfolders.

Both the summary and transcript are searched for customer names. When
--calendar reclaim or --calendar google is specified, calendar events are fetched
for each recording's date; attendee email domains are matched against the
customers file to confirm or add customer matches.

When a recording matches multiple customers, files are copied to each folder.
Use --move to remove the originals from the output root after copying.`,
	RunE: runCorrelate,
}

func init() {
	rootCmd.AddCommand(correlateCmd)
	correlateCmd.Flags().String("output-dir", config.DefaultOutputDir, "directory containing downloaded files")
	correlateCmd.Flags().String("customers-file", "", "path to customer registry YAML file (required)")
	correlateCmd.Flags().Bool("move", false, "move files instead of copying (removes originals from output root)")
	correlateCmd.Flags().String("min-confidence", customer.ConfidenceMedium, "minimum confidence level to act on: high, medium, or low")
	correlateCmd.Flags().String("calendar", "", "confirm matches via calendar attendees: reclaim or google")
	correlateCmd.Flags().Duration("calendar-tolerance", 15*time.Minute, "time window around recording start to search for a matching calendar event")

	_ = correlateCmd.MarkFlagRequired("customers-file")
	_ = viper.BindPFlag("output_dir", correlateCmd.Flags().Lookup("output-dir"))
}

func runCorrelate(cmd *cobra.Command, _ []string) error {
	logger := newLogger()

	outputDir := viper.GetString("output_dir")
	customersFile, _ := cmd.Flags().GetString("customers-file")
	moveFiles, _ := cmd.Flags().GetBool("move")
	minConf, _ := cmd.Flags().GetString("min-confidence")
	calProvider, _ := cmd.Flags().GetString("calendar")
	calTolerance, _ := cmd.Flags().GetDuration("calendar-tolerance")

	if customer.ConfidenceRank(minConf) == 0 {
		return fmt.Errorf("invalid --min-confidence %q: must be high, medium, or low", minConf)
	}
	if calProvider != "" && calProvider != "google" && calProvider != "reclaim" {
		return fmt.Errorf("invalid --calendar %q: must be \"google\" or \"reclaim\"", calProvider)
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
		base := filepath.Base(summaryPath)

		var matches []customer.Match

		if calClient != nil {
			// Calendar path: time-window + attendee domain (high) merged with
			// body text (medium). calendarMatches handles both passes.
			calMatches, _, err := calendarMatches(cmd.Context(), calClient, registry, summaryPath, calTolerance, logger)
			if err != nil {
				logger.Warn("calendar lookup failed — falling back to text matching", "file", base, "err", err)
				// Non-fatal: fall back to text-only.
				matches, err = customer.CorrelateFile(summaryPath, registry)
				if err != nil {
					logger.Warn("skipping (parse error)", "file", base, "err", err)
					skipped++
					continue
				}
			} else {
				matches = calMatches
			}
		} else {
			// No calendar: text-only matching.
			var err error
			matches, err = customer.CorrelateFile(summaryPath, registry)
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
			logger.Debug("no customer match", "file", base)
			skipped++
			continue
		}

		// Copy (or move) summary to every matched customer folder.
		for _, m := range eligible {
			destDir := customer.CustomerOutputDir(outputDir, m.Customer.Name)
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

		// Multi-customer + --move: remove original after all copies.
		if moveFiles && len(eligible) > 1 {
			_ = os.Remove(summaryPath)
		}
	}

	fmt.Printf("\nCorrelation complete: %d recording(s) placed, %d skipped (no match)\n", placed, skipped)
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

	// --- Pass 2: summary body text matching (medium confidence) ---
	// Any customer found in the body text that was NOT already matched at high
	// confidence from the calendar is added at medium confidence.
	textMatches := registry.MatchText(info.Title, info.Body)
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
