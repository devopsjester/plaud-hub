// Package calendar defines shared types and matching logic for calendar
// integration. All time values are stored and compared in UTC.
package calendar

import "time"

// CalendarEvent represents a single calendar event fetched from a provider.
// All time fields are in UTC.
type CalendarEvent struct {
	ID        string
	Title     string
	Start     time.Time // UTC
	End       time.Time // UTC
	AllDay    bool      // true when the event spans a full day with no time component
	Attendees []Attendee
	Source    string // "reclaim" or "google"
}

// Attendee is a meeting participant.
type Attendee struct {
	Name  string
	Email string
}
