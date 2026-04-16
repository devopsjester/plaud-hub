## Learnings

### 2026-04-16 — LLM integration security review (GitHub Models + splitter)

**Project patterns:**
- All `Save*` credential functions in `internal/config/` enforce `chmod 600` on the config file. Any new credential-saving function must follow this pattern; a `Load*` function without a corresponding `Save*` is a smell that warrants checking whether the token could arrive without permission enforcement.
- `CustomerOutputDir` + `SplitByLLM` use the customer registry as the source of truth for file-system paths, not LLM response keys. This structurally limits LLM path-traversal blast radius.

**Findings summary:**

| ID   | Severity | Title                                          | Status  |
|------|----------|------------------------------------------------|---------|
| F-1  | High     | Unbounded LLM response body (OOM)              | Fixed   |
| F-2  | Medium   | No SaveGitHubToken → config may be 644         | Fixed   |
| F-3  | Medium   | Prompt injection via summary body              | Documented, blast radius limited by design |
| F-4  | Low      | Go stdlib CVEs (TLS DoS, IPv6, x509) go1.24.13 | Fixed (go1.25.9) |

**Key patterns to reuse:**
- Always cap external API response bodies: `io.LimitReader(resp.Body, maxBytes)` before `json.NewDecoder(...).Decode(...)`.
- When adding a `Load*` credential function, add a matching `Save*` with `chmod 600`, and a corresponding `auth setup-*` subcommand.
- Run `govulncheck ./...` after any Go toolchain or dependency update.
