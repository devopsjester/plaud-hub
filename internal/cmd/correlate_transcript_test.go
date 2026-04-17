package cmd

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devopsjester/plaud-hub/internal/calendar"
	"github.com/devopsjester/plaud-hub/internal/customer"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

// fakeCalendarClient is a stub calendarClient for unit tests. It returns a
// fixed event list (or error) on every ListEvents call.
type fakeCalendarClient struct {
	events []calendar.CalendarEvent
	err    error
}

func (f *fakeCalendarClient) ListEvents(_ context.Context, _, _ time.Time) ([]calendar.CalendarEvent, error) {
	return f.events, f.err
}

// nopLogger satisfies the anonymous logger interface required by calendarMatches.
type nopLogger struct{}

func (nopLogger) Warn(_ string, _ ...any)  {}
func (nopLogger) Debug(_ string, _ ...any) {}

// newFullCorrelateCmd creates a fresh *cobra.Command with all flags that
// runCorrelate reads, including --keep-transcripts.
// Use this helper for integration-style tests that drive runCorrelate directly.
func newFullCorrelateCmd(customersFile string, keepTranscripts bool) *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().String("customers-file", customersFile, "")
	cmd.Flags().Bool("keep", false, "")
	cmd.Flags().String("min-confidence", "medium", "")
	cmd.Flags().String("calendar", "", "")
	cmd.Flags().Duration("calendar-tolerance", 15*time.Minute, "")
	cmd.Flags().String("split-llm", "", "")
	cmd.Flags().Bool("keep-transcripts", keepTranscripts, "")
	return cmd
}

// viperSetup configures the global viper instance for a single runCorrelate
// integration test and registers a cleanup to restore the previous state.
// Not safe for parallel use — callers must not call t.Parallel().
func viperSetup(t *testing.T, outputDir string) {
	t.Helper()
	prev := viper.GetString("output_dir")
	prevCal := viper.GetString("calendar_provider")
	viper.Set("output_dir", outputDir)
	viper.Set("calendar_provider", "") // prevent calendar path
	t.Cleanup(func() {
		viper.Set("output_dir", prev)
		viper.Set("calendar_provider", prevCal)
	})
}

// minSummaryContent is a minimal valid summary that parses cleanly but does
// not mention any recognisable customer name, so files land in unmatched/.
const minSummaryContent = "---\ntitle: \"Internal weekly sync\"\ndate: \"2026-02-24T10:00:00Z\"\nduration: \"00:30\"\n---\nGeneral housekeeping notes. No external attendees.\n"

// minTranscriptContent is a matching transcript file.
const minTranscriptContent = "---\ntitle: \"Internal weekly sync transcript\"\ndate: \"2026-02-24T10:00:00Z\"\n---\nSpeaker 1: OK let us get started.\nSpeaker 2: Sounds good.\n"

// minCustomersYAML is the smallest valid customers YAML accepted by LoadRegistry.
const minCustomersYAML = "customers:\n  - name: TestCo\n    aliases: [test company]\n    domains: [testco.com]\n"

// ---------------------------------------------------------------------------
// TestCorrelateKeepTranscriptsFlagDefault
// ---------------------------------------------------------------------------

// TestCorrelateKeepTranscriptsFlagDefault verifies that --keep-transcripts is
// registered on correlateCmd with a default value of false.
func TestCorrelateKeepTranscriptsFlagDefault(t *testing.T) {
	t.Parallel()
	f := correlateCmd.Flags().Lookup("keep-transcripts")
	if f == nil {
		t.Fatal("--keep-transcripts flag not registered on correlateCmd")
	}
	if f.DefValue != "false" {
		t.Errorf("--keep-transcripts default = %q, want \"false\"", f.DefValue)
	}
}

// ---------------------------------------------------------------------------
// calendarMatches unit tests
// ---------------------------------------------------------------------------

// buildCalSummaryDir creates a temp directory containing a _summary.md and
// optionally a _transcript.md. Returns the path to the summary file.
func buildCalSummaryDir(t *testing.T, summaryContent, transcriptContent string, writeTranscript bool) string {
	t.Helper()
	dir := t.TempDir()
	summaryPath := filepath.Join(dir, "2026-02-24_meeting_summary.md")
	if err := os.WriteFile(summaryPath, []byte(summaryContent), 0o600); err != nil {
		t.Fatalf("write summary: %v", err)
	}
	if writeTranscript {
		transcriptPath := filepath.Join(dir, "2026-02-24_meeting_transcript.md")
		if err := os.WriteFile(transcriptPath, []byte(transcriptContent), 0o600); err != nil {
			t.Fatalf("write transcript: %v", err)
		}
	}
	return summaryPath
}

// TestCalendarMatchesTranscriptBodyUsed verifies that Pass 2 of calendarMatches
// reads the sibling transcript file. A customer name present only in the
// transcript body (not in the summary) must appear in the returned matches.
func TestCalendarMatchesTranscriptBodyUsed(t *testing.T) {
	t.Parallel()

	reg := &customer.Registry{Customers: []customer.Customer{
		{Name: "Acme"},
		{Name: "Beta"},
	}}

	// Summary body mentions only "Acme"; transcript body mentions only "Beta".
	const summaryContent = "---\ntitle: \"Test Meeting\"\ndate: \"2026-02-24T10:00:00Z\"\nduration: \"00:30\"\n---\nThe Acme team reviewed their roadmap.\n"
	const transcriptContent = "---\ntitle: \"Test Meeting Transcript\"\ndate: \"2026-02-24T10:00:00Z\"\n---\nBeta corporation representatives joined the session.\n"

	summaryPath := buildCalSummaryDir(t, summaryContent, transcriptContent, true)

	// Calendar returns no events — no attendee matches. Pass 2 still executes.
	cal := &fakeCalendarClient{events: nil}

	matches, _, err := calendarMatches(
		context.Background(), cal, reg, summaryPath,
		15*time.Minute, nopLogger{},
	)
	if err != nil {
		t.Fatalf("calendarMatches error: %v", err)
	}

	found := make(map[string]bool, len(matches))
	for _, m := range matches {
		found[m.Customer.Name] = true
	}
	if !found["Acme"] {
		t.Error("expected Acme in matches (from summary body), not found")
	}
	if !found["Beta"] {
		t.Error("expected Beta in matches (from transcript body), not found")
	}
}

