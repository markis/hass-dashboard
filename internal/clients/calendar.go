// Package clients provides HTTP clients for external APIs.
package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/markis/hass-dashboard/internal/models"
)

// CalendarClient fetches calendar events from Home Assistant.
type CalendarClient struct {
	httpClient *http.Client
	apiURL     string
	token      string
	location   *time.Location
}

// NewCalendarClient creates a new calendar client.
func NewCalendarClient(apiURL, token string, loc *time.Location) *CalendarClient {
	return &CalendarClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		apiURL:     apiURL,
		token:      token,
		location:   loc,
	}
}

// GetCalendars fetches events from multiple calendars.
func (c *CalendarClient) GetCalendars(
	ctx context.Context,
	calendarIDs []string,
	start, end time.Time,
) (map[time.Time][]models.Event, error) {
	startStr := start.Format(time.RFC3339)
	endStr := end.Format(time.RFC3339)

	// Collect all events
	allEvents := make([]models.Event, 0)

	for _, calID := range calendarIDs {
		events, err := c.fetchCalendarEvents(ctx, calID, startStr, endStr)
		if err != nil {
			return nil, fmt.Errorf("fetching calendar %s: %w", calID, err)
		}

		allEvents = append(allEvents, events...)
	}

	// Sort events by start time
	sort.Slice(allEvents, func(i, j int) bool {
		return allEvents[i].Start.Before(allEvents[j].Start)
	})

	// Group by date
	grouped := make(map[time.Time][]models.Event)

	for _, event := range allEvents {
		date := time.Date(
			event.Start.Year(), event.Start.Month(), event.Start.Day(),
			0, 0, 0, 0, c.location,
		)
		grouped[date] = append(grouped[date], event)
	}

	return grouped, nil
}

func (c *CalendarClient) fetchCalendarEvents(
	ctx context.Context,
	calendarID, startStr, endStr string,
) ([]models.Event, error) {
	// Ensure calendar ID has the "calendar." prefix
	if !strings.HasPrefix(calendarID, "calendar.") {
		calendarID = "calendar." + calendarID
	}

	// Build URL with properly encoded query parameters
	baseURL := fmt.Sprintf("%scalendars/%s", c.apiURL, calendarID)

	reqURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing base URL: %w", err)
	}

	query := reqURL.Query()
	query.Set("start", startStr)
	query.Set("end", endStr)
	reqURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	log.Printf("Fetching calendar events from: %s", reqURL.String())

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body) //nolint:errcheck // Best effort for error logging
		log.Printf("Calendar API error (status %d): %s", resp.StatusCode, string(body))

		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var rawEvents []models.CalendarEventRaw
	if err := json.NewDecoder(resp.Body).Decode(&rawEvents); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	events := make([]models.Event, 0, len(rawEvents))

	for idx := range rawEvents {
		event, err := models.ParseEvent(&rawEvents[idx], c.location)
		if err != nil {
			// Skip events that can't be parsed
			continue
		}

		events = append(events, *event)
	}

	return events, nil
}
