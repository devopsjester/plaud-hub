# plaud-hub — Workspace Instructions

## Project Overview

A Go CLI tool that downloads Plaud Note transcripts and AI summaries as Markdown files with YAML front matter, and correlates them with calendar events, customers, and external stores.

**Stack:** Go 1.24, Cobra (CLI), Viper (config), mpb (progress bar)
**Entry point:** `./cmd/plaud-hub/main.go`
**Output format:** Markdown with YAML front matter, compatible with Obsidian

## Project Layout

```
cmd/plaud-hub/          CLI entry point
internal/api/           Plaud API client, models, endpoints
internal/cmd/           Cobra command definitions
internal/config/        Viper-based config loading and token storage
internal/download/      Download orchestration, filename sanitization, file writing
```

## Configuration

- Config file: `{os.UserConfigDir()}/plaud-hub/plaud-hub.yaml` (macOS: `~/Library/Application Support/plaud-hub/plaud-hub.yaml`)
- Config file permissions: chmod 600
- Token lookup precedence: `--token` flag → `PLAUD_TOKEN` env → config file

## Key Bugs Fixed (do not reintroduce)

- `Accept-Encoding` header removed from `internal/api/client.go` — Go's HTTP client handles gzip automatically when the header is absent
- Config filename is `plaud-hub.yaml`, not `config.yaml` — read and write paths must match
- `viper.SetConfigType("yaml")` is only applied when an explicit config file path is given, not during path-based search (prevents Viper from parsing the compiled binary as YAML)

## Planned Feature Areas

| Feature                 | Status  | Description                                                               |
| ----------------------- | ------- | ------------------------------------------------------------------------- |
| Calendar correlation    | Planned | Match recordings to M365 / Google Calendar events by time window          |
| Customer identification | Planned | Identify related customers from calendar attendees and transcript content |
| Distribution            | Planned | Push summaries to GitHub repo, Gainsight (MCP), SharePoint/OneDrive       |

## Security Baseline

- Secrets never committed; tokens stored in OS config dir at chmod 600
- TLS only for all external HTTP calls
- All output file paths sanitized against path traversal
- No shell injection via `exec`
