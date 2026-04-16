## Learnings

### 2026-04-16 — LLM split tests (splitter, github, correlate)

- `SplitByLLM` had no early return for empty matches; added `if len(matches) == 0 { return nil, nil }` to prevent a vacuous LLM call and make the test contract clear.
- `GitHubClient` used a hardcoded `const` URL directly in `SplitSummary`. Added a `baseURL string` field (set to the const in the constructor) so `httptest.NewServer` can be pointed at in tests — no production behaviour changed.
- For `internal/cmd` tests: calling `runCorrelate` directly (same package) with a fresh `*cobra.Command` is cleaner than executing the full Cobra command tree. Avoids fighting `MarkFlagRequired` and Viper global state while still exercising real validation logic.
- `--keep` default is tested via `Flags().Lookup("keep").DefValue` — no command execution needed, no shared-state mutation.
- Race detector `go test -race ./...` passed green; earlier interrupt was a terminal Ctrl+C, not an actual race.
