package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// chatResponseJSON builds the JSON bytes that SplitSummary expects from the API.
// content is placed as the message content in the first choice.
func chatResponseJSON(content string) []byte {
	resp := chatResponse{
		Choices: []chatChoice{
			{Message: chatMessage{Role: "assistant", Content: content}},
		},
	}
	b, _ := json.Marshal(resp)
	return b
}

// jsonMap encodes a map[string]string as a JSON string (used as the LLM's content).
func jsonMap(m map[string]string) string {
	b, _ := json.Marshal(m)
	return string(b)
}

// newTestClient creates a GitHubClient pointing at the given httptest.Server URL.
func newTestClient(t *testing.T, srv *httptest.Server, token, model string) *GitHubClient {
	t.Helper()
	c := NewGitHubClient(token, model)
	c.baseURL = srv.URL
	return c
}

func TestNewGitHubClient_DefaultModel(t *testing.T) {
	t.Parallel()
	c := NewGitHubClient("tok", "")
	if c.model != "gpt-4o-mini" {
		t.Errorf("default model = %q, want %q", c.model, "gpt-4o-mini")
	}
}

func TestNewGitHubClient_ExplicitModel(t *testing.T) {
	t.Parallel()
	c := NewGitHubClient("tok", "gpt-4o")
	if c.model != "gpt-4o" {
		t.Errorf("model = %q, want %q", c.model, "gpt-4o")
	}
}

func TestSplitSummary_AuthorizationHeader(t *testing.T) {
	t.Parallel()

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(chatResponseJSON(jsonMap(map[string]string{"Acme": "content"})))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "secret-token", "")
	_, err := c.SplitSummary(context.Background(), "body", []string{"Acme"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotAuth != "Bearer secret-token" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer secret-token")
	}
}

func TestSplitSummary_ContentTypeHeader(t *testing.T) {
	t.Parallel()

	var gotContentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(chatResponseJSON(jsonMap(map[string]string{"Acme": "content"})))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "tok", "")
	_, err := c.SplitSummary(context.Background(), "body", []string{"Acme"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", gotContentType, "application/json")
	}
}

func TestSplitSummary_ParsesValidResponse(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"Acme":   "## Acme notes",
		"Globex": "## Globex notes",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(chatResponseJSON(jsonMap(want)))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "tok", "")
	got, err := c.SplitSummary(context.Background(), "body", []string{"Acme", "Globex"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d", len(got), len(want))
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("got[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestSplitSummary_Non200Error(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "tok", "")
	_, err := c.SplitSummary(context.Background(), "body", []string{"Acme"})
	if err == nil {
		t.Fatal("expected error for non-200 status, got nil")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("error = %q, want to mention status 429", err.Error())
	}
}

func TestSplitSummary_InvalidJSONContent(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// LLM returns prose instead of a JSON map.
		_, _ = w.Write(chatResponseJSON("Here is a summary for Acme: they discussed pricing."))
	}))
	defer srv.Close()

	c := newTestClient(t, srv, "tok", "")
	_, err := c.SplitSummary(context.Background(), "body", []string{"Acme"})
	if err == nil {
		t.Fatal("expected error for non-JSON LLM content, got nil")
	}
	if !strings.Contains(err.Error(), "parse LLM content as JSON") {
		t.Errorf("error = %q, want to contain \"parse LLM content as JSON\"", err.Error())
	}
}
