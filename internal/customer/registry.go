// Package customer handles customer identification and correlation for Plaud recordings.
package customer

import (
	"fmt"
	"os"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Customer represents a known customer with canonical name and optional aliases.
type Customer struct {
	// Name is the canonical folder-safe customer name (e.g. "McDonalds").
	Name string `yaml:"name"`
	// Aliases are additional terms to match (e.g. "McDonald's", "mcd").
	Aliases []string `yaml:"aliases,omitempty"`
	// Domains are email domains associated with this customer (e.g. "us.mcd.com").
	Domains []string `yaml:"domains,omitempty"`
}

// allTerms returns a lowercase, quote-normalized slice of the canonical name
// plus all aliases — ready for direct comparison against normalized text.
func (c Customer) allTerms() []string {
	terms := make([]string, 0, 1+len(c.Aliases))
	terms = append(terms, normalizeQuotes(strings.ToLower(c.Name)))
	for _, a := range c.Aliases {
		if a != "" {
			terms = append(terms, normalizeQuotes(strings.ToLower(a)))
		}
	}
	return terms
}

// Registry holds the list of known customers loaded from a YAML file.
type Registry struct {
	Customers []Customer `yaml:"customers"`
}

// LoadRegistry reads a customer registry from a YAML file at the given path.
// The file must contain a top-level "customers" list.
func LoadRegistry(path string) (*Registry, error) {
	// Validate the path to prevent path traversal.
	if strings.Contains(path, "..") {
		return nil, fmt.Errorf("invalid customers file path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read customers file %q: %w", path, err)
	}
	var r Registry
	if err := yaml.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("parse customers file %q: %w", path, err)
	}
	return &r, nil
}
