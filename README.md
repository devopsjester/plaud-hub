# plaud-hub

A CLI tool to download, correlate, and distribute Plaud Note transcripts and AI summaries as Markdown files with YAML front matter.

## Features

- Download transcripts, summaries, or both (default: both)
- Date range filtering (`--from`, `--to`)
- Bounded concurrent downloads with progress bar (mpb)
- Skip existing files by default (`--force` to re-download)
- Hand-rolled HTTP retry with `Retry-After` header support
- Sanitized filenames with hash-based truncation for long titles
- YAML front matter for Obsidian/static-site compatibility
- Configurable via CLI flags, environment variables, or YAML config file

## Installation

```bash
go install github.com/devopsjester/plaud-hub/cmd/plaud-hub@latest
```

Or build from source:

```bash
git clone https://github.com/devopsjester/plaud-hub.git
cd plaud-hub
go build -o plaud-hub ./cmd/plaud-hub
```

## Authentication

Get your Plaud API token:

1. Open [web.plaud.ai](https://web.plaud.ai) and sign in
2. Open DevTools → Network tab → find any request to `api.plaud.ai`
3. Copy the `Authorization` header value (without the "bearer " prefix)

Then set it up (choose one):

```bash
# Interactive setup (saves to ~/Library/Application Support/plaud-hub/plaud-hub.yaml)
plaud-hub auth setup

# Environment variable
export PLAUD_TOKEN='your-token-here'

# CLI flag
plaud-hub download --token 'your-token-here'
```

Token resolution precedence: `--token` flag → `PLAUD_TOKEN` env → config file.

## Usage

```bash
# Download all transcripts and summaries
plaud-hub download

# Download only transcripts from a date range
plaud-hub download --type transcript --from 2024-01-01 --to 2024-03-31

# Download summaries to a custom directory with 10 workers
plaud-hub download --type summary --output-dir ./summaries --concurrency 10

# Force re-download of existing files
plaud-hub download --force

# Verbose/debug output
plaud-hub download -v
```

## Configuration

Config file (`./plaud-hub.yaml` or `~/Library/Application Support/plaud-hub/plaud-hub.yaml`):

```yaml
token: "your-plaud-api-token"
output_dir: "./output"
concurrency: 5
type: "all"
```

Use `--config` to specify a custom config file path.

## Output Format

Files are saved as Markdown with YAML front matter:

```markdown
---
recording_id: abc123
date: 2024-01-15
duration: "12:34"
title: "Weekly Standup"
type: transcript
---

**Speaker 1** [00:01:23]: Hello everyone...

**Speaker 2** [00:01:45]: Good morning...
```

## Project Layout

```
cmd/plaud-hub/main.go          # Entry point
internal/
  api/                         # Plaud API client (endpoints, models, HTTP)
  cmd/                         # Cobra CLI commands (root, download, auth)
  config/                      # Viper configuration management
  download/                    # Download orchestration, file writing, filename utility
```

## License

MIT — see [LICENSE](LICENSE).

## Disclaimer

This tool uses an unofficial, reverse-engineered API based on [arbuzmell/plaud-api](https://github.com/arbuzmell/plaud-api). It is not affiliated with or endorsed by Plaud Inc. The API may change without notice.
