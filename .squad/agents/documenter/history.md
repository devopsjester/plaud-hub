## Learnings

### 2026-04-16 — correlate flag refactor

- Always read the `Long` Cobra string before writing docs — it was already accurate and needed no changes.
- `LoadGitHubToken` lives in `internal/config/calendar.go`, reads the top-level `github_token` key (not nested under `calendar:`).
- `--split-llm` only accepts `"github"`; document the constraint explicitly.
- Move is now the default for `correlate`; `--keep` is the opt-out. Do not describe `--keep` as replacing `--move` in user-facing docs — describe the default behavior and what the flag changes.
- No interactive auth command exists for the GitHub token; document config-file-only setup.

### 2026-04-16 — transcript staging lifecycle docs

- The `correlate` Long description already had "Both the summary and transcript are searched for customer names" — always read source before writing, avoid duplicating accurate text.
- Add transcript deletion behavior as a separate paragraph in the Long description so it appears in `--help` output.
- In README, a blockquote callout under the Correlate intro is more visible than burying the behaviour in the flag table alone — use both: callout for the workflow, flag table row for the opt-out flag.
- `--keep-transcripts` flag description in code: "keep transcript files in downloaded/ after correlation (default is to delete them)". README flag table entry: slightly shorter — "Keep transcript files in `downloaded/` after correlation (default: deleted)".
- No changes to `download.go` — the `--type` flag description ("content type: transcript, summary, or all") is accurate; the staging lifecycle belongs in correlate docs.
