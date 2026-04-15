package download

import (
	"strings"
	"testing"
)

func TestSanitizeFilename_Basic(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple title",
			input: "Weekly Standup",
			want:  "Weekly_Standup",
		},
		{
			name:  "special characters removed",
			input: "Meeting: Q1 Review / Budget (final)",
			want:  "Meeting_Q1_Review_Budget_final",
		},
		{
			name:  "empty string",
			input: "",
			want:  "untitled",
		},
		{
			name:  "only special chars",
			input: "///???***",
			want:  "untitled",
		},
		{
			name:  "preserves hyphens and dots",
			input: "2024-01-15 meeting.notes",
			want:  "2024-01-15_meeting.notes",
		},
		{
			name:  "collapses whitespace",
			input: "too   many     spaces",
			want:  "too_many_spaces",
		},
		{
			name:  "trims leading/trailing underscores",
			input: "  _hello world_  ",
			want:  "hello_world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeFilename(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeFilename(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeFilename_Truncation(t *testing.T) {
	// Create a title longer than MaxFilenameLength.
	longTitle := strings.Repeat("a", MaxFilenameLength+50)

	got := SanitizeFilename(longTitle)

	if len(got) > MaxFilenameLength {
		t.Errorf("expected length <= %d, got %d (%q)", MaxFilenameLength, len(got), got)
	}

	// Should end with a hash suffix.
	if !strings.Contains(got, "_") {
		t.Errorf("expected truncated name to contain '_' separator, got %q", got)
	}

	// Two different long titles should produce different filenames.
	longTitle2 := strings.Repeat("b", MaxFilenameLength+50)
	got2 := SanitizeFilename(longTitle2)
	if got == got2 {
		t.Errorf("different long titles should produce different filenames: %q == %q", got, got2)
	}
}

func TestShortHash_Deterministic(t *testing.T) {
	h1 := shortHash("hello")
	h2 := shortHash("hello")
	if h1 != h2 {
		t.Errorf("shortHash should be deterministic: %q != %q", h1, h2)
	}
	if len(h1) != hashSuffixLen {
		t.Errorf("expected hash length %d, got %d", hashSuffixLen, len(h1))
	}
}
