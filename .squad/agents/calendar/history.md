## Learnings

### 2026-04-15 — Initial calendar package design and implementation

**Package structure chosen**

```
internal/calendar/
  event.go          — shared CalendarEvent and Attendee models
  correlate.go      — MatchRecording time-window matching
  m365/
    client.go       — Microsoft Graph calendarView client (stdlib net/http)
  google/
    client.go       — Google Calendar Events API client (stdlib net/http)
  auth/
    m365.go         — M365 device-code OAuth flow
    google.go       — Google device-code OAuth flow
internal/config/
  calendar.go       — SaveCalendarToken / LoadCalendarToken helpers
```

**Key design decisions**

- UTC-only comparisons: all `time.Time` values stored and returned as UTC. `MatchRecording` forces `.UTC()` on every operand before comparison — no assumptions about caller timezone.
- DST boundary flag: M365 Graph API returns Windows timezone names (e.g. "Eastern Standard Time") that Go's `time.LoadLocation` does not understand. The client sends `Prefer: outlook.timezone="UTC"` to force UTC responses from Graph, sidestepping the entire CLDR-mapping problem. This is the correct production-grade approach.
- Google all-day events: `date`-only events (no `dateTime`) are mapped to midnight UTC on that date. This is a deliberate simplification — a recording on an "all-day event" day will match if the recording happened during the 24-hour UTC day. If the user is in UTC-5, events at 23:00 local (04:00 UTC next day) will NOT match. Documented in code; revisit if it becomes a problem.
- Device-code flow: no browser redirect needed. Works in headless/SSH/terminal environments. Both providers follow the same polling pattern (authorization_pending → slow_down → success/error).
- No external SDK: `net/http` + `encoding/json` only. Zero additions to go.mod.
- Token storage: reuses the existing `plaud-hub.yaml` config file and the existing ReadInConfig→Set→WriteConfigAs→Chmod-600 pattern from `SaveToken`.

**Open questions for Architect**

- Token refresh is not implemented. M365 refresh uses `grant_type=refresh_token` against the same token URL. Google uses `grant_type=refresh_token` against `https://oauth2.googleapis.com/token`. Should refresh be automatic (transparent retry on 401) or explicit (caller calls `RefreshM365Token`)?
- Pagination: both Graph calendarView and Google Events API paginate via `@odata.nextLink` / `nextPageToken`. Current clients return only the first page. Need to add pagination before production use.
- M365 tenant: currently using `/common` tenant which covers both personal (MSA) and work/school (AAD) accounts. If enterprise customers need only AAD, the tenant should be configurable.
- Client ID registration: callers must supply their own Azure AD and Google Cloud project client IDs. There is no default. This needs to be documented in the README and wired into the config (e.g. `calendar.m365.client_id`, `calendar.google.client_id`).
