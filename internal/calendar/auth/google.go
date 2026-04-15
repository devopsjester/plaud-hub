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

// Google device-code OAuth endpoints.
const (
	googleDeviceCodeURL = "https://oauth2.googleapis.com/device/code"
	googleTokenURL      = "https://oauth2.googleapis.com/token"

	// googleScope is the minimum scope needed to read calendar events.
	googleScope = "https://www.googleapis.com/auth/calendar.events.readonly"
)

type googleDeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

type googleTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

// AuthorizeGoogle performs the OAuth 2.0 device-code flow for Google Calendar.
// It prints the user_code and verification_url to stdout so the user can
// authenticate in any browser. It polls until the authorization completes or
// the context is cancelled.
//
// clientID and clientSecret must be from a registered Google Cloud project with
// the Calendar API enabled. Token storage is the caller's responsibility.
func AuthorizeGoogle(ctx context.Context, clientID, clientSecret string) (accessToken, refreshToken string, err error) {
	httpClient := &http.Client{Timeout: 15 * time.Second}

	// Step 1: Request device and user codes.
	dcResp, interval, err := requestGoogleDeviceCode(ctx, httpClient, clientID)
	if err != nil {
		return "", "", err
	}

	fmt.Printf("Visit %s and enter code: %s\n", dcResp.VerificationURL, dcResp.UserCode)

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

		tok, pending, err := pollGoogleToken(ctx, httpClient, clientID, clientSecret, dcResp.DeviceCode)
		if err != nil {
			return "", "", err
		}
		if pending {
			continue
		}
		return tok.AccessToken, tok.RefreshToken, nil
	}
}

func requestGoogleDeviceCode(ctx context.Context, client *http.Client, clientID string) (*googleDeviceCodeResponse, int, error) {
	data := url.Values{}
	data.Set("client_id", clientID)
	data.Set("scope", googleScope)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleDeviceCodeURL,
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

	var dc googleDeviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&dc); err != nil {
		return nil, 0, fmt.Errorf("decode device-code response: %w", err)
	}

	interval := dc.Interval
	if interval <= 0 {
		interval = 5 // Google default is 5 seconds
	}

	return &dc, interval, nil
}

// pollGoogleToken polls the Google token endpoint once.
// Returns (token, false, nil) on success, (nil, true, nil) when still pending,
// or (nil, false, err) on a hard error.
func pollGoogleToken(ctx context.Context, client *http.Client, clientID, clientSecret, deviceCode string) (*googleTokenResponse, bool, error) {
	data := url.Values{}
	data.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)
	data.Set("device_code", deviceCode)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenURL,
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

	var tok googleTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tok); err != nil {
		return nil, false, fmt.Errorf("decode token response: %w", err)
	}

	switch tok.Error {
	case "":
		return &tok, false, nil
	case "authorization_pending":
		return nil, true, nil
	case "slow_down":
		// Google uses the same "slow_down" error as MSFT; caller loop handles
		// timing via the fixed interval.
		return nil, true, nil
	default:
		return nil, false, fmt.Errorf("token error %q: %s", tok.Error, tok.ErrorDesc)
	}
}
