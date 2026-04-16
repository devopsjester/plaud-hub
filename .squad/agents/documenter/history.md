## Learnings

### 2026-04-16 — correlate flag refactor

- Always read the `Long` Cobra string before writing docs — it was already accurate and needed no changes.
- `LoadGitHubToken` lives in `internal/config/calendar.go`, reads the top-level `github_token` key (not nested under `calendar:`).
- `--split-llm` only accepts `"github"`; document the constraint explicitly.
- Move is now the default for `correlate`; `--keep` is the opt-out. Do not describe `--keep` as replacing `--move` in user-facing docs — describe the default behavior and what the flag changes.
- No interactive auth command exists for the GitHub token; document config-file-only setup.
