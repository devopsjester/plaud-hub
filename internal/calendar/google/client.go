// Package google provides a Google Calendar API client for reading calendar
// events. It uses only the Go standard library — no external Google SDK.
package google

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/devopsjester/plaud-hub/internal/calendar"
)

// calendarBaseURL is the root for Google Calendar API v3 calls.
const calendarBaseURL = "https://www.googleapis.com/calendar/v3"

// Client holds an access token for calls to Google Calendar API.
type Client struct {
	token      string
	httpClient *http.Client
}

// NewClient constructs a Client backed by a default TLS HTTP client.
// token must be a valid Bearer access token for the
// https://www.googleapis.com/auth/calendar.events.readonly scope.
func NewClient(token string) *Client {
	return &Client{
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

type googleEventDateTime struct {
	// DateTime is an RFC3339 string including timezone offset, e.g.
	// "2019-01-01T00:00:00Z" or "2019-01-01T09:00:00+09:00".
	DateTime string `json:"dateTime"`
	// Date is set for all-day events ("2019-01-01") instead of DateTime.
	Date string `json:"date"`
}

type googleAttendee struct {
	DisplayName string `json:"displayName"`
	Email       string `json:"email"`
}

type googleEvent struct {
	ID        string              `json:"id"`
	Summary   string              `json:"summary"`
	Start     googleEventDateTime `json:"start"`
	End       googleEventDateTime `json:"end"`
	Attendees []googleAttendee    `json:"attendees"`
}

type googleEventsResponse struct {
	Items []googleEvent `json:"items"`
}

// parseGoogleTime parses a googleEventDateTime into a UTC time.Time.
// RFC3339 strings carry full timezone information, so the conversion to UTC is
// unambiguous. All-day events (Date field only) are treated as midnight UTC on
// that date, which is a deliberate simplification — callers should be aware
// that an all-day event's "real" wall-clock start depends on the user's timezone.
func parseGoogleTime(gdt googleEventDateTime) (time.Time, error) {
	if gdt.DateTime != "" {
		t, err := time.Parse(time.RFC3339, gdt.DateTime)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse google datetime %q: %w", gdt.DateTime, err)
		}
		return t.UTC(), nil
	}
	if gdt.Date != "" {
		// All-day event: "2006-01-02" → midnight UTC.
		t, err := time.Parse("2006-01-02", gdt.Date)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse google date %q: %w", gdt.Date, err)
		}
		return t.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("google event has neither dateTime nor date field")
}

// ListEvents fetches calendar events from the primary calendar between from and
// to (UTC). It calls GET /calendars/primary/events with timeMin and timeMax as
// RFC3339 UTC strings.
func (c *Client) ListEvents(ctx context.Context, from, to time.Time) ([]calendar.CalendarEvent, error) {
	// RFC3339 with nanoseconds truncated to seconds, explicit Z suffix for UTC.
	timeMin := from.UTC().Format(time.RFC3339)
	timeMax := to.UTC().Format(time.RFC3339)

	u, err := url.Parse(calendarBaseURL + "/calendars/primary/events")
	if err != nil {
		return nil, fmt.Errorf("build events URL: %w", err)
	}
	q := u.Query()
	q.Set("timeMin", timeMin)
	q.Set("timeMax", timeMax)
	q.Set("singleEvents", "true") // expand recurring events
	q.Set("orderBy", "startTime")
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("events request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("events: unexpected status %d", resp.StatusCode)
	}

	var body googleEventsResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode events response: %w", err)
	}

	events := make([]calendar.CalendarEvent, 0, len(body.Items))
	for _, ge := range body.Items {
		start, err := parseGoogleTime(ge.Start)
		if err != nil {
			return nil, err
		}
		end, err := parseGoogleTime(ge.End)
		if err != nil {
			return nil, err
		}

		attendees := make([]calendar.Attendee, 0, len(ge.Attendees))
		for _, a := range ge.Attendees {
			attendees = append(attendees, calendar.Attendee{
				Name:  a.DisplayName,
				Email: a.Email,
			})
		}

		events = append(events, calendar.CalendarEvent{
			ID:        ge.ID,
			Title:     ge.Summary,
			Start:     start,
			End:       end,
			AllDay:    ge.Start.Date != "", // Google sets Date (not DateTime) for all-day events
			Attendees: attendees,
			Source:    "google",
		})
	}

	return events, nil
}
