// Package clients provides HTTP clients for external APIs.
package clients

import (
	"context"
	"fmt"
	"log"
	"os"
	"sort"
	"time"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/option"

	"github.com/markis/hass-dashboard/internal/models"
)

// CalendarClient fetches calendar events from the Google Calendar API using a
// service account with domain-wide delegation.
type CalendarClient struct {
	service  *calendar.Service
	location *time.Location
}

// NewCalendarClient creates a calendar client authenticated with a Google
// service account. credentialsFile is the path to the service account key JSON.
// impersonate is the Workspace user email to impersonate (required for
// domain-wide delegation); leave empty only for calendars the service account
// itself owns. The Calendar API is enabled in the Google Cloud project that
// owns the service account, and the service account's client ID must be
// authorized in the Workspace Admin console for the
// https://www.googleapis.com/auth/calendar.readonly scope.
func NewCalendarClient(
	ctx context.Context,
	credentialsFile, impersonate string,
	loc *time.Location,
) (*CalendarClient, error) {
	keyData, err := os.ReadFile(credentialsFile) //nolint:gosec // Path from admin config
	if err != nil {
		return nil, fmt.Errorf("reading service account credentials %s: %w", credentialsFile, err)
	}

	jwtConfig, err := google.JWTConfigFromJSON(keyData, calendar.CalendarReadonlyScope)
	if err != nil {
		return nil, fmt.Errorf("parsing service account credentials: %w", err)
	}

	// Impersonate a Workspace user so the service account can read their
	// calendars via domain-wide delegation.
	if impersonate != "" {
		jwtConfig.Subject = impersonate
	}

	srv, err := calendar.NewService(ctx, option.WithHTTPClient(jwtConfig.Client(ctx)))
	if err != nil {
		return nil, fmt.Errorf("creating calendar service: %w", err)
	}

	return &CalendarClient{service: srv, location: loc}, nil
}

// GetCalendars fetches events from multiple Google calendars and groups them by
// date (in the client's configured location).
func (c *CalendarClient) GetCalendars(
	ctx context.Context,
	calendarIDs []string,
	start, end time.Time,
) (map[time.Time][]models.Event, error) {
	allEvents := make([]models.Event, 0)

	for _, calID := range calendarIDs {
		events, err := c.fetchCalendarEvents(ctx, calID, start, end)
		if err != nil {
			return nil, fmt.Errorf("fetching calendar %s: %w", calID, err)
		}

		allEvents = append(allEvents, events...)
	}

	return groupEventsByDate(allEvents, c.location), nil
}

func (c *CalendarClient) fetchCalendarEvents(
	ctx context.Context,
	calendarID string,
	start, end time.Time,
) ([]models.Event, error) {
	log.Printf("Fetching calendar events from: %s", calendarID)

	var events []models.Event

	pageToken := ""

	for {
		call := c.service.Events.List(calendarID).
			SingleEvents(true).
			TimeMin(start.Format(time.RFC3339)).
			TimeMax(end.Format(time.RFC3339)).
			OrderBy("startTime").
			Context(ctx)

		if pageToken != "" {
			call = call.PageToken(pageToken)
		}

		resp, err := call.Do()
		if err != nil {
			return nil, fmt.Errorf("listing events: %w", err)
		}

		for idx := range resp.Items {
			event, err := toModelEvent(resp.Items[idx], c.location)
			if err != nil {
				// Skip events that can't be parsed rather than failing the batch.
				log.Printf("Skipping unparseable event in %s: %v", calendarID, err)

				continue
			}

			events = append(events, *event)
		}

		pageToken = resp.NextPageToken
		if pageToken == "" {
			break
		}
	}

	return events, nil
}

// toModelEvent converts a Google Calendar event into the dashboard's Event
// model. Reuses the models parser so date/dateTime handling stays consistent.
func toModelEvent(ev *calendar.Event, loc *time.Location) (*models.Event, error) {
	raw := models.CalendarEventRaw{
		Summary: ev.Summary,
	}

	if ev.Start != nil {
		raw.Start = models.EventDateTime{DateTime: ev.Start.DateTime, Date: ev.Start.Date}
	}

	if ev.End != nil {
		raw.End = models.EventDateTime{DateTime: ev.End.DateTime, Date: ev.End.Date}
	}

	event, err := models.ParseEvent(&raw, loc)
	if err != nil {
		return nil, fmt.Errorf("parsing event %q: %w", ev.Summary, err)
	}

	return event, nil
}

// groupEventsByDate sorts events by start time and groups them into a map keyed
// by the calendar date (midnight in loc) of each event's start time.
func groupEventsByDate(events []models.Event, loc *time.Location) map[time.Time][]models.Event {
	sort.Slice(events, func(i, j int) bool {
		return events[i].Start.Before(events[j].Start)
	})

	grouped := make(map[time.Time][]models.Event)

	for _, event := range events {
		date := time.Date(
			event.Start.Year(), event.Start.Month(), event.Start.Day(),
			0, 0, 0, 0, loc,
		)
		grouped[date] = append(grouped[date], event)
	}

	return grouped
}
