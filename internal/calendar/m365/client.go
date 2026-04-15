// Package m365 provides a Microsoft Graph API client for reading calendar
// events. It uses only the Go standard library — no external Graph SDK.
package m365

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/devopsjester/plaud-hub/internal/calendar"
)

// graphBaseURL is the root for Microsoft Graph v1 API calls.
const graphBaseURL = "https://graph.microsoft.com/v1.0"

// Client holds an access token for calls to Microsoft Graph.
type Client struct {
	token      string
	httpClient *http.Client
}

// NewClient constructs a Client backed by a default TLS HTTP client.
// token must be a valid Bearer access token for the Calendars.Read scope.
func NewClient(token string) *Client {
	return &Client{
		token:      token,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// graphEventDateTime carries the start/end date-time structure returned by
// the Graph API.
type graphEventDateTime struct {
	// DateTime is formatted as "2006-01-02T15:04:05.0000000" — no timezone suffix.
	DateTime string `json:"dateTime"`
	// TimeZone is a Windows timezone name (e.g., "UTC", "Eastern Standard Time").
	// NOTE: Go's time.LoadLocation does not understand Windows timezone names.
	// The conversion below handles only the "UTC" case explicitly; all others
	// are treated as UTC with a warning comment. A future improvement should use
	// a CLDR/TZID mapping table or accept only UTC from Graph by requesting
	// Prefer: outlook.timezone="UTC" in the request header.
	TimeZone string `json:"timeZone"`
}

type graphAttendee struct {
	EmailAddress struct {
		Name    string `json:"name"`
		Address string `json:"address"`
	} `json:"emailAddress"`
}

type graphEvent struct {
	ID        string             `json:"id"`
	Subject   string             `json:"subject"`
	Start     graphEventDateTime `json:"start"`
	End       graphEventDateTime `json:"end"`
	Attendees []graphAttendee    `json:"attendees"`
}

type graphCalendarViewResponse struct {
	Value []graphEvent `json:"value"`
}

// parseGraphTime parses a graphEventDateTime into a UTC time.Time.
// To avoid Windows-timezone ambiguity, this client always requests times in UTC
// from Graph (via the Prefer header). If the server returns a non-UTC timezone
// the field is parsed as UTC and callers should validate with their own audit.
func parseGraphTime(gdt graphEventDateTime) (time.Time, error) {
	// Graph returns "2006-01-02T15:04:05.0000000" without a zone suffix.
	const layout = "2006-01-02T15:04:05.0000000"
	t, err := time.Parse(layout, gdt.DateTime)
	if err != nil {
		// Fallback: try without sub-second precision.
		const layoutShort = "2006-01-02T15:04:05"
		t, err = time.Parse(layoutShort, gdt.DateTime)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse graph datetime %q: %w", gdt.DateTime, err)
		}
	}
	// time.Parse with no location yields UTC already; call UTC() to be explicit.
	return t.UTC(), nil
}

// ListEvents fetches calendar events visible between from and to (UTC).
// It calls GET /me/calendarView with the Prefer: outlook.timezone="UTC" header
// so that all returned date-times are UTC, sidestepping Windows timezone name
// resolution.
func (c *Client) ListEvents(ctx context.Context, from, to time.Time) ([]calendar.CalendarEvent, error) {
	// Always send UTC bounds to the API.
	startStr := from.UTC().Format(time.RFC3339)
	endStr := to.UTC().Format(time.RFC3339)

	u, err := url.Parse(graphBaseURL + "/me/calendarView")
	if err != nil {
		return nil, fmt.Errorf("build calendarView URL: %w", err)
	}
	q := u.Query()
	q.Set("startDateTime", startStr)
	q.Set("endDateTime", endStr)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Prefer", `outlook.timezone="UTC"`)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calendar view request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("calendar view: unexpected status %d", resp.StatusCode)
	}

	var body graphCalendarViewResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("decode calendar view response: %w", err)
	}

	events := make([]calendar.CalendarEvent, 0, len(body.Value))
	for _, ge := range body.Value {
		start, err := parseGraphTime(ge.Start)
		if err != nil {
			return nil, err
		}
		end, err := parseGraphTime(ge.End)
		if err != nil {
			return nil, err
		}

		attendees := make([]calendar.Attendee, 0, len(ge.Attendees))
		for _, a := range ge.Attendees {
			attendees = append(attendees, calendar.Attendee{
				Name:  a.EmailAddress.Name,
				Email: a.EmailAddress.Address,
			})
		}

		events = append(events, calendar.CalendarEvent{
			ID:        ge.ID,
			Title:     ge.Subject,
			Start:     start,
			End:       end,
			Attendees: attendees,
			Source:    "m365",
		})
	}

	return events, nil
}
