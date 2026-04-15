package customer

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CorrelateFile reads a Markdown file (summary or transcript) and returns any
// customer matches found in its YAML front matter title and body content.
func CorrelateFile(path string, registry *Registry) ([]Match, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	title, body, err := parseFrontMatter(f)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	return registry.MatchText(title, body), nil
}

// CustomerOutputDir returns the directory where files for a customer should be
// written: {outputDir}/customers/{customerName}.
func CustomerOutputDir(outputDir, customerName string) string {
	return filepath.Join(outputDir, "customers", customerName)
}

// parseFrontMatter reads a Markdown file and splits it into the YAML front
// matter title value and the remaining body text.
func parseFrontMatter(r io.Reader) (title, body string, err error) {
	scanner := bufio.NewScanner(r)

	// Check for opening "---".
	if !scanner.Scan() {
		return "", "", nil
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
		return "", sb.String(), scanner.Err()
	}

	// Read lines until closing "---".
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			break
		}
		if strings.HasPrefix(line, "title:") {
			val := strings.TrimPrefix(line, "title:")
			val = strings.TrimSpace(val)
			// Strip surrounding quotes added by the writer.
			val = strings.Trim(val, `"'`)
			title = val
		}
	}

	// Remaining content is the body.
	var sb strings.Builder
	for scanner.Scan() {
		sb.WriteString(scanner.Text())
		sb.WriteByte('\n')
	}
	return title, sb.String(), scanner.Err()
}
