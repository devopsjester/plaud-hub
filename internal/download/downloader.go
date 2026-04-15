package download

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"

	"github.com/devopsjester/plaud-hub/internal/api"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
)

// Options configures the download behavior.
type Options struct {
	OutputDir   string
	Type        string // "transcript", "summary", or "all"
	Force       bool   // Re-download even if file exists.
	Concurrency int
}

// Result summarizes what happened during a download run.
type Result struct {
	Downloaded int
	Skipped    int
	Failed     int
}

// Run downloads transcripts and/or summaries for the given recordings
// using bounded concurrency and mpb progress bars.
func Run(ctx context.Context, client *api.Client, recordings []api.Recording, opts Options, logger *slog.Logger) (*Result, error) {
	if len(recordings) == 0 {
		logger.Info("no recordings to process")
		return &Result{}, nil
	}

	// Determine what to download.
	wantTranscript := opts.Type == "all" || opts.Type == "transcript"
	wantSummary := opts.Type == "all" || opts.Type == "summary"

	// Build work items: each recording may need transcript and/or summary fetched.
	type workItem struct {
		rec api.Recording
	}
	items := make([]workItem, 0, len(recordings))
	for _, rec := range recordings {
		items = append(items, workItem{rec: rec})
	}

	// Set up mpb progress container.
	progress := mpb.NewWithContext(ctx,
		mpb.WithWidth(60),
	)

	bar := progress.AddBar(int64(len(items)),
		mpb.PrependDecorators(
			decor.Name("Downloading: "),
			decor.CountersNoUnit("%d / %d"),
		),
		mpb.AppendDecorators(
			decor.Percentage(),
		),
	)

	// Bounded concurrency via semaphore channel.
	sem := make(chan struct{}, opts.Concurrency)
	var wg sync.WaitGroup

	var downloaded, skipped, failed atomic.Int64

	for i := range items {
		item := items[i]
		wg.Add(1)

		go func() {
			defer wg.Done()
			defer bar.Increment()

			// Acquire semaphore.
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			// Fetch full details for this recording.
			details, err := client.GetRecordingDetails(ctx, []string{item.rec.ID})
			if err != nil {
				failed.Add(1)
				logger.Error("failed to fetch recording details",
					"id", item.rec.ID,
					"filename", item.rec.Filename,
					"error", err,
				)
				return
			}
			if len(details) == 0 {
				failed.Add(1)
				logger.Error("recording not found in details response",
					"id", item.rec.ID,
					"filename", item.rec.Filename,
				)
				return
			}

			rec := details[0]

			// Process transcript.
			if wantTranscript {
				outPath := OutputPath(opts.OutputDir, rec, "transcript")
				if !opts.Force && fileExists(outPath) {
					logger.Debug("skipping existing transcript",
						"path", outPath,
						"id", rec.ID,
					)
					skipped.Add(1)
				} else {
					segments, err := ParseTranscriptSegments(rec.TransResult)
					if err != nil {
						failed.Add(1)
						logger.Error("failed to parse transcript",
							"id", rec.ID,
							"filename", rec.Filename,
							"error", err,
						)
					} else if len(segments) > 0 {
						if _, err := WriteTranscript(opts.OutputDir, rec, segments); err != nil {
							failed.Add(1)
							logger.Error("failed to write transcript",
								"id", rec.ID,
								"filename", rec.Filename,
								"error", err,
							)
						} else {
							downloaded.Add(1)
						}
					} else {
						logger.Debug("no transcript segments available",
							"id", rec.ID,
							"filename", rec.Filename,
						)
					}
				}
			}

			// Process summary.
			if wantSummary {
				outPath := OutputPath(opts.OutputDir, rec, "summary")
				if !opts.Force && fileExists(outPath) {
					logger.Debug("skipping existing summary",
						"path", outPath,
						"id", rec.ID,
					)
					skipped.Add(1)
				} else {
					content := ParseSummaryContent(rec.AIContent, rec.SummaryList)
					if content != "" {
						if _, err := WriteSummary(opts.OutputDir, rec, content); err != nil {
							failed.Add(1)
							logger.Error("failed to write summary",
								"id", rec.ID,
								"filename", rec.Filename,
								"error", err,
							)
						} else {
							downloaded.Add(1)
						}
					} else {
						logger.Debug("no summary content available",
							"id", rec.ID,
							"filename", rec.Filename,
						)
					}
				}
			}
		}()
	}

	wg.Wait()
	progress.Wait()

	return &Result{
		Downloaded: int(downloaded.Load()),
		Skipped:    int(skipped.Load()),
		Failed:     int(failed.Load()),
	}, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// FormatResult returns a human-readable summary of the download result.
func (r *Result) FormatResult() string {
	return fmt.Sprintf("Done: %d downloaded, %d skipped, %d failed", r.Downloaded, r.Skipped, r.Failed)
}
