package models

import (
	"testing"
	"time"
)

func TestParseEvent(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")

	tests := []struct {
		name    string
		raw     CalendarEventRaw
		wantErr bool
		check   func(*testing.T, *Event)
	}{
		{
			name: "datetime event",
			raw: CalendarEventRaw{
				Summary: "Meeting",
				Start:   EventDateTime{DateTime: "2024-01-15T10:00:00-05:00"},
				End:     EventDateTime{DateTime: "2024-01-15T11:00:00-05:00"},
			},
			wantErr: false,
			check: func(t *testing.T, e *Event) {
				t.Helper()

				if e.Name != "Meeting" {
					t.Errorf("Name = %q, want %q", e.Name, "Meeting")
				}

				if e.AllDay {
					t.Error("AllDay should be false for datetime event")
				}

				if e.Start.Hour() != 10 {
					t.Errorf("Start hour = %d, want 10", e.Start.Hour())
				}
			},
		},
		{
			name: "all day event",
			raw: CalendarEventRaw{
				Summary: "Holiday",
				Start:   EventDateTime{Date: "2024-01-15"},
				End:     EventDateTime{Date: "2024-01-16"},
			},
			wantErr: false,
			check: func(t *testing.T, e *Event) {
				t.Helper()

				if e.Name != "Holiday" {
					t.Errorf("Name = %q, want %q", e.Name, "Holiday")
				}

				if !e.AllDay {
					t.Error("AllDay should be true for date-only event")
				}
			},
		},
		{
			name: "missing start date",
			raw: CalendarEventRaw{
				Summary: "Bad Event",
				Start:   EventDateTime{},
				End:     EventDateTime{Date: "2024-01-16"},
			},
			wantErr: true,
		},
		{
			name: "missing end date",
			raw: CalendarEventRaw{
				Summary: "Bad Event",
				Start:   EventDateTime{Date: "2024-01-15"},
				End:     EventDateTime{},
			},
			wantErr: true,
		},
		{
			name: "invalid datetime format",
			raw: CalendarEventRaw{
				Summary: "Bad Event",
				Start:   EventDateTime{DateTime: "not-a-date"},
				End:     EventDateTime{DateTime: "2024-01-15T11:00:00-05:00"},
			},
			wantErr: true,
		},
		{
			name: "invalid date format",
			raw: CalendarEventRaw{
				Summary: "Bad Event",
				Start:   EventDateTime{Date: "01-15-2024"},
				End:     EventDateTime{Date: "2024-01-16"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := ParseEvent(&tt.raw, loc)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}

				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)

				return
			}

			if tt.check != nil {
				tt.check(t, event)
			}
		})
	}
}

func TestDateCount(t *testing.T) {
	now := time.Now()
	dc := DateCount{
		Day:     now,
		Events:  3,
		IsPast:  false,
		IsToday: true,
	}

	if dc.Events != 3 {
		t.Errorf("Events = %d, want 3", dc.Events)
	}

	if dc.IsPast {
		t.Error("IsPast should be false")
	}

	if !dc.IsToday {
		t.Error("IsToday should be true")
	}

	// Use dc.Day to avoid unused warning
	_ = dc.Day
}
