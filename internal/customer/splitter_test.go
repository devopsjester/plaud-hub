package customer

import (
	"context"
	"errors"
	"testing"
)

// mockSplitter is a test double for LLMSplitter that records whether it was called.
type mockSplitter struct {
	called bool
	result map[string]string
	err    error
}

func (m *mockSplitter) SplitSummary(_ context.Context, _ string, _ []string) (map[string]string, error) {
	m.called = true
	return m.result, m.err
}

// makeMatches builds a []Match slice from customer names, using ConfidenceMedium.
func makeMatches(names ...string) []Match {
	out := make([]Match, len(names))
	for i, name := range names {
		out[i] = Match{Customer: &Customer{Name: name}, Confidence: ConfidenceMedium}
	}
	return out
}

func TestSplitByLLM(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		matches        []Match
		splitterResult map[string]string
		splitterErr    error
		wantLen        int
		wantNames      []string
		wantErr        bool
		wantCalled     bool
	}{
		{
			name:    "happy path two customers",
			matches: makeMatches("Acme", "Globex"),
			splitterResult: map[string]string{
				"Acme":   "## Acme content",
				"Globex": "## Globex content",
			},
			wantLen:    2,
			wantNames:  []string{"Acme", "Globex"},
			wantCalled: true,
		},
		{
			name:    "LLM returns content for only one of two customers",
			matches: makeMatches("Acme", "Globex"),
			splitterResult: map[string]string{
				"Acme": "## Acme content",
			},
			wantLen:    1,
			wantNames:  []string{"Acme"},
			wantCalled: true,
		},
		{
			name:    "other key in LLM response is ignored",
			matches: makeMatches("Acme"),
			splitterResult: map[string]string{
				"Acme":  "## Acme content",
				"other": "## leftover stuff",
			},
			wantLen:    1,
			wantNames:  []string{"Acme"},
			wantCalled: true,
		},
		{
			name:    "empty string content for a customer is omitted",
			matches: makeMatches("Acme", "Globex"),
			splitterResult: map[string]string{
				"Acme":   "",
				"Globex": "## Globex content",
			},
			wantLen:    1,
			wantNames:  []string{"Globex"},
			wantCalled: true,
		},
		{
			name:        "LLM error is propagated",
			matches:     makeMatches("Acme"),
			splitterErr: errors.New("LLM unavailable"),
			wantErr:     true,
			wantCalled:  true,
		},
		{
			name:       "empty matches returns nil without calling LLM",
			matches:    []Match{},
			wantLen:    0,
			wantCalled: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			mock := &mockSplitter{result: tt.splitterResult, err: tt.splitterErr}
			results, err := SplitByLLM(context.Background(), mock, "meeting body text", tt.matches)

			if (err != nil) != tt.wantErr {
				t.Fatalf("SplitByLLM() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}

			if mock.called != tt.wantCalled {
				t.Errorf("splitter called = %v, want %v", mock.called, tt.wantCalled)
			}

			if len(results) != tt.wantLen {
				t.Fatalf("got %d results, want %d", len(results), tt.wantLen)
			}

			for i, name := range tt.wantNames {
				if results[i].CustomerName != name {
					t.Errorf("results[%d].CustomerName = %q, want %q", i, results[i].CustomerName, name)
				}
				if results[i].Body == "" {
					t.Errorf("results[%d].Body is empty for customer %q", i, name)
				}
			}

			// Verify "other" key never surfaces as a SplitResult.
			for _, r := range results {
				if r.CustomerName == "other" {
					t.Errorf("SplitResult with CustomerName \"other\" must not be returned")
				}
			}
		})
	}
}
