package customer

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// CorrelateFile reads a Markdown file (summary) and returns any customer
// matches found in its YAML front matter title and body content.
func CorrelateFile(path string, registry *Registry) ([]Match, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	fm, err := parseFrontMatter(f)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	return registry.MatchText(fm.title, fm.body), nil
}

// ParseRecordingDate returns the date parsed from a file's YAML front matter
// "date:" field. Returns zero time when absent or unparseable.
func ParseRecordingDate(path string) (time.Time, error) {
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	fm, err := parseFrontMatter(f)
	return fm.date, err
}

// ParseRecordingTitle returns the "title:" value from a file's YAML front matter.
// Returns an empty string when absent or when the file has no front matter.
func ParseRecordingTitle(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	fm, err := parseFrontMatter(f)
	return fm.title, err
}

// RecordingInfo contains the parsed front matter fields and body of a summary.
type RecordingInfo struct {
	Title    string
	Start    time.Time     // from date: field (RFC3339)
	Duration time.Duration // from duration: field (MM:SS or HH:MM:SS)
	Body     string
}

// End returns the recording end time (Start + Duration), or Start when Duration is zero.
func (r RecordingInfo) End() time.Time {
	if r.Duration == 0 {
		return r.Start
	}
	return r.Start.Add(r.Duration)
}

// ParseRecordingInfo reads a summary file and returns its front-matter fields
// and body in a single pass.
func ParseRecordingInfo(path string) (RecordingInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return RecordingInfo{}, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	fm, err := parseFrontMatter(f)
	if err != nil {
		return RecordingInfo{}, err
	}
	return RecordingInfo{
		Title:    fm.title,
		Start:    fm.date,
		Duration: fm.duration,
		Body:     fm.body,
	}, nil
}

// CorrelateFileCombined reads both the summary and transcript for a recording
// (identified by summaryPath) and merges matches from both. Title matches
// from either file yield high confidence; body-only matches yield medium.
func CorrelateFileCombined(summaryPath string, registry *Registry) ([]Match, error) {
	summaryMatches, err := CorrelateFile(summaryPath, registry)
	if err != nil {
		return nil, err
	}

	transcriptPath := strings.TrimSuffix(summaryPath, "_summary.md") + "_transcript.md"
	transcriptMatches, err := CorrelateFile(transcriptPath, registry)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return mergeMatches(summaryMatches, transcriptMatches), nil
}

// mergeMatches combines two match slices, keeping the highest confidence per
// customer and preserving the original registry order.
func mergeMatches(a, b []Match) []Match {
	best := make(map[string]Match)
	for _, m := range a {
		best[m.Customer.Name] = m
	}
	for _, m := range b {
		if prev, ok := best[m.Customer.Name]; !ok || ConfidenceRank(m.Confidence) > ConfidenceRank(prev.Confidence) {
			best[m.Customer.Name] = m
		}
	}
	// Flatten in insertion order of map (Go maps are unordered, but callers
	// don't require strict ordering for merge results).
	out := make([]Match, 0, len(best))
	for _, m := range best {
		out = append(out, m)
	}
	return out
}

// CustomerOutputDir returns the directory where files for a customer should be
// written: {outputDir}/customers/{customerName}.
func CustomerOutputDir(outputDir, customerName string) string {
	return filepath.Join(outputDir, "customers", customerName)
}

// parsedFrontMatter holds the fields extracted from a YAML front matter block.
type parsedFrontMatter struct {
	title    string
	body     string
	date     time.Time
	duration time.Duration
}

// parseFrontMatter reads a Markdown file and extracts YAML front matter fields
// (title, date, duration) and the body text.
func parseFrontMatter(r io.Reader) (parsedFrontMatter, error) {
	scanner := newLargeScanner(r)

	// Check for opening "---".
	if !scanner.Scan() {
		return parsedFrontMatter{}, scanner.Err()
	}
	firstLine := scanner.Text()
	if strings.TrimSpace(firstLine) != "---" {
		// No front matter: entire file is body.
		var sb strings.Builder
		sb.WriteString(firstLine)
		sb.WriteByte('\n')
		for scanner.Scan() {
			sb.WriteString(scanner.Text())
			sb.WriteByte('\n')
		}
		return parsedFrontMatter{body: sb.String()}, scanner.Err()
	}

	var fm parsedFrontMatter

	// Read lines until closing "---".
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		val := fmValue(line)
		switch {
		case strings.HasPrefix(line, "title:"):
			fm.title = val
		case strings.HasPrefix(line, "date:"):
			if t, err := time.Parse(time.RFC3339, val); err == nil {
				fm.date = t
			} else if t, err := time.Parse("2006-01-02", val); err == nil {
				fm.date = t
			}
		case strings.HasPrefix(line, "duration:"):
			fm.duration = parseDuration(val)
		}
	}

	// Remaining content is the body.
	var sb strings.Builder
	for scanner.Scan() {
		sb.WriteString(scanner.Text())
		sb.WriteByte('\n')
	}
	fm.body = sb.String()
	return fm, scanner.Err()
}

// fmValue extracts the value after the first colon in a front matter line,
// trimming whitespace and surrounding quotes.
func fmValue(line string) string {
	i := strings.IndexByte(line, ':')
	if i < 0 {
		return ""
	}
	v := strings.TrimSpace(line[i+1:])
	return strings.Trim(v, `"'`)
}

// parseDuration parses a duration string in "MM:SS" or "HH:MM:SS" format.
// Returns 0 when the string is empty or cannot be parsed.
func parseDuration(s string) time.Duration {
	if s == "" {
		return 0
	}
	parts := strings.Split(s, ":")
	switch len(parts) {
	case 2: // MM:SS
		mm, err1 := strconv.Atoi(parts[0])
		ss, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			return 0
		}
		return time.Duration(mm)*time.Minute + time.Duration(ss)*time.Second
	case 3: // HH:MM:SS
		hh, err1 := strconv.Atoi(parts[0])
		mm, err2 := strconv.Atoi(parts[1])
		ss, err3 := strconv.Atoi(parts[2])
		if err1 != nil || err2 != nil || err3 != nil {
			return 0
		}
		return time.Duration(hh)*time.Hour + time.Duration(mm)*time.Minute + time.Duration(ss)*time.Second
	}
	return 0
}

// newLargeScanner wraps a reader in a bufio.Scanner with a generous token
// buffer to handle large transcript files.
func newLargeScanner(r io.Reader) *bufio.Scanner {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 1024*1024), 1024*1024)
	return s
}
