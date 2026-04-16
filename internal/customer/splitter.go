package customer

import "context"

// SplitResult holds LLM-extracted content per customer.
type SplitResult struct {
	CustomerName string
	Body         string // extracted markdown body for this customer
}

// LLMSplitter is the interface the correlate command calls.
type LLMSplitter interface {
	SplitSummary(ctx context.Context, body string, customers []string) (map[string]string, error)
}

// SplitByLLM calls the splitter and returns one SplitResult per customer.
// If a customer has no content in the response, it is omitted.
// "other" key content is ignored (not placed anywhere).
func SplitByLLM(ctx context.Context, splitter LLMSplitter, body string, matches []Match) ([]SplitResult, error) {
	if len(matches) == 0 {
		return nil, nil
	}
	names := make([]string, len(matches))
	for i, m := range matches {
		names[i] = m.Customer.Name
	}

	parts, err := splitter.SplitSummary(ctx, body, names)
	if err != nil {
		return nil, err
	}

	var results []SplitResult
	for _, m := range matches {
		content, ok := parts[m.Customer.Name]
		if !ok || content == "" {
			continue
		}
		results = append(results, SplitResult{
			CustomerName: m.Customer.Name,
			Body:         content,
		})
	}
	return results, nil
}
