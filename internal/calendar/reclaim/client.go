// Package reclaim provides a Reclaim.ai API client for reading calendar events.
// It uses only the Go standard library — no external SDK.
//
// Authentication: API key passed as a Bearer token.
// Endpoint: https://api.app.reclaim.ai/api/events?start=YYYY-MM-DD&end=YYYY-MM-DD
package reclaim

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/devopsjester/plaud-hub/internal/calendar"
)

// BaseURL is the root URL for the Reclaim API. Override in tests.
var BaseURL = "https://api.app.reclaim.ai/api"

// Client holds an API key for calls to the Reclaim.ai API.
type Client struct {
	apiKey     string
	httpClient *http.Client
}

// NewClient constructs a Client with the given Reclaim API key.
func NewClient(apiKey string) *Client {
	return &Client{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

type reclaimAttendee struct {
	Email       string `json:"email"`
	Self        bool   `json:"self"`
	DisplayName string `json:"displayName"`
}

type reclaimEvent struct {
	EventID    string            `json:"eventId"`
	Title      string            `json:"title"`
	EventStart string            `json:"eventStart"` // RFC3339 with offset
	EventEnd   string            `json:"eventEnd"`
	Attendees  []reclaimAttendee `json:"attendees"`
	AllDay     bool              `json:"allDay"`
}

// ListEvents returns all calendar events between from and to (inclusive).
// from and to are truncated to date boundaries.
// The Reclaim API end parameter is exclusive, so we add one day.
func (c *Client) ListEvents(ctx context.Context, from, to time.Time) ([]calendar.CalendarEvent, error) {
	params := url.Values{}
	params.Set("start", from.Format("2006-01-02"))
	params.Set("end", to.AddDate(0, 0, 1).Format("2006-01-02")) // end is exclusive

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		BaseURL+"/events?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("reclaim API returned %s", resp.Status)
	}

	var raw []reclaimEvent
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode reclaim response: %w", err)
	}

	events := make([]calendar.CalendarEvent, 0, len(raw))
	for _, r := range raw {
		ev, err := toCalendarEvent(r)
		if err != nil {
			continue // skip unparseable events
		}
		events = append(events, ev)
	}
	return events, nil
}

func toCalendarEvent(r reclaimEvent) (calendar.CalendarEvent, error) {
	start, err := time.Parse(time.RFC3339Nano, r.EventStart)
	if err != nil {
		// Try without nanoseconds (e.g. "2026-02-18T09:00:00.000-04:00")
		start, err = time.Parse("2006-01-02T15:04:05.000-07:00", r.EventStart)
		if err != nil {
			return calendar.CalendarEvent{}, fmt.Errorf("parse event start %q: %w", r.EventStart, err)
		}
	}
	end, err := time.Parse(time.RFC3339Nano, r.EventEnd)
	if err != nil {
		end, err = time.Parse("2006-01-02T15:04:05.000-07:00", r.EventEnd)
		if err != nil {
			return calendar.CalendarEvent{}, fmt.Errorf("parse event end %q: %w", r.EventEnd, err)
		}
	}

	attendees := make([]calendar.Attendee, 0, len(r.Attendees))
	for _, a := range r.Attendees {
		if a.Email == "" {
			continue
		}
		attendees = append(attendees, calendar.Attendee{
			Name:  a.DisplayName,
			Email: a.Email,
		})
	}

	return calendar.CalendarEvent{
		ID:        r.EventID,
		Title:     r.Title,
		Start:     start.UTC(),
		End:       end.UTC(),
		AllDay:    r.AllDay,
		Attendees: attendees,
		Source:    "reclaim",
	}, nil
}
