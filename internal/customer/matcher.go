package customer

import (
	"strings"
	"unicode"
)

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
	titleLower := normalizeQuotes(strings.ToLower(title))
	bodyLower := normalizeQuotes(strings.ToLower(body))

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

// MatchDomain returns the customer whose Domains list contains the given email
// domain (e.g. "us.mcd.com"). Comparison is case-insensitive.
// Returns nil when no customer claims the domain.
func (r *Registry) MatchDomain(domain string) *Customer {
	domainLower := strings.ToLower(strings.TrimSpace(domain))
	if domainLower == "" {
		return nil
	}
	for i := range r.Customers {
		c := &r.Customers[i]
		for _, d := range c.Domains {
			if strings.ToLower(d) == domainLower {
				return c
			}
		}
	}
	return nil
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

// containsWord returns true if text contains term bounded by non-alphanumeric
// characters (or the start/end of the string). Both text and term should be
// lower-cased by the caller.
func containsWord(text, term string) bool {
	if term == "" {
		return false
	}
	termRunes := []rune(term)
	textRunes := []rune(text)
	termLen := len(termRunes)
	textLen := len(textRunes)

	for i := 0; i <= textLen-termLen; i++ {
		// Quick check: first rune must match.
		if textRunes[i] != termRunes[0] {
			continue
		}
		// Compare the full term.
		if string(textRunes[i:i+termLen]) != term {
			continue
		}
		// Check left boundary.
		if i > 0 && isWordChar(textRunes[i-1]) {
			continue
		}
		// Check right boundary.
		end := i + termLen
		if end < textLen && isWordChar(textRunes[end]) {
			continue
		}
		return true
	}
	return false
}

// isWordChar returns true for runes that are letters or digits.
func isWordChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

// normalizeQuotes replaces Unicode curly/typographic quote characters with
// their plain ASCII equivalents so that alias matching is not defeated by
// AI-generated summaries that use smart punctuation.
//
// Replacements:
//
//	\u2018 LEFT SINGLE QUOTATION MARK  → '
//	\u2019 RIGHT SINGLE QUOTATION MARK → '
//	\u201C LEFT DOUBLE QUOTATION MARK  → "
//	\u201D RIGHT DOUBLE QUOTATION MARK → "
func normalizeQuotes(s string) string {
	s = strings.ReplaceAll(s, "\u2018", "'")
	s = strings.ReplaceAll(s, "\u2019", "'")
	s = strings.ReplaceAll(s, "\u201C", "\"")
	s = strings.ReplaceAll(s, "\u201D", "\"")
	return s
}
