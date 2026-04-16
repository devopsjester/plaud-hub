# Squad Decisions

## Active Decisions

---

### [2026-04-16] correlate: `--keep` replaces `--move`; `--split-llm github` added

**Author:** Customer + Documenter  
**Status:** Accepted

- Default behavior of `correlate` is now **move** (output root is cleared after correlation). The old `--move` flag is removed.
- `--keep` flag (bool, default `false`) preserves originals in the output root while still copying to customer folders.
- `--split-llm github` routes multi-customer summaries through GitHub Models (gpt-4o-mini). Each customer receives a separately named file containing only their extracted content.
- Only `"github"` is a valid `--split-llm` value.
- LLM split falls back to full-copy on any error. No recordings go unplaced.
- Split files are named `{recording}_{CustomerName}_summary.md`; front matter gains `source_recording:` field.
- `LLMSplitter` interface lives in `internal/customer/splitter.go`; `internal/llm` is infrastructure only.
- `github_token` is a top-level key in `plaud-hub.yaml` (not nested under `calendar:`).

---

### [2026-04-16] Security: LLM integration hardening

**Author:** Security  
**Status:** Accepted

- **F-1 (HIGH, FIXED):** `io.LimitReader(resp.Body, 2 MiB)` added to `internal/llm/github.go` before JSON decode. Prevents OOM from unbounded LLM response bodies.
- **F-2 (MEDIUM, FIXED):** `SaveGitHubToken` added to `internal/config/calendar.go` with `os.Chmod(path, 0o600)`. `auth setup-github` subcommand added so token is never written manually.
- **F-3 (MEDIUM, BY DESIGN):** Prompt injection blast radius is structurally limited — `SplitByLLM` uses only trusted registry customer names as map keys. Future hardening: cap summary body to 64 KB before LLM submission (product decision pending).
- **F-4 (LOW, FIXED):** `go.mod` updated to `go 1.25.9` / `toolchain go1.25.9` to address GO-2026-4870 (crypto/tls DoS), GO-2026-4601 (net/url IPv6), GO-2026-4947, GO-2026-4946 (crypto/x509).

---

### [2026-04-16] Testing: approach for LLM split feature

**Author:** Tester  
**Status:** Accepted

- `GitHubClient.baseURL` field added to production struct for `httptest.NewServer` testability (standard Go pattern).
- `SplitByLLM` returns early `(nil, nil)` when `matches` is empty — avoids wasteful LLM calls.
- `cmd` tests call `runCorrelate` directly (same package), not via `cobra.Execute`, to avoid `MarkFlagRequired` friction.
- No third-party test libraries. All mocks inline in `_test.go` files using standard `testing` package.

---

### [2026-04-15] Calendar API client and auth design

**Author:** Calendar  
**Status:** Proposed (pending Architect review)

- All `time.Time` values in `internal/calendar/` stored and compared in UTC. M365 Graph requests include `Prefer: outlook.timezone="UTC"` to eliminate DST ambiguity.
- Both providers use OAuth 2.0 device-code flow — works in headless/SSH/terminal environments, no local redirect listener required.
- No external Microsoft Graph SDK or Google API client library — stdlib `net/http` + `encoding/json` only.
- Tokens stored in shared `plaud-hub.yaml` config file at `chmod 600`. Viper keys: `calendar.m365.*`, `calendar.google.*`.
- No hardcoded client IDs or secrets; all supplied by callers at runtime.

**Open questions for Architect:**
1. Token refresh: automatic (transparent 401 retry) or explicit functions?
2. Pagination: `@odata.nextLink` (Graph) / `nextPageToken` (Google) not yet implemented.
3. M365 tenant scope: `/common` endpoint in use — make `tenant_id` configurable?
4. Client ID storage: config file (`calendar.m365.client_id` / `calendar.google.client_id`) or always CLI flags?
5. `slow_down` polling: currently treated as `authorization_pending`; spec requires +5s interval increase.

---

## Governance

- All meaningful changes require team consensus
- Document architectural decisions here
- Keep history focused on work, decisions focused on direction