// TestCalendarMatchesMissingTranscript verifies that the absence of a transcript
// file does not cause calendarMatches to return an error. Summary-body matches
// must still be returned; transcript-only customers must not appear.
func TestCalendarMatchesMissingTranscript(t *testing.T) {
	t.Parallel()

	reg := &customer.Registry{Customers: []customer.Customer{
		{Name: "Acme"},
		{Name: "Beta"},
	}}

	// Summary body mentions "Acme" only. No transcript file will be written.
	const summaryContent = "---\ntitle: \"Test Meeting\"\ndate: \"2026-02-24T10:00:00Z\"\nduration: \"00:30\"\n---\nThe Acme team reviewed their roadmap.\n"

	summaryPath := buildCalSummaryDir(t, summaryContent, "", false /* no transcript */)

	cal := &fakeCalendarClient{events: nil}

	matches, _, err := calendarMatches(
		context.Background(), cal, reg, summaryPath,
		15*time.Minute, nopLogger{},
	)
	if err != nil {
		t.Fatalf("calendarMatches returned unexpected error: %v", err)
	}

	found := make(map[string]bool, len(matches))
	for _, m := range matches {
		found[m.Customer.Name] = true
	}
	if !found["Acme"] {
		t.Error("expected Acme in matches (from summary body), not found")
	}
	if found["Beta"] {
		t.Error("Beta should not appear in matches when transcript file is absent")
	}
}

// ---------------------------------------------------------------------------
// Transcript deletion integration tests
// ---------------------------------------------------------------------------

// TestTranscriptDeletion_DeletedByDefault confirms that *_transcript.md files
// in downloaded/ are removed after runCorrelate when --keep-transcripts is not
// set (its default is false).
func TestTranscriptDeletion_DeletedByDefault(t *testing.T) {
	// Not parallel: uses global viper state.
	outputDir := t.TempDir()
	downloadedDir := filepath.Join(outputDir, "downloaded")
	if err := os.MkdirAll(downloadedDir, 0o755); err != nil {
		t.Fatalf("create downloaded dir: %v", err)
	}

	summaryPath := filepath.Join(downloadedDir, "2026-02-24_meeting_summary.md")
	transcriptPath := filepath.Join(downloadedDir, "2026-02-24_meeting_transcript.md")

	if err := os.WriteFile(summaryPath, []byte(minSummaryContent), 0o600); err != nil {
		t.Fatalf("write summary: %v", err)
	}
	if err := os.WriteFile(transcriptPath, []byte(minTranscriptContent), 0o600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	customersFile := filepath.Join(t.TempDir(), "customers.yaml")
	if err := os.WriteFile(customersFile, []byte(minCustomersYAML), 0o600); err != nil {
		t.Fatalf("write customers file: %v", err)
	}

	viperSetup(t, outputDir)

	cmd := newFullCorrelateCmd(customersFile, false /* keepTranscripts=false */)
	if err := runCorrelate(cmd, nil); err != nil {
		t.Fatalf("runCorrelate error: %v", err)
	}

	if _, statErr := os.Stat(transcriptPath); !os.IsNotExist(statErr) {
		t.Errorf("transcript file should have been deleted but still exists: %s", transcriptPath)
	}
}

// TestTranscriptDeletion_KeptWhenFlagSet confirms that *_transcript.md files
// are retained when --keep-transcripts=true is passed to runCorrelate.
func TestTranscriptDeletion_KeptWhenFlagSet(t *testing.T) {
	// Not parallel: uses global viper state.
	outputDir := t.TempDir()
	downloadedDir := filepath.Join(outputDir, "downloaded")
	if err := os.MkdirAll(downloadedDir, 0o755); err != nil {
		t.Fatalf("create downloaded dir: %v", err)
	}

	summaryPath := filepath.Join(downloadedDir, "2026-02-24_meeting_summary.md")
	transcriptPath := filepath.Join(downloadedDir, "2026-02-24_meeting_transcript.md")

	if err := os.WriteFile(summaryPath, []byte(minSummaryContent), 0o600); err != nil {
		t.Fatalf("write summary: %v", err)
	}
	if err := os.WriteFile(transcriptPath, []byte(minTranscriptContent), 0o600); err != nil {
		t.Fatalf("write transcript: %v", err)
	}

	customersFile := filepath.Join(t.TempDir(), "customers.yaml")
	if err := os.WriteFile(customersFile, []byte(minCustomersYAML), 0o600); err != nil {
		t.Fatalf("write customers file: %v", err)
	}

	viperSetup(t, outputDir)

	cmd := newFullCorrelateCmd(customersFile, true /* keepTranscripts=true */)
	if err := runCorrelate(cmd, nil); err != nil {
		t.Fatalf("runCorrelate error: %v", err)
	}

	if _, statErr := os.Stat(transcriptPath); os.IsNotExist(statErr) {
		t.Errorf("transcript file should have been kept but was deleted: %s", transcriptPath)
	}
}
