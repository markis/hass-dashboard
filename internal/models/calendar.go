// Package models contains data structures for weather, calendar, and dashboard data.
package models

import (
	"fmt"
	"time"
)

// CalendarEventRaw represents a raw calendar event from the calendar API.
type CalendarEventRaw struct {
	Summary string        `json:"summary"`
	Start   EventDateTime `json:"start"`
	End     EventDateTime `json:"end"`
}

// EventDateTime represents a date/time in calendar events.
type EventDateTime struct {
	DateTime string `json:"dateTime,omitempty"`
	Date     string `json:"date,omitempty"`
}

// Event represents a parsed calendar event.
type Event struct {
	Start  time.Time
	End    time.Time
	Name   string
	AllDay bool
}

// ParseEvent parses a raw calendar event into an Event.
func ParseEvent(raw *CalendarEventRaw, loc *time.Location) (*Event, error) {
	start, allDay, err := parseEventDateTime(raw.Start, loc)
	if err != nil {
		return nil, fmt.Errorf("parsing start time for '%s': %w", raw.Summary, err)
	}

	end, _, err := parseEventDateTime(raw.End, loc)
	if err != nil {
		return nil, fmt.Errorf("parsing end time for '%s': %w", raw.Summary, err)
	}

	return &Event{
		Start:  start,
		End:    end,
		Name:   raw.Summary,
		AllDay: allDay,
	}, nil
}

func parseEventDateTime(edt EventDateTime, loc *time.Location) (time.Time, bool, error) {
	if edt.DateTime != "" {
		t, err := time.Parse(time.RFC3339, edt.DateTime)
		if err != nil {
			return time.Time{}, false, fmt.Errorf("parsing dateTime: %w", err)
		}

		return t.In(loc), false, nil
	}

	if edt.Date != "" {
		t, err := time.ParseInLocation("2006-01-02", edt.Date, loc)
		if err != nil {
			return time.Time{}, true, fmt.Errorf("parsing date: %w", err)
		}

		return t, true, nil
	}

	return time.Time{}, false, fmt.Errorf("no date or dateTime provided")
}

// DateCount represents a date with event count for the calendar display.
type DateCount struct {
	Day     time.Time
	Events  int
	IsPast  bool
	IsToday bool
}
