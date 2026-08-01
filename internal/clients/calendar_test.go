package clients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/oauth2"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/markis/hass-dashboard/internal/models"
)

// newTestClient builds a CalendarClient whose underlying Google service points
// at a test server, bypassing service-account JWT auth with a static token
// source.
func newTestClient(t *testing.T, server *httptest.Server, loc *time.Location) *CalendarClient {
	t.Helper()

	// A static token source skips real OAuth/JWT exchange; the test server
	// ignores the bearer token anyway.
	ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "test-token"})
	httpClient := oauth2.NewClient(context.Background(), ts)

	srv, err := calendar.NewService(
		context.Background(),
		option.WithHTTPClient(httpClient),
		option.WithEndpoint(server.URL),
	)
	if err != nil {
		t.Fatalf("creating test calendar service: %v", err)
	}

	return &CalendarClient{service: srv, location: loc}
}

// googleEvent is a minimal struct matching the Calendar API events.list JSON
// response shape needed by the client.
type googleEvent struct {
	Summary string                  `json:"summary,omitempty"`
	Start   *calendar.EventDateTime `json:"start,omitempty"`
	End     *calendar.EventDateTime `json:"end,omitempty"`
}

func writeEvents(t *testing.T, w http.ResponseWriter, items []googleEvent) {
	t.Helper()
	resp := calendar.Events{Items: make([]*calendar.Event, 0, len(items))}
	for i := range items {
		resp.Items = append(resp.Items, &calendar.Event{
			Summary: items[i].Summary,
			Start:   items[i].Start,
			End:     items[i].End,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(&resp); err != nil {
		t.Fatalf("encoding events: %v", err)
	}
}

func TestCalendarClientGetCalendars(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	now := time.Now().In(loc)
	startTime := now.Format(time.RFC3339)
	endTime := now.Add(24 * time.Hour).Format(time.RFC3339)

	events := []googleEvent{
		{
			Summary: "Test Event 1",
			Start:   &calendar.EventDateTime{DateTime: startTime},
			End:     &calendar.EventDateTime{DateTime: endTime},
		},
		{
			Summary: "Test Event 2",
			Start:   &calendar.EventDateTime{DateTime: startTime},
			End:     &calendar.EventDateTime{DateTime: endTime},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeEvents(t, w, events)
	}))
	defer server.Close()

	client := newTestClient(t, server, loc)

	result, err := client.GetCalendars(
		context.Background(),
		[]string{"test_calendar"},
		now,
		now.Add(7*24*time.Hour),
	)
	if err != nil {
		t.Fatalf("GetCalendars error: %v", err)
	}

	totalEvents := 0
	for _, evts := range result {
		totalEvents += len(evts)
	}

	if totalEvents != 2 {
		t.Errorf("total events = %d, want 2", totalEvents)
	}
}

func TestCalendarClientGetCalendarsMultiple(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	now := time.Now().In(loc)
	startTime := now.Format(time.RFC3339)
	endTime := now.Add(1 * time.Hour).Format(time.RFC3339)

	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++

		events := []googleEvent{
			{
				Summary: "Event from calendar " + r.URL.Path,
				Start:   &calendar.EventDateTime{DateTime: startTime},
				End:     &calendar.EventDateTime{DateTime: endTime},
			},
		}
		writeEvents(t, w, events)
	}))
	defer server.Close()

	client := newTestClient(t, server, loc)

	_, err := client.GetCalendars(
		context.Background(),
		[]string{"cal1", "cal2", "cal3"},
		now,
		now.Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("GetCalendars error: %v", err)
	}

	if requestCount != 3 {
		t.Errorf("request count = %d, want 3", requestCount)
	}
}

func TestCalendarClientGetCalendarsError(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := newTestClient(t, server, loc)

	_, err := client.GetCalendars(
		context.Background(),
		[]string{"test"},
		time.Now(),
		time.Now().Add(24*time.Hour),
	)

	if err == nil {
		t.Error("expected error for 500 response")
	}
}

func TestCalendarClientGetCalendarsInvalidJSON(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not valid json"))
	}))
	defer server.Close()

	client := newTestClient(t, server, loc)

	_, err := client.GetCalendars(
		context.Background(),
		[]string{"test"},
		time.Now(),
		time.Now().Add(24*time.Hour),
	)

	if err == nil {
		t.Error("expected error for invalid JSON response")
	}
}

func TestCalendarClientSkipsInvalidEvents(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	now := time.Now().In(loc)
	startTime := now.Format(time.RFC3339)
	endTime := now.Add(1 * time.Hour).Format(time.RFC3339)

	events := []googleEvent{
		{
			Summary: "Valid Event",
			Start:   &calendar.EventDateTime{DateTime: startTime},
			End:     &calendar.EventDateTime{DateTime: endTime},
		},
		{
			Summary: "Invalid Event",
			Start:   &calendar.EventDateTime{}, // Missing date
			End:     &calendar.EventDateTime{DateTime: endTime},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeEvents(t, w, events)
	}))
	defer server.Close()

	client := newTestClient(t, server, loc)

	result, err := client.GetCalendars(
		context.Background(),
		[]string{"test"},
		now,
		now.Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("GetCalendars error: %v", err)
	}

	totalEvents := 0
	for _, evts := range result {
		totalEvents += len(evts)
	}

	if totalEvents != 1 {
		t.Errorf("total events = %d, want 1 (invalid event should be skipped)", totalEvents)
	}
}

func TestCalendarClientContextCancellation(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := newTestClient(t, server, loc)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := client.GetCalendars(
		ctx,
		[]string{"test"},
		time.Now(),
		time.Now().Add(24*time.Hour),
	)

	if err == nil {
		t.Error("expected error for canceled context")
	}
}

func TestGroupEventsByDate(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")

	day1 := time.Date(2025, 1, 1, 10, 0, 0, 0, loc)
	day2 := time.Date(2025, 1, 2, 14, 0, 0, 0, loc)

	events := []models.Event{
		{Start: day2, End: day2.Add(time.Hour), Name: "later"},
		{Start: day1, End: day1.Add(time.Hour), Name: "earlier"},
		{Start: day1.Add(2 * time.Hour), End: day1.Add(3 * time.Hour), Name: "earlier-later"},
	}

	grouped := groupEventsByDate(events, loc)

	keys := make([]time.Time, 0, len(grouped))
	for k := range grouped {
		keys = append(keys, k)
	}

	if len(keys) != 2 {
		t.Fatalf("grouped dates = %d, want 2", len(keys))
	}

	// Events on the same date should be sorted by start time.
	d1 := time.Date(2025, 1, 1, 0, 0, 0, 0, loc)
	if len(grouped[d1]) != 2 {
		t.Fatalf("events on 2025-01-01 = %d, want 2", len(grouped[d1]))
	}

	if grouped[d1][0].Name != "earlier" {
		t.Errorf("first event on 2025-01-01 = %q, want %q", grouped[d1][0].Name, "earlier")
	}

	if grouped[d1][1].Name != "earlier-later" {
		t.Errorf("second event on 2025-01-01 = %q, want %q", grouped[d1][1].Name, "earlier-later")
	}
}
