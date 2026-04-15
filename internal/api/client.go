package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"time"
)

// browserHeaders are required by the Plaud API (reverse-engineered from browser traffic).
var browserHeaders = map[string]string{
	"Content-Type":    "application/json",
	"Accept":          "*/*",
	"Accept-Language":  "en-GB,en-US;q=0.9,en;q=0.8",
	"Accept-Encoding":  "gzip, deflate, br",
	"Origin":          "https://web.plaud.ai",
	"Referer":         "https://web.plaud.ai/",
	"User-Agent":      "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.6 Safari/605.1.15",
	"Sec-Fetch-Site":  "same-site",
	"Sec-Fetch-Mode":  "cors",
	"Sec-Fetch-Dest":  "empty",
	"app-platform":    "web",
	"edit-from":       "web",
	"Priority":        "u=3, i",
}

// Client is an HTTP client for the Plaud API with retry and Retry-After support.
type Client struct {
	token      string
	httpClient *http.Client
	maxRetries int
	logger     *slog.Logger
}

// NewClient creates a new Plaud API client.
func NewClient(token string, logger *slog.Logger) *Client {
	return &Client{
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		maxRetries: 3,
		logger:     logger,
	}
}

// doRequest executes an HTTP request with retry logic and Retry-After support.
func (c *Client) doRequest(ctx context.Context, method, url string, body any) ([]byte, error) {
	var bodyReader func() (io.Reader, error)
	if body != nil {
		bodyReader = func() (io.Reader, error) {
			data, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("marshal request body: %w", err)
			}
			return bytes.NewReader(data), nil
		}
	}

	var lastErr error
	for attempt := range c.maxRetries {
		if attempt > 0 {
			c.logger.Debug("retrying request", "attempt", attempt+1, "url", url)
		}

		var reqBody io.Reader
		if bodyReader != nil {
			var err error
			reqBody, err = bodyReader()
			if err != nil {
				return nil, err
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}

		for k, v := range browserHeaders {
			req.Header.Set(k, v)
		}
		req.Header.Set("Authorization", "bearer "+c.token)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("read response: %w", err)
			continue
		}

		switch {
		case resp.StatusCode == 401:
			return nil, fmt.Errorf("authentication failed (401): token may be expired")
		case resp.StatusCode == 404:
			return nil, fmt.Errorf("resource not found (404): %s", url)
		case resp.StatusCode == 429 || (resp.StatusCode >= 500 && resp.StatusCode < 600):
			wait := c.parseRetryAfter(resp, attempt)
			c.logger.Warn("server error, will retry",
				"status", resp.StatusCode,
				"wait", wait,
				"attempt", attempt+1,
				"url", url,
			)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
			snippet := "(empty body)"
			if len(data) > 0 {
				snippet = string(data[:min(len(data), 200)])
			}
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, snippet)
			continue
		case resp.StatusCode >= 400:
			snippet := "(empty body)"
			if len(data) > 0 {
				snippet = string(data[:min(len(data), 300)])
			}
			return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, snippet)
		}

		return data, nil
	}

	return nil, fmt.Errorf("request failed after %d attempts: %w", c.maxRetries, lastErr)
}

// parseRetryAfter extracts the wait duration from Retry-After header,
// falling back to exponential backoff.
func (c *Client) parseRetryAfter(resp *http.Response, attempt int) time.Duration {
	if ra := resp.Header.Get("Retry-After"); ra != "" {
		// Try parsing as seconds.
		if secs, err := strconv.Atoi(ra); err == nil {
			return time.Duration(secs) * time.Second
		}
		// Try parsing as HTTP-date.
		if t, err := http.ParseTime(ra); err == nil {
			wait := time.Until(t)
			if wait > 0 {
				return wait
			}
		}
	}
	// Exponential backoff: 1s, 2s, 4s, ...
	base := time.Duration(1<<uint(attempt)) * time.Second
	jitter := time.Duration(rand.Int64N(int64(time.Second)))
	return base + jitter
}

// get performs a GET request. A random cache-bust parameter is added.
func (c *Client) get(ctx context.Context, url string, params map[string]string) ([]byte, error) {
	if params == nil {
		params = make(map[string]string)
	}
	params["r"] = strconv.FormatFloat(rand.Float64(), 'f', -1, 64)

	req := url
	if len(params) > 0 {
		req += "?"
		first := true
		for k, v := range params {
			if !first {
				req += "&"
			}
			req += k + "=" + v
			first = false
		}
	}
	return c.doRequest(ctx, http.MethodGet, req, nil)
}

// post performs a POST request with a JSON body.
func (c *Client) post(ctx context.Context, url string, body any) ([]byte, error) {
	return c.doRequest(ctx, http.MethodPost, url, body)
}
