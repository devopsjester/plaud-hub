// Package llm provides clients for LLM APIs used by plaud-hub.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// maxResponseBytes caps the LLM response body to 2 MiB to prevent unbounded
// memory consumption from a malformed or rogue API response.
const maxResponseBytes = 2 << 20 // 2 MiB

const githubModelsURL = "https://models.github.com/v1/chat/completions"

// GitHubClient is a client for the GitHub Models API (OpenAI-compatible).
type GitHubClient struct {
	token      string
	model      string
	baseURL    string
	httpClient *http.Client
}

// NewGitHubClient constructs a client for GitHub Models.
// model defaults to "gpt-4o-mini" if empty.
func NewGitHubClient(token, model string) *GitHubClient {
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &GitHubClient{
		token:      token,
		model:      model,
		baseURL:    githubModelsURL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatChoice struct {
	Message chatMessage `json:"message"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
}

// SplitSummary sends the summary body to the LLM and asks it to extract
// per-customer content. Returns a map of customerName -> extracted markdown.
// Falls back gracefully: if the response can't be parsed, returns an error
// so the caller can fall back to full-copy behavior.
func (c *GitHubClient) SplitSummary(ctx context.Context, body string, customers []string) (map[string]string, error) {
	customerList, err := json.Marshal(customers)
	if err != nil {
		return nil, fmt.Errorf("marshal customer list: %w", err)
	}

	prompt := fmt.Sprintf(`You are a meeting notes splitter. Given a meeting summary that covers multiple customers, extract ONLY the content relevant to each named customer.

Customers: %s

Rules:
- Do NOT invent or add any content — only use what is in the summary below.
- Preserve all markdown formatting in extracted content.
- Return valid JSON only, with no extra text: {"CustomerName": "extracted markdown content", ...}
- Include an "other" key for content not tied to any specific customer.
- If a customer has no relevant content, omit them from the JSON.

Summary:
%s`, string(customerList), body)

	reqBody := chatRequest{
		Model: c.model,
		Messages: []chatMessage{
			{Role: "user", Content: prompt},
		},
	}

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub Models API returned status %d", resp.StatusCode)
	}

	var chatResp chatResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&chatResp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	raw := strings.TrimSpace(chatResp.Choices[0].Message.Content)

	// Models often wrap JSON in markdown code fences despite being told not to.
	// Strip ```json ... ``` or ``` ... ``` wrappers before unmarshaling.
	if strings.HasPrefix(raw, "```") {
		if idx := strings.IndexByte(raw, '\n'); idx != -1 {
			raw = raw[idx+1:]
		}
		if idx := strings.LastIndex(raw, "```"); idx != -1 {
			raw = strings.TrimSpace(raw[:idx])
		}
	}

	var result map[string]string
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return nil, fmt.Errorf("parse LLM content as JSON: %w", err)
	}

	return result, nil
}
