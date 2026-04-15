// Package download provides utilities for downloading Plaud transcripts and summaries.
package download

import (
	"crypto/sha256"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

const (
	// MaxFilenameLength is the maximum length for a sanitized filename (without extension).
	MaxFilenameLength = 100

	// hashSuffixLen is the length of the short hash appended when truncating.
	hashSuffixLen = 8
)

// unsafeChars matches characters that are not safe in filenames.
var unsafeChars = regexp.MustCompile(`[^\w\-. ]+`)

// multiSpace collapses multiple spaces/underscores.
var multiSpace = regexp.MustCompile(`[\s_]+`)

// SanitizeFilename creates a filesystem-safe filename from a title string.
// It replaces unsafe characters, collapses whitespace, and truncates long
// names by appending a short hash to preserve uniqueness.
func SanitizeFilename(title string) string {
	if title == "" {
		return "untitled"
	}

	// Trim whitespace.
	name := strings.TrimSpace(title)

	// Replace unsafe characters with underscores.
	name = unsafeChars.ReplaceAllString(name, "_")

	// Collapse multiple spaces/underscores into a single underscore.
	name = multiSpace.ReplaceAllString(name, "_")

	// Trim leading/trailing underscores and dots.
	name = strings.Trim(name, "_.")

	// Remove non-printable characters.
	name = strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return -1
	}, name)

	if name == "" {
		return "untitled"
	}

	// Truncate if too long, appending a short hash.
	if len(name) > MaxFilenameLength {
		hash := shortHash(title)
		// Leave room for "_" + hash.
		maxBase := MaxFilenameLength - 1 - hashSuffixLen
		if maxBase < 1 {
			maxBase = 1
		}
		name = name[:maxBase] + "_" + hash
	}

	return name
}

// shortHash returns the first hashSuffixLen hex characters of the SHA-256
// of the input string. This preserves uniqueness when titles are truncated.
func shortHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h[:hashSuffixLen/2])
}
