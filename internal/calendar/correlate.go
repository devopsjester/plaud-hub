package calendar

import "time"

// MatchRecording finds the best CalendarEvent whose window contains
// recordingStart. The search window is [event.Start - tolerance, event.End +
// tolerance]. If multiple events match, the one with the narrowest window
// (smallest event.End - event.Start) is preferred, which handles back-to-back
// meetings gracefully.
//
// All comparisons are performed in UTC regardless of the timezone stored in the
// time values. Callers must ensure recordingStart is meaningful (non-zero).
//
// Returns nil when no event matches.
func MatchRecording(recordingStart time.Time, events []CalendarEvent, tolerance time.Duration) *CalendarEvent {
	// Normalize to UTC; DST has no effect here because Go's UTC location carries
	// no DST rules. Callers who load times from local or named zones must be
	// aware that time.Time.UTC() strips the zone label but preserves the instant.
	start := recordingStart.UTC()

	var best *CalendarEvent
	var bestWindow time.Duration

	for i := range events {
		ev := &events[i]

		lo := ev.Start.UTC().Add(-tolerance)
		hi := ev.End.UTC().Add(tolerance)

		if start.Before(lo) || start.After(hi) {
			continue
		}

		window := ev.End.UTC().Sub(ev.Start.UTC())
		if best == nil || window < bestWindow {
			best = ev
			bestWindow = window
		}
	}

	return best
}
