// Package auth implements OAuth 2.0 device-code flows for calendar providers.
// Client IDs and secrets are passed in by callers — nothing is hardcoded here.
// Tokens are returned to callers and never logged by this package.
package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// m365 device-code OAuth endpoints (common tenant — works for personal and
// work/school accounts alike).
const (
	m365DeviceCodeURL = "https://login.microsoftonline.com/common/oauth2/v2.0/devicecode"
	m365TokenURL      = "https://login.microsoftonline.com/common/oauth2/v2.0/token"

	// m365Scopes are the minimum scopes needed to read calendar events.
	// "offline_access" requests a refresh token.
	m365Scopes = "Calendars.Read offline_access"
)

type m365DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
	Message         string `json:"message"`
}

type m365TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

// AuthorizeM365 performs the OAuth 2.0 device-code flow for Microsoft 365.
// It prints the user_code and verification_uri to stdout so the user can
// authenticate in any browser. It polls until the authorization completes or
// the context is cancelled.
//
// clientID must be a registered Azure AD application client ID.
// Token storage is the caller's responsibility.
func AuthorizeM365(ctx context.Context, clientID string) (accessToken, refreshToken string, err error) {
	httpClient := &http.Client{Timeout: 15 * time.Second}

	// Step 1: Request device and user codes.
	dcResp, interval, err := requestM365DeviceCode(ctx, httpClient, clientID)
	if err != nil {
		return "", "", err
	}

	// Print instructions for the user — use the message from the server when
	// available, otherwise construct one ourselves.
	if dcResp.Message != "" {
		fmt.Println(dcResp.Message)
	} else {
		fmt.Printf("Visit %s and enter code: %s\n", dcResp.VerificationURI, dcResp.UserCode)
	}

	// Step 2: Poll for the token.
	deadline := time.Now().Add(time.Duration(dcResp.ExpiresIn) * time.Second)
	pollInterval := time.Duration(interval) * time.Second

	for {
		if time.Now().After(deadline) {
			return "", "", fmt.Errorf("authorization timed out")
		}

		select {
		case <-ctx.Done():
			return "", "", ctx.Err()
		case <-time.After(pollInterval):
		}

		tok, pending, err := pollM365Token(ctx, httpClient, clientID, dcResp.DeviceCode)
		if err != nil {
			return "", "", err
		}
		if pending {
			continue
		}
		return tok.AccessToken, tok.RefreshToken, nil
	}
}

func requestM365DeviceCode(ctx context.Context, client *http.Client, clientID string) (*m365DeviceCodeResponse, int, error) {
	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("scope", m365Scopes)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m365DeviceCodeURL,
		strings.NewReader(data.Encode()))
	if err != nil {
		return nil, 0, fmt.Errorf("build device-code request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("device-code request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("device-code endpoint: unexpected status %d", resp.StatusCode)
	}

	var dc m365DeviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&dc); err != nil {
		return nil, 0, fmt.Errorf("decode device-code response: %w", err)
	}

	interval := dc.Interval
	if interval <= 0 {
		interval = 5 // MSFT default is 5 seconds
	}

	return &dc, interval, nil
}

// pollM365Token polls the M365 token endpoint once.
// Returns (token, false, nil) on success, (nil, true, nil) when still pending,
// or (nil, false, err) on a hard error.
func pollM365Token(ctx context.Context, client *http.Client, clientID, deviceCode string) (*m365TokenResponse, bool, error) {
	data := url.Values{}
	data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	data.Set("client_id", clientID)
	data.Set("device_code", deviceCode)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m365TokenURL,
		strings.NewReader(data.Encode()))
	if err != nil {
		return nil, false, fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return nil, false, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	var tok m365TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return nil, false, fmt.Errorf("decode token response: %w", err)
	}

	switch tok.Error {
	case "":
		// Success — access_token is populated.
		return &tok, false, nil
	case "authorization_pending":
		return nil, true, nil
	case "slow_down":
		// Server asked us to back off; caller loop will use the same interval,
		// which is sufficient because we already sleep before polling.
		return nil, true, nil
	default:
		return nil, false, fmt.Errorf("token error %q: %s", tok.Error, tok.ErrorDesc)
	}
}
