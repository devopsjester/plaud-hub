package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/devopsjester/plaud-hub/internal/calendar"
	reclaimcal "github.com/devopsjester/plaud-hub/internal/calendar/reclaim"
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
// It spins up a local HTTP stub that returns empty Reclaim events so tests
// never hit the real Reclaim API.
// Not safe for parallel use — callers must not call t.Parallel().
func viperSetup(t *testing.T, outputDir string) {
	t.Helper()

	// Stub the Reclaim API so tests never make real network calls.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	t.Cleanup(srv.Close)

	prevBaseURL := reclaimcal.BaseURL
	reclaimcal.BaseURL = srv.URL
	t.Cleanup(func() { reclaimcal.BaseURL = prevBaseURL })

	prev := viper.GetString("output_dir")
	prevCal := viper.GetString("calendar_provider")
	prevKey := viper.GetString("calendar.reclaim.api_key")
	viper.Set("output_dir", outputDir)
	viper.Set("calendar_provider", "reclaim")
	viper.Set("calendar.reclaim.api_key", "test-stub-key")
	t.Cleanup(func() {
		viper.Set("output_dir", prev)
		viper.Set("calendar_provider", prevCal)
		viper.Set("calendar.reclaim.api_key", prevKey)
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

// TestCalendarMatchesAttendeeHighConfidence verifies that when a calendar event
// overlaps the recording and an attendee's domain matches a known customer,
// that customer appears in matches at high confidence. Body text is not used —
// the summary deliberately mentions a different customer name.
func TestCalendarMatchesAttendeeHighConfidence(t *testing.T) {
	t.Parallel()

	reg := &customer.Registry{Customers: []customer.Customer{
		{Name: "TestCo", Domains: []string{"testco.com"}},
	}}

	// Summary body mentions "Acme" only — body text must NOT drive matching.
	const summaryContent = "---\ntitle: \"TestCo planning\"\ndate: \"2026-02-24T10:00:00Z\"\nduration: \"30:00\"\n---\nThe Acme team reviewed their roadmap. No mention of TestCo here.\n"

	summaryPath := buildCalSummaryDir(t, summaryContent, "", false)

	cal := &fakeCalendarClient{events: []calendar.CalendarEvent{
		{
			Title:  "TestCo planning",
			Start:  time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC),
			End:    time.Date(2026, 2, 24, 11, 0, 0, 0, time.UTC),
			AllDay: false,
			Attendees: []calendar.Attendee{
				{Email: "alice@testco.com"},
				{Email: "bob@github.com"},
			},
		},
	}}

	matches, eventFound, githubOnly, err := calendarMatches(
		context.Background(), cal, reg, summaryPath,
		15*time.Minute, nopLogger{},
	)
	if err != nil {
		t.Fatalf("calendarMatches error: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("len(matches) = %d, want 1", len(matches))
	}
	if matches[0].Customer.Name != "TestCo" {
		t.Errorf("matches[0].Customer.Name = %q, want \"TestCo\"", matches[0].Customer.Name)
	}
	if matches[0].Confidence != customer.ConfidenceHigh {
		t.Errorf("matches[0].Confidence = %q, want %q", matches[0].Confidence, customer.ConfidenceHigh)
	}
	if githubOnly {
		t.Error("githubOnly = true, want false")
	}
	if !eventFound {
		t.Error("eventFound = false, want true")
	}
}

// TestCalendarMatchesGitHubOnlyEvent verifies that when the only overlapping
// calendar event has attendees exclusively from github.com, calendarMatches
// returns githubOnly=true, eventFound=true, and an empty matches slice.
func TestCalendarMatchesGitHubOnlyEvent(t *testing.T) {
	t.Parallel()

	reg := &customer.Registry{Customers: []customer.Customer{
		{Name: "TestCo", Domains: []string{"testco.com"}},
	}}

	const summaryContent = "---\ntitle: \"Internal GitHub sync\"\ndate: \"2026-02-24T10:00:00Z\"\nduration: \"30:00\"\n---\nInternal team discussion.\n"

	summaryPath := buildCalSummaryDir(t, summaryContent, "", false)

	cal := &fakeCalendarClient{events: []calendar.CalendarEvent{
		{
			Title:  "Internal GitHub sync",
			Start:  time.Date(2026, 2, 24, 10, 0, 0, 0, time.UTC),
			End:    time.Date(2026, 2, 24, 11, 0, 0, 0, time.UTC),
			AllDay: false,
			Attendees: []calendar.Attendee{
				{Email: "alice@github.com"},
				{Email: "bob@github.com"},
			},
		},
	}}

	matches, eventFound, githubOnly, err := calendarMatches(
		context.Background(), cal, reg, summaryPath,
		15*time.Minute, nopLogger{},
	)
	if err != nil {
		t.Fatalf("calendarMatches error: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("len(matches) = %d, want 0", len(matches))
	}
	if !eventFound {
		t.Error("eventFound = false, want true")
	}
	if !githubOnly {
		t.Error("githubOnly = false, want true")
	}
}

// TestCalendarMatchesNoEvent verifies that when the calendar returns no
// overlapping events, calendarMatches returns empty matches, eventFound=false,
// githubOnly=false, and no error.
func TestCalendarMatchesNoEvent(t *testing.T) {
	t.Parallel()

	reg := &customer.Registry{Customers: []customer.Customer{
		{Name: "TestCo", Domains: []string{"testco.com"}},
	}}

	const summaryContent = "---\ntitle: \"Solo session\"\ndate: \"2026-02-24T10:00:00Z\"\nduration: \"30:00\"\n---\nNo calendar event expected.\n"

	summaryPath := buildCalSummaryDir(t, summaryContent, "", false)

	cal := &fakeCalendarClient{events: nil}

	matches, eventFound, githubOnly, err := calendarMatches(
		context.Background(), cal, reg, summaryPath,
		15*time.Minute, nopLogger{},
	)
	if err != nil {
		t.Fatalf("calendarMatches error: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("len(matches) = %d, want 0", len(matches))
	}
	if eventFound {
		t.Error("eventFound = true, want false")
	}
	if githubOnly {
		t.Error("githubOnly = true, want false")
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

// ---------------------------------------------------------------------------
// TestSplitMarker_MultiCustomerNoLLM
// ---------------------------------------------------------------------------

// multiCustomersYAML contains two customers so a single calendar event with
// attendees from both domains triggers a multi-customer match.
const multiCustomersYAML = "customers:\n" +
	"  - name: Acme\n    aliases: [acme corp]\n    domains: [acme.com]\n" +
	"  - name: Globex\n    aliases: [globex corp]\n    domains: [globex.com]\n"

// multiCustomerSummary is a valid summary whose date window will be matched by
// the stub Reclaim server events in TestSplitMarker_MultiCustomerNoLLM.
const multiCustomerSummaryContent = "---\ntitle: \"Joint planning session\"\ndate: \"2026-02-24T10:00:00Z\"\nduration: \"01:00\"\n---\nNotes from the joint planning session.\n"

// TestSplitMarker_MultiCustomerNoLLM verifies that when a recording matches
// two customer domains and no --split-llm is configured, the output files in
// each customer folder are renamed with a _SPLIT_ marker so the user knows
// manual splitting is required.
func TestSplitMarker_MultiCustomerNoLLM(t *testing.T) {
	// Not parallel: uses global viper state.
	outputDir := t.TempDir()
	downloadedDir := filepath.Join(outputDir, "downloaded")
	if err := os.MkdirAll(downloadedDir, 0o755); err != nil {
		t.Fatalf("create downloaded dir: %v", err)
	}

	summaryPath := filepath.Join(downloadedDir, "2026-02-24_meeting_summary.md")
	if err := os.WriteFile(summaryPath, []byte(multiCustomerSummaryContent), 0o600); err != nil {
		t.Fatalf("write summary: %v", err)
	}

	customersFile := filepath.Join(t.TempDir(), "customers.yaml")
	if err := os.WriteFile(customersFile, []byte(multiCustomersYAML), 0o600); err != nil {
		t.Fatalf("write customers file: %v", err)
	}

	// Stub Reclaim to return one event overlapping the recording window with
	// attendees from both customer domains.
	srv := newSplitMarkerStubServer(t)
	defer srv.Close()

	prevBaseURL := reclaimcal.BaseURL
	reclaimcal.BaseURL = srv.URL
	defer func() { reclaimcal.BaseURL = prevBaseURL }()

	prev := viper.GetString("output_dir")
	prevCal := viper.GetString("calendar_provider")
	prevKey := viper.GetString("calendar.reclaim.api_key")
	viper.Set("output_dir", outputDir)
	viper.Set("calendar_provider", "reclaim")
	viper.Set("calendar.reclaim.api_key", "test-stub-key")
	defer func() {
		viper.Set("output_dir", prev)
		viper.Set("calendar_provider", prevCal)
		viper.Set("calendar.reclaim.api_key", prevKey)
	}()

	cmd := newFullCorrelateCmd(customersFile, true)
	if err := runCorrelate(cmd, nil); err != nil {
		t.Fatalf("runCorrelate error: %v", err)
	}

	// Both customer folders should have the _SPLIT_ marker.
	for _, cust := range []string{"Acme", "Globex"} {
		pattern := filepath.Join(outputDir, "processed", "customers", cust, "*", "*_SPLIT_summary.md")
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("glob %s: %v", pattern, err)
		}
		if len(matches) != 1 {
			t.Errorf("customer %s: want 1 _SPLIT_summary.md file, got %d (pattern: %s)", cust, len(matches), pattern)
		}
	}
}

// newSplitMarkerStubServer returns an httptest.Server that replies with a
// Reclaim-shaped JSON response containing one event overlapping 2026-02-24
// with attendees from acme.com and globex.com.
func newSplitMarkerStubServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{
			"eventId": "evt1",
			"title": "Joint planning",
			"eventStart": "2026-02-24T10:00:00Z",
			"eventEnd": "2026-02-24T11:00:00Z",
			"allDay": false,
			"attendees": [
				{"email": "alice@acme.com"},
				{"email": "bob@globex.com"}
			]
		}]`))
	}))
}

