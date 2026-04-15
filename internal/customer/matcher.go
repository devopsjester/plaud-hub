package customer

import "strings"

// Confidence levels for a customer match.
const (
	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"
)

// ConfidenceRank returns a numeric rank for a confidence string so callers can
// compare levels. Higher is more confident.
func ConfidenceRank(c string) int {
	switch c {
	case ConfidenceHigh:
		return 3
	case ConfidenceMedium:
		return 2
	case ConfidenceLow:
		return 1
	}
	return 0
}

// Match represents a single customer identified in a recording.
type Match struct {
	Customer   *Customer
	Confidence string
}

// MatchText searches title and body for customer names/aliases.
//
//   - title match → ConfidenceHigh
//   - body-only match → ConfidenceMedium
//
// Multiple customers may be returned when a recording mentions more than one.
func (r *Registry) MatchText(title, body string) []Match {
	titleLower := strings.ToLower(title)
	bodyLower := strings.ToLower(body)

	// Track best confidence per customer name to avoid duplicates.
	best := make(map[string]string, len(r.Customers))

	for i := range r.Customers {
		c := &r.Customers[i]
		conf := matchCustomer(c, titleLower, bodyLower)
		if conf == "" {
			continue
		}
		if prev, ok := best[c.Name]; !ok || ConfidenceRank(conf) > ConfidenceRank(prev) {
			best[c.Name] = conf
		}
	}

	if len(best) == 0 {
		return nil
	}

	// Return in registry order for determinism.
	matches := make([]Match, 0, len(best))
	for i := range r.Customers {
		c := &r.Customers[i]
		if conf, ok := best[c.Name]; ok {
			matches = append(matches, Match{Customer: c, Confidence: conf})
		}
	}
	return matches
}

// matchCustomer checks all terms for a single customer against title and body.
// Returns the best confidence level, or "" if no match.
func matchCustomer(c *Customer, titleLower, bodyLower string) string {
	for _, term := range c.allTerms() {
		if containsWord(titleLower, term) {
			return ConfidenceHigh
		}
	}
	for _, term := range c.allTerms() {
		if containsWord(bodyLower, term) {
			return ConfidenceMedium
		}
	}
	return ""
}

// containsWord returns true if text contains term as a substring.
// Uses simple substring match since customer names are usually distinctive.
func containsWord(text, term string) bool {
	return strings.Contains(text, term)
}
