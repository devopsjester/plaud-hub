## Learnings

- `--move` → `--keep` inversion: the flag polarity flip (`moveFiles := !keepFiles`) is straightforward but be careful that the single-customer `useRename` path and the multi-customer post-loop `os.Remove` both reference `moveFiles`, not the flag directly — they were correct without further change.
- `parseFrontMatter` is unexported in the `customer` package; raw line-by-line parsing of the YAML front matter is the correct approach for the `buildSplitContent` helper in `correlate.go`.
- The `LLMSplitter` interface lives in `internal/customer/splitter.go` so tests can mock it without importing the `llm` package.
- `*llm.GitHubClient` satisfies `customer.LLMSplitter` implicitly — no explicit `var _ customer.LLMSplitter = (*llm.GitHubClient)(nil)` assertion needed (though one could be added in tests).
- Atomic writes via `os.CreateTemp` + `os.Rename` are the correct pattern for all new file outputs; reused `writeTempThenRename` helper in `correlate.go`.
- GitHub token config key is `github_token` at YAML root (not nested under `calendar:`), following the same Viper pattern as `LoadReclaimKey`.
