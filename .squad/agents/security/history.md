## Learnings

### 2026-04-16 — LLM integration security review (GitHub Models + splitter)

**Project patterns:**

- All `Save*` credential functions in `internal/config/` enforce `chmod 600` on the config file. Any new credential-saving function must follow this pattern; a `Load*` function without a corresponding `Save*` is a smell that warrants checking whether the token could arrive without permission enforcement.
- `CustomerOutputDir` + `SplitByLLM` use the customer registry as the source of truth for file-system paths, not LLM response keys. This structurally limits LLM path-traversal blast radius.

**Findings summary:**

| ID  | Severity | Title                                          | Status                                     |
| --- | -------- | ---------------------------------------------- | ------------------------------------------ |
| F-1 | High     | Unbounded LLM response body (OOM)              | Fixed                                      |
| F-2 | Medium   | No SaveGitHubToken → config may be 644         | Fixed                                      |
| F-3 | Medium   | Prompt injection via summary body              | Documented, blast radius limited by design |
| F-4 | Low      | Go stdlib CVEs (TLS DoS, IPv6, x509) go1.24.13 | Fixed (go1.25.9)                           |

**Key patterns to reuse:**

- Always cap external API response bodies: `io.LimitReader(resp.Body, maxBytes)` before `json.NewDecoder(...).Decode(...)`.
- When adding a `Load*` credential function, add a matching `Save*` with `chmod 600`, and a corresponding `auth setup-*` subcommand.
- Run `govulncheck ./...` after any Go toolchain or dependency update.

---

### 2026-04-16 — Transcript path derivation and deletion loop review (commit c1bd2f1)

**Project patterns confirmed:**

- `filepath.Glob` with a single `*` never matches `/` — paths returned are always direct children of the base directory. String manipulation on those paths (e.g., suffix swap) cannot escape the directory. This is a reliable structural constraint for path traversal review.
- `os.Remove` on a symlink removes the symlink entry, not the target. Symlink deletion attacks against `os.Remove` loops are not possible on Linux/macOS.

**Findings summary:**

| ID  | Severity    | Title                                       | Status               |
|-----|-------------|---------------------------------------------|----------------------|
| F-1 | Low         | Silent transcript deletion failure          | Fix recommended      |
| F-2 | Theoretical | Symlink read via ParseRecordingInfo         | Accepted/documented  |

**Key patterns to reuse:**

- Silence `os.Remove` only when the deletion truly has no observable consequence. For staging files that may contain sensitive content, log failures at warn level so operators know when intended cleanup did not complete.
- When a function opens a file that originates from user-controlled directory contents (even indirectly), document the symlink assumption. For single-user CLIs it is acceptable; for shared deployments add `os.Lstat` + `fi.Mode()&os.ModeSymlink` guard before `os.Open`.
- `govulncheck` must be invoked as `go run golang.org/x/vuln/cmd/govulncheck@latest ./...` in CI to avoid local PATH/install issues. Local binary installs under `$(go env GOPATH)/bin` are unreliable if that directory is not in `$PATH`.
