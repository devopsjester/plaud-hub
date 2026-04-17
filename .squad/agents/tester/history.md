## Learnings

### 2026-04-17 — Calendar-only routing tests (correlate_transcript_test.go)

- Calendar agent updated `calendarMatches` signature to `(matches, eventFound, githubOnly bool, err)` and removed Pass 2 (body text matching). All test call sites updated from 3-value to 4-value unpacking.
- `TestCalendarMatchesTranscriptBodyUsed` was deleted (Pass 2 no longer exists); replaced with `TestCalendarMatchesAttendeeHighConfidence` that proves attendee domain → high-confidence customer match while deliberately putting a *different* customer name in the body (proves body is not consulted).
- `TestCalendarMatchesMissingTranscript` was deleted (body text path gone); replaced with `TestCalendarMatchesGitHubOnlyEvent` asserting `githubOnly=true/eventFound=true/matches=empty` when all attendees are @github.com.
- Added `TestCalendarMatchesNoEvent` for the `eventFound=false/githubOnly=false/matches=empty` path when the fake calendar returns no events.
- `newFullCorrelateCmd` flag default changed from `""` to `"reclaim"` to match the updated `correlateCmd` registration. Integration tests remain correct because `viperSetup` sets `calendar_provider=""` via viper, overriding the flag default, so `calClient == nil` → file routes to unmatched (transcript-deletion assertions still hold).
- When a Calendar agent modifies a calendarMatches signature in parallel: read the production file first before writing any test; the Calendar agent had already committed all changes by the time tests were authored.



### 2026-04-16 — Transcript deletion & calendar body tests (correlate_transcript_test.go)

- For `calendarMatches` unit tests a minimal `fakeCalendarClient` struct implementing the `calendarClient` interface is enough — no need for mocks frameworks; the anonymous logger interface is satisfied by a zero-value `nopLogger` struct.
- `calendarMatches` Pass 2 (body text matching) runs unconditionally regardless of whether any calendar event was found; testing with an empty event list is sufficient to exercise the transcript-body read path.
- For integration-style `runCorrelate` tests: `runCorrelate` reads `output_dir` from global viper, not from a command flag directly. Use `viper.Set("output_dir", tmpDir)` + `t.Cleanup` to restore state. Also set `calendar_provider` to `""` to prevent calendar API key lookups. These tests must NOT call `t.Parallel()`.
- Spread new tests across a separate `_test.go` file (idiomatic Go) rather than appending to the existing file, to avoid needing to rewrite the import block.
- `containsWord` uses word-boundary checks, so `"Beta corporation"` correctly matches customer `"Beta"`.

### 2026-04-16 — LLM split tests (splitter, github, correlate)

- `SplitByLLM` had no early return for empty matches; added `if len(matches) == 0 { return nil, nil }` to prevent a vacuous LLM call and make the test contract clear.
- `GitHubClient` used a hardcoded `const` URL directly in `SplitSummary`. Added a `baseURL string` field (set to the const in the constructor) so `httptest.NewServer` can be pointed at in tests — no production behaviour changed.
- For `internal/cmd` tests: calling `runCorrelate` directly (same package) with a fresh `*cobra.Command` is cleaner than executing the full Cobra command tree. Avoids fighting `MarkFlagRequired` and Viper global state while still exercising real validation logic.
- `--keep` default is tested via `Flags().Lookup("keep").DefValue` — no command execution needed, no shared-state mutation.
- Race detector `go test -race ./...` passed green; earlier interrupt was a terminal Ctrl+C, not an actual race.
