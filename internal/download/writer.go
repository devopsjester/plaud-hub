package download

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/devopsjester/plaud-downloader/internal/api"
)

// WriteTranscript writes a transcript markdown file with YAML front matter.
func WriteTranscript(outputDir string, rec api.Recording, segments []api.TranscriptSegment) (string, error) {
	date := rec.CreatedAt().Format("2006-01-02")
	base := SanitizeFilename(rec.Filename)
	filename := fmt.Sprintf("%s_%s_transcript.md", date, base)
	path := filepath.Join(outputDir, filename)

	var sb strings.Builder

	// YAML front matter.
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("recording_id: %s\n", rec.ID))
	sb.WriteString(fmt.Sprintf("date: %s\n", date))
	sb.WriteString(fmt.Sprintf("duration: %q\n", rec.DurationDisplay()))
	sb.WriteString(fmt.Sprintf("title: %q\n", rec.Filename))
	sb.WriteString("type: transcript\n")
	sb.WriteString("---\n\n")

	// Transcript body.
	for _, seg := range segments {
		ts := seg.FormatTimestamp()
		speaker := seg.Speaker
		if speaker == "" {
			speaker = "Unknown"
		}
		sb.WriteString(fmt.Sprintf("**%s** [%s]: %s\n\n", speaker, ts, seg.Content))
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		return "", fmt.Errorf("write transcript: %w", err)
	}

	return path, nil
}

// WriteSummary writes a summary markdown file with YAML front matter.
func WriteSummary(outputDir string, rec api.Recording, content string) (string, error) {
	date := rec.CreatedAt().Format("2006-01-02")
	base := SanitizeFilename(rec.Filename)
	filename := fmt.Sprintf("%s_%s_summary.md", date, base)
	path := filepath.Join(outputDir, filename)

	var sb strings.Builder

	// YAML front matter.
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("recording_id: %s\n", rec.ID))
	sb.WriteString(fmt.Sprintf("date: %s\n", date))
	sb.WriteString(fmt.Sprintf("duration: %q\n", rec.DurationDisplay()))
	sb.WriteString(fmt.Sprintf("title: %q\n", rec.Filename))
	sb.WriteString("type: summary\n")
	sb.WriteString("---\n\n")

	sb.WriteString(content)
	sb.WriteString("\n")

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create output directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		return "", fmt.Errorf("write summary: %w", err)
	}

	return path, nil
}

// ParseTranscriptSegments parses the raw trans_result JSON into segments.
func ParseTranscriptSegments(raw json.RawMessage) ([]api.TranscriptSegment, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}

	var segments []api.TranscriptSegment
	if err := json.Unmarshal(raw, &segments); err != nil {
		return nil, fmt.Errorf("parse transcript segments: %w", err)
	}
	return segments, nil
}

// ParseSummaryContent extracts markdown text from Plaud's ai_content field.
// The field can be plain markdown or a JSON string with varying schemas.
func ParseSummaryContent(raw string, summaryList []string) string {
	if raw == "" && len(summaryList) > 0 {
		raw = summaryList[0]
	}
	if raw == "" {
		return ""
	}

	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "{") {
		return raw
	}

	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return raw
	}

	if md, ok := parsed["markdown"].(string); ok {
		return md
	}
	if content, ok := parsed["content"].(map[string]any); ok {
		if md, ok := content["markdown"].(string); ok {
			return md
		}
	}
	if summary, ok := parsed["summary"].(string); ok {
		return summary
	}

	return raw
}

// OutputPath returns the expected file path for a given recording and type.
// Used to check whether a file already exists (for skip-existing logic).
func OutputPath(outputDir string, rec api.Recording, fileType string) string {
	date := rec.CreatedAt().Format("2006-01-02")
	base := SanitizeFilename(rec.Filename)

	// Use zero time check for recordings with no created date.
	if rec.CreatedAt() == (time.Time{}) {
		date = "0000-00-00"
	}

	return filepath.Join(outputDir, fmt.Sprintf("%s_%s_%s.md", date, base, fileType))
}
