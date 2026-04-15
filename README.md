# plaud-downloader

A CLI tool to download Plaud Note transcripts and/or AI summaries as Markdown files with YAML front matter.

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
go install github.com/devopsjester/plaud-downloader/cmd/plaud-downloader@latest
```

Or build from source:

```bash
git clone https://github.com/devopsjester/plaud-downloader.git
cd plaud-downloader
go build -o plaud-downloader ./cmd/plaud-downloader
```

## Authentication

Get your Plaud API token:

1. Open [web.plaud.ai](https://web.plaud.ai) and sign in
2. Open DevTools → Network tab → find any request to `api.plaud.ai`
3. Copy the `Authorization` header value (without the "bearer " prefix)

Then set it up (choose one):

```bash
# Interactive setup (saves to ~/.config/plaud-downloader/config.yaml)
plaud-downloader auth setup

# Environment variable
export PLAUD_TOKEN='your-token-here'

# CLI flag
plaud-downloader download --token 'your-token-here'
```

Token resolution precedence: `--token` flag → `PLAUD_TOKEN` env → config file.

## Usage

```bash
# Download all transcripts and summaries
plaud-downloader download

# Download only transcripts from a date range
plaud-downloader download --type transcript --from 2024-01-01 --to 2024-03-31

# Download summaries to a custom directory with 10 workers
plaud-downloader download --type summary --output-dir ./summaries --concurrency 10

# Force re-download of existing files
plaud-downloader download --force

# Verbose/debug output
plaud-downloader download -v
```

## Configuration

Config file (`./plaud-downloader.yaml` or `~/.config/plaud-downloader/config.yaml`):

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
cmd/plaud-downloader/main.go   # Entry point
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
