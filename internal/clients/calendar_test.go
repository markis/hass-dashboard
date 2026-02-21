package clients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/markis/hass-dashboard/internal/models"
)

func TestCalendarClientGetCalendars(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	now := time.Now().In(loc)
	startTime := now.Format(time.RFC3339)
	endTime := now.Add(24 * time.Hour).Format(time.RFC3339)

	events := []models.CalendarEventRaw{
		{
			Summary: "Test Event 1",
			Start:   models.EventDateTime{DateTime: startTime},
			End:     models.EventDateTime{DateTime: endTime},
		},
		{
			Summary: "Test Event 2",
			Start:   models.EventDateTime{DateTime: startTime},
			End:     models.EventDateTime{DateTime: endTime},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer test-token" {
			t.Errorf("Authorization header = %q, want %q", authHeader, "Bearer test-token")
		}

		// Verify content type
		contentType := r.Header.Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(events)
	}))
	defer server.Close()

	client, err := NewCalendarClient(server.URL+"/api/", "test-token", loc)
	if err != nil {
		t.Fatalf("Failed to create calendar client: %v", err)
	}

	result, err := client.GetCalendars(
		context.Background(),
		[]string{"test_calendar"},
		now,
		now.Add(7*24*time.Hour),
	)
	if err != nil {
		t.Fatalf("GetCalendars error: %v", err)
	}

	// Should have events grouped by date
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

		events := []models.CalendarEventRaw{
			{
				Summary: "Event from calendar " + r.URL.Path,
				Start:   models.EventDateTime{DateTime: startTime},
				End:     models.EventDateTime{DateTime: endTime},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(events)
	}))
	defer server.Close()

	client, err := NewCalendarClient(server.URL+"/api/", "token", loc)
	if err != nil {
		t.Fatalf("Failed to create calendar client: %v", err)
	}

	_, err = client.GetCalendars(
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

	client, err := NewCalendarClient(server.URL+"/api/", "token", loc)
	if err != nil {
		t.Fatalf("Failed to create calendar client: %v", err)
	}

	_, err = client.GetCalendars(
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

	client, err := NewCalendarClient(server.URL+"/api/", "token", loc)
	if err != nil {
		t.Fatalf("Failed to create calendar client: %v", err)
	}

	_, err = client.GetCalendars(
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

	events := []models.CalendarEventRaw{
		{
			Summary: "Valid Event",
			Start:   models.EventDateTime{DateTime: startTime},
			End:     models.EventDateTime{DateTime: endTime},
		},
		{
			Summary: "Invalid Event",
			Start:   models.EventDateTime{}, // Missing date
			End:     models.EventDateTime{DateTime: endTime},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(events)
	}))
	defer server.Close()

	client, err := NewCalendarClient(server.URL+"/api/", "token", loc)
	if err != nil {
		t.Fatalf("Failed to create calendar client: %v", err)
	}

	result, err := client.GetCalendars(
		context.Background(),
		[]string{"test"},
		now,
		now.Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("GetCalendars error: %v", err)
	}

	// Should only have 1 valid event
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

	client, err := NewCalendarClient(server.URL+"/api/", "token", loc)
	if err != nil {
		t.Fatalf("Failed to create calendar client: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err = client.GetCalendars(
		ctx,
		[]string{"test"},
		time.Now(),
		time.Now().Add(24*time.Hour),
	)

	if err == nil {
		t.Error("expected error for canceled context")
	}
}
