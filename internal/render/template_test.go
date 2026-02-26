package render

import (
	"strings"
	"testing"
	"time"

	"github.com/markis/hass-dashboard/internal/models"
)

func TestGetCalendarDates(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")

	tests := []struct {
		name       string
		date       time.Time
		weeks      int
		wantLen    int
		wantSunday bool
	}{
		{
			name:       "4 weeks from Wednesday",
			date:       time.Date(2024, 1, 17, 12, 0, 0, 0, loc), // Wednesday
			weeks:      4,
			wantLen:    28,
			wantSunday: true, // Should start on Sunday Jan 14
		},
		{
			name:       "2 weeks from Sunday",
			date:       time.Date(2024, 1, 14, 12, 0, 0, 0, loc), // Sunday
			weeks:      2,
			wantLen:    14,
			wantSunday: true,
		},
		{
			name:       "1 week from Saturday",
			date:       time.Date(2024, 1, 20, 12, 0, 0, 0, loc), // Saturday
			weeks:      1,
			wantLen:    7,
			wantSunday: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dates, end := GetCalendarDates(tt.date, tt.weeks)

			if len(dates) != tt.wantLen {
				t.Errorf("len(dates) = %d, want %d", len(dates), tt.wantLen)
			}

			if tt.wantSunday && dates[0].Weekday() != time.Sunday {
				t.Errorf("first day = %v, want Sunday", dates[0].Weekday())
			}

			// End should be after all dates
			if !end.After(dates[len(dates)-1]) {
				t.Errorf("end %v should be after last date %v", end, dates[len(dates)-1])
			}
		})
	}
}

func TestGenerateDatesWithEvents(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	today := time.Date(2024, 1, 15, 12, 0, 0, 0, loc)
	todayStart := time.Date(2024, 1, 15, 0, 0, 0, 0, loc)

	dates := []time.Time{
		time.Date(2024, 1, 14, 0, 0, 0, 0, loc), // Yesterday
		time.Date(2024, 1, 15, 0, 0, 0, 0, loc), // Today
		time.Date(2024, 1, 16, 0, 0, 0, 0, loc), // Tomorrow
	}

	events := map[time.Time][]models.Event{
		todayStart: {
			{Name: "Event 1"},
			{Name: "Event 2"},
		},
	}

	result := GenerateDatesWithEvents(dates, events, today)

	if len(result) != 3 {
		t.Fatalf("len(result) = %d, want 3", len(result))
	}

	// Yesterday should be past
	if !result[0].IsPast {
		t.Error("yesterday should be past")
	}
	if result[0].IsToday {
		t.Error("yesterday should not be today")
	}
	if result[0].Events != 0 {
		t.Errorf("yesterday events = %d, want 0", result[0].Events)
	}

	// Today should be today with 2 events
	if result[1].IsPast {
		t.Error("today should not be past")
	}
	if !result[1].IsToday {
		t.Error("today should be today")
	}
	if result[1].Events != 2 {
		t.Errorf("today events = %d, want 2", result[1].Events)
	}

	// Tomorrow should be future
	if result[2].IsPast {
		t.Error("tomorrow should not be past")
	}
	if result[2].IsToday {
		t.Error("tomorrow should not be today")
	}
}

func TestDrawWeatherForecast(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		forecasts []models.HourlyForecast
		wantEmpty bool
	}{
		{
			name:      "empty forecasts",
			forecasts: []models.HourlyForecast{},
			wantEmpty: true,
		},
		{
			name: "single forecast",
			forecasts: []models.HourlyForecast{
				{Date: now, Temp: 72},
			},
			wantEmpty: false,
		},
		{
			name: "multiple forecasts",
			forecasts: []models.HourlyForecast{
				{Date: now, Temp: 70},
				{Date: now.Add(1 * time.Hour), Temp: 72},
				{Date: now.Add(2 * time.Hour), Temp: 75},
				{Date: now.Add(3 * time.Hour), Temp: 73},
			},
			wantEmpty: false,
		},
		{
			name: "same temperature",
			forecasts: []models.HourlyForecast{
				{Date: now, Temp: 70},
				{Date: now.Add(1 * time.Hour), Temp: 70},
			},
			wantEmpty: false,
		},
		{
			name: "more than 12 forecasts truncated",
			forecasts: func() []models.HourlyForecast {
				f := make([]models.HourlyForecast, 20)
				for i := range f {
					f[i] = models.HourlyForecast{
						Date: now.Add(time.Duration(i) * time.Hour),
						Temp: 70 + i,
					}
				}
				return f
			}(),
			wantEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DrawWeatherForecast(tt.forecasts, 400, 40, 5)

			if tt.wantEmpty {
				if result != "" {
					t.Errorf("expected empty result, got %q", result)
				}
				return
			}

			if result == "" {
				t.Error("expected non-empty result")
				return
			}

			// Should be a data URL
			if !strings.HasPrefix(string(result), "url('data:image/svg+xml;base64,") {
				t.Errorf("result should be base64 data URL, got %q", result)
			}
		})
	}
}

func TestSortedEventDates(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")

	date1 := time.Date(2024, 1, 15, 0, 0, 0, 0, loc)
	date2 := time.Date(2024, 1, 16, 0, 0, 0, 0, loc)
	date3 := time.Date(2024, 1, 14, 0, 0, 0, 0, loc)

	events := map[time.Time][]models.Event{
		date1: {{Name: "Event 1"}},
		date2: {{Name: "Event 2"}},
		date3: {{Name: "Event 3"}},
	}

	result := SortedEventDates(events)

	if len(result) != 3 {
		t.Fatalf("len(result) = %d, want 3", len(result))
	}

	// Should be sorted chronologically
	if !result[0].Equal(date3) {
		t.Errorf("first date = %v, want %v", result[0], date3)
	}
	if !result[1].Equal(date1) {
		t.Errorf("second date = %v, want %v", result[1], date1)
	}
	if !result[2].Equal(date2) {
		t.Errorf("third date = %v, want %v", result[2], date2)
	}
}

func TestSortedEventDatesEmpty(t *testing.T) {
	events := map[time.Time][]models.Event{}

	result := SortedEventDates(events)

	if len(result) != 0 {
		t.Errorf("len(result) = %d, want 0", len(result))
	}
}

func TestSortedEventDatesSkipsEmptyEvents(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")

	date1 := time.Date(2024, 1, 15, 0, 0, 0, 0, loc)
	date2 := time.Date(2024, 1, 16, 0, 0, 0, 0, loc)

	events := map[time.Time][]models.Event{
		date1: {{Name: "Event 1"}},
		date2: {}, // Empty events
	}

	result := SortedEventDates(events)

	if len(result) != 1 {
		t.Errorf("len(result) = %d, want 1 (empty events should be skipped)", len(result))
	}
}

func TestHTML(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	now := time.Now().In(loc)

	weather := &models.Weather{
		Temperature:  72,
		HighTemp:     80,
		LowTemp:      65,
		Condition:    "Clear",
		WeatherClass: "wi wi-day-sunny",
		Forecasts: []models.Forecast{
			{
				Date:         now.Add(24 * time.Hour),
				HighTemp:     78,
				LowTemp:      62,
				Condition:    "Cloudy",
				WeatherClass: "wi wi-cloudy",
			},
		},
		Hourly: []models.HourlyForecast{
			{Date: now, Temp: 72, Condition: "Clear", WeatherClass: "wi wi-day-sunny"},
		},
	}

	data := &TemplateData{
		Weather:         weather,
		Hourly:          weather.Hourly,
		DatesWithEvents: []models.DateCount{},
		Events:          map[time.Time][]models.Event{},
		SortedDates:     []time.Time{},
		HourlySVG:       "url('test')",
		WeatherIconsCSS: GetWeatherIconsCSS(),
	}

	html, css, err := HTML(data)
	if err != nil {
		t.Fatalf("HTML error: %v", err)
	}

	if html == "" {
		t.Error("html should not be empty")
	}

	if css == "" {
		t.Error("css should not be empty")
	}

	// Check that template values are rendered
	if !strings.Contains(html, "72") {
		t.Error("html should contain temperature")
	}

	if !strings.Contains(html, "Clear") {
		t.Error("html should contain condition")
	}
}

func TestHTMLWithEvents(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	now := time.Now().In(loc)
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)

	weather := &models.Weather{
		Temperature:  72,
		HighTemp:     80,
		LowTemp:      65,
		Condition:    "Clear",
		WeatherClass: "wi wi-day-sunny",
		Forecasts: []models.Forecast{
			{
				Date:         now.Add(24 * time.Hour),
				HighTemp:     78,
				LowTemp:      62,
				Condition:    "Cloudy",
				WeatherClass: "wi wi-cloudy",
			},
		},
		Hourly: []models.HourlyForecast{
			{Date: now, Temp: 72, Condition: "Clear", WeatherClass: "wi wi-day-sunny"},
			{Date: now.Add(2 * time.Hour), Temp: 75, Condition: "Clear", WeatherClass: "wi wi-day-sunny"},
		},
	}

	events := map[time.Time][]models.Event{
		todayStart: {
			{
				Name:  "Test Event",
				Start: now,
				End:   now.Add(1 * time.Hour),
			},
		},
	}

	dates, _ := GetCalendarDates(now, 4)
	datesWithEvents := GenerateDatesWithEvents(dates, events, now)

	data := &TemplateData{
		Weather:         weather,
		Hourly:          weather.Hourly[:1], // Just first hourly
		DatesWithEvents: datesWithEvents,
		Events:          events,
		SortedDates:     SortedEventDates(events),
		HourlySVG:       DrawWeatherForecast(weather.Hourly, 400, 40, 5),
		WeatherIconsCSS: GetWeatherIconsCSS(),
	}

	html, css, err := HTML(data)
	if err != nil {
		t.Fatalf("HTML error: %v", err)
	}

	// Check HTML contains expected elements
	if !strings.Contains(html, "Test Event") {
		t.Error("html should contain event name")
	}

	if !strings.Contains(html, "weather") {
		t.Error("html should contain weather section")
	}

	if !strings.Contains(html, "calendar") {
		t.Error("html should contain calendar section")
	}

	// Check CSS is returned
	if !strings.Contains(css, ".weather") {
		t.Error("css should contain weather styles")
	}
}

func TestHTMLFormattingFunctions(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")

	// Test with specific time to verify formatting
	specificTime := time.Date(2024, 1, 15, 14, 30, 0, 0, loc) // 2:30 PM on Monday Jan 15

	weather := &models.Weather{
		Temperature:  72,
		HighTemp:     80,
		LowTemp:      65,
		Condition:    "Clear",
		WeatherClass: "wi wi-day-sunny",
		Forecasts:    []models.Forecast{},
		Hourly: []models.HourlyForecast{
			{Date: specificTime, Temp: 72, Condition: "Clear", WeatherClass: "wi wi-day-sunny"},
		},
	}

	todayStart := time.Date(2024, 1, 15, 0, 0, 0, 0, loc)
	events := map[time.Time][]models.Event{
		todayStart: {
			{
				Name:  "Meeting",
				Start: specificTime,
				End:   specificTime.Add(1 * time.Hour),
			},
		},
	}

	data := &TemplateData{
		Weather: weather,
		Hourly:  weather.Hourly,
		DatesWithEvents: []models.DateCount{
			{Day: specificTime, Events: 1, IsToday: true, IsPast: false},
		},
		Events:          events,
		SortedDates:     []time.Time{todayStart},
		HourlySVG:       "",
		WeatherIconsCSS: GetWeatherIconsCSS(),
	}

	html, _, err := HTML(data)
	if err != nil {
		t.Fatalf("HTML error: %v", err)
	}

	// Check time formatting (02:30 PM)
	if !strings.Contains(html, "02:30 PM") {
		t.Error("html should contain formatted time")
	}

	// Check day formatting (Mon)
	if !strings.Contains(html, "Mon") {
		t.Error("html should contain day abbreviation")
	}

	// Check weekday formatting (Monday)
	if !strings.Contains(html, "Monday") {
		t.Error("html should contain full weekday name")
	}

	// Check date formatting (January 15)
	if !strings.Contains(html, "January 15") {
		t.Error("html should contain formatted date")
	}
}

func TestDrawWeatherForecastEdgeCases(t *testing.T) {
	now := time.Now()

	t.Run("same time different temps", func(t *testing.T) {
		// This tests the timeRange == 0 edge case
		forecasts := []models.HourlyForecast{
			{Date: now, Temp: 70},
			{Date: now, Temp: 75}, // Same time!
		}

		result := DrawWeatherForecast(forecasts, 400, 40, 5)
		if result == "" {
			t.Error("expected non-empty result")
		}
	})
}

func TestHTMLTemplateFunctions(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")

	// Test hour format with leading zero removal
	t.Run("hour format removes leading zero", func(t *testing.T) {
		// 9 AM should show as "9AM" not "09AM"
		nineAM := time.Date(2024, 1, 15, 9, 0, 0, 0, loc)

		weather := &models.Weather{
			Temperature:  72,
			HighTemp:     80,
			LowTemp:      65,
			Condition:    "Clear",
			WeatherClass: "wi wi-day-sunny",
			Forecasts:    []models.Forecast{},
			Hourly: []models.HourlyForecast{
				{Date: nineAM, Temp: 72, Condition: "Clear", WeatherClass: "wi wi-day-sunny"},
			},
		}

		data := &TemplateData{
			Weather:         weather,
			Hourly:          weather.Hourly,
			DatesWithEvents: []models.DateCount{},
			Events:          map[time.Time][]models.Event{},
			SortedDates:     []time.Time{},
			HourlySVG:       "",
			WeatherIconsCSS: GetWeatherIconsCSS(),
		}

		html, _, err := HTML(data)
		if err != nil {
			t.Fatalf("HTML error: %v", err)
		}

		// Check that hour is formatted correctly (9AM not 09AM)
		if !strings.Contains(html, "9AM") {
			t.Error("html should contain '9AM' (without leading zero)")
		}
	})

	t.Run("hour format with double digit hour", func(t *testing.T) {
		// 11 AM should show as "11AM"
		elevenAM := time.Date(2024, 1, 15, 11, 0, 0, 0, loc)

		weather := &models.Weather{
			Temperature:  72,
			HighTemp:     80,
			LowTemp:      65,
			Condition:    "Clear",
			WeatherClass: "wi wi-day-sunny",
			Forecasts:    []models.Forecast{},
			Hourly: []models.HourlyForecast{
				{Date: elevenAM, Temp: 72, Condition: "Clear", WeatherClass: "wi wi-day-sunny"},
			},
		}

		data := &TemplateData{
			Weather:         weather,
			Hourly:          weather.Hourly,
			DatesWithEvents: []models.DateCount{},
			Events:          map[time.Time][]models.Event{},
			SortedDates:     []time.Time{},
			HourlySVG:       "",
			WeatherIconsCSS: GetWeatherIconsCSS(),
		}

		html, _, err := HTML(data)
		if err != nil {
			t.Fatalf("HTML error: %v", err)
		}

		// 11AM already doesn't have leading zero
		if !strings.Contains(html, "11AM") {
			t.Error("html should contain '11AM'")
		}
	})
}

func TestFilterPastEvents(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	now := time.Date(2024, 1, 15, 14, 0, 0, 0, loc) // 2 PM on Jan 15

	date1 := time.Date(2024, 1, 15, 0, 0, 0, 0, loc) // Today
	date2 := time.Date(2024, 1, 16, 0, 0, 0, 0, loc) // Tomorrow
	date3 := time.Date(2024, 1, 14, 0, 0, 0, 0, loc) // Yesterday

	tests := []struct {
		name       string
		events     map[time.Time][]models.Event
		now        time.Time
		wantDates  []time.Time
		wantCounts map[time.Time]int
	}{
		{
			name: "filters out ended events",
			events: map[time.Time][]models.Event{
				date1: {
					{Name: "Past Event", Start: now.Add(-2 * time.Hour), End: now.Add(-1 * time.Hour)}, // Ended 1 hour ago
					{Name: "Future Event", Start: now.Add(1 * time.Hour), End: now.Add(2 * time.Hour)}, // Starts in 1 hour
				},
			},
			now:       now,
			wantDates: []time.Time{date1},
			wantCounts: map[time.Time]int{
				date1: 1, // Only future event
			},
		},
		{
			name: "keeps ongoing events",
			events: map[time.Time][]models.Event{
				date1: {
					{Name: "Ongoing Event", Start: now.Add(-30 * time.Minute), End: now.Add(30 * time.Minute)}, // Still going
				},
			},
			now:       now,
			wantDates: []time.Time{date1},
			wantCounts: map[time.Time]int{
				date1: 1,
			},
		},
		{
			name: "removes dates with only past events",
			events: map[time.Time][]models.Event{
				date3: {
					{Name: "Yesterday Event", Start: now.Add(-25 * time.Hour), End: now.Add(-24 * time.Hour)},
				},
				date1: {
					{Name: "Future Event", Start: now.Add(1 * time.Hour), End: now.Add(2 * time.Hour)},
				},
			},
			now:       now,
			wantDates: []time.Time{date1},
			wantCounts: map[time.Time]int{
				date1: 1,
			},
		},
		{
			name: "handles all-day events correctly",
			events: map[time.Time][]models.Event{
				date1: {
					// All-day event ending at midnight (start of next day)
					{Name: "All Day Today", Start: date1, End: date2, AllDay: true},
				},
			},
			now:       now,
			wantDates: []time.Time{date1},
			wantCounts: map[time.Time]int{
				date1: 1, // Should keep it since it ends at midnight (future)
			},
		},
		{
			name: "handles multiple dates with mixed events",
			events: map[time.Time][]models.Event{
				date1: {
					{Name: "Past 1", Start: now.Add(-2 * time.Hour), End: now.Add(-1 * time.Hour)},
					{Name: "Future 1", Start: now.Add(1 * time.Hour), End: now.Add(2 * time.Hour)},
					{Name: "Future 2", Start: now.Add(3 * time.Hour), End: now.Add(4 * time.Hour)},
				},
				date2: {
					{Name: "Tomorrow", Start: now.Add(24 * time.Hour), End: now.Add(25 * time.Hour)},
				},
			},
			now:       now,
			wantDates: []time.Time{date1, date2},
			wantCounts: map[time.Time]int{
				date1: 2, // 2 future events
				date2: 1,
			},
		},
		{
			name:       "empty events",
			events:     map[time.Time][]models.Event{},
			now:        now,
			wantDates:  []time.Time{},
			wantCounts: map[time.Time]int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterPastEvents(tt.events, tt.now)

			// Check number of dates
			if len(result) != len(tt.wantDates) {
				t.Errorf("FilterPastEvents() returned %d dates, want %d", len(result), len(tt.wantDates))
			}

			// Check event counts for each date
			for date, wantCount := range tt.wantCounts {
				gotEvents, exists := result[date]
				if !exists {
					t.Errorf("FilterPastEvents() missing date %v", date)
					continue
				}
				if len(gotEvents) != wantCount {
					t.Errorf("FilterPastEvents() date %v has %d events, want %d", date, len(gotEvents), wantCount)
				}
			}

			// Verify no past events remain
			for date, events := range result {
				for _, event := range events {
					if !event.End.After(tt.now) {
						t.Errorf("FilterPastEvents() kept past event %q on %v (ended at %v, now is %v)",
							event.Name, date, event.End, tt.now)
					}
				}
			}
		})
	}
}

func TestBase64Encode(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
		want  string
	}{
		{
			name:  "empty",
			input: []byte{},
			want:  "",
		},
		{
			name:  "hello",
			input: []byte("hello"),
			want:  "aGVsbG8=",
		},
		{
			name:  "hello world",
			input: []byte("hello world"),
			want:  "aGVsbG8gd29ybGQ=",
		},
		{
			name:  "single byte",
			input: []byte("a"),
			want:  "YQ==",
		},
		{
			name:  "two bytes",
			input: []byte("ab"),
			want:  "YWI=",
		},
		{
			name:  "three bytes",
			input: []byte("abc"),
			want:  "YWJj",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := base64Encode(tt.input)
			if got != tt.want {
				t.Errorf("base64Encode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCSSLayoutContainsFlexBody(t *testing.T) {
	_, css, err := HTML(&TemplateData{
		Weather: &models.Weather{
			Forecasts: []models.Forecast{},
			Hourly:    []models.HourlyForecast{},
		},
		DatesWithEvents: []models.DateCount{},
		Events:          map[time.Time][]models.Event{},
		SortedDates:     []time.Time{},
		HourlySVG:       "",
		WeatherIconsCSS: GetWeatherIconsCSS(),
	})
	if err != nil {
		t.Fatalf("HTML() error: %v", err)
	}

	// body must be a flex column so sections stack vertically within the viewport
	if !strings.Contains(css, "display: flex") {
		t.Error("css body should use display: flex")
	}
	if !strings.Contains(css, "flex-direction: column") {
		t.Error("css body should use flex-direction: column")
	}

	// body height must be 100vh (viewport height), not 100vw (viewport width)
	if strings.Contains(css, "height: 100vw") {
		t.Error("css body should not use height: 100vw; should use height: 100vh")
	}
	if !strings.Contains(css, "height: 100vh") {
		t.Error("css body should use height: 100vh")
	}

	// overflow must be hidden (not the invalid 'none')
	if strings.Contains(css, "overflow: none") {
		t.Error("css body should not use overflow: none (invalid value)")
	}
	if !strings.Contains(css, "overflow: hidden") {
		t.Error("css body should use overflow: hidden")
	}

	// events section must not have a fixed vw-based height
	if strings.Contains(css, "height: 70vw") {
		t.Error("css .events should not use fixed height: 70vw")
	}

	// events section must use flex: 1 to fill remaining space
	if !strings.Contains(css, "flex: 1") {
		t.Error("css .events should use flex: 1")
	}
}

func TestWeatherIconsCSSEmbedded(t *testing.T) {
	css := GetWeatherIconsCSS()
	cssStr := string(css)

	// Test that CSS is not empty
	if cssStr == "" {
		t.Error("GetWeatherIconsCSS() returned empty string")
	}

	// Test that it contains font-face declarations
	if !strings.Contains(cssStr, "@font-face") {
		t.Error("CSS should contain @font-face declarations")
	}

	// Test that it references weathericons font
	if !strings.Contains(cssStr, "weathericons") {
		t.Error("CSS should reference weathericons font")
	}

	// Test that it contains weather icon classes
	if !strings.Contains(cssStr, ".wi-") {
		t.Error("CSS should contain weather icon style rules (.wi-)")
	}

	// Test that it contains embedded font data (base64 or data URLs)
	if !strings.Contains(cssStr, "url(") {
		t.Error("CSS should contain url() references for fonts")
	}

	// Test that CSS is substantial (embedded fonts make it large)
	if len(cssStr) < 50000 {
		t.Errorf("CSS length = %d bytes, expected at least 50000 (should include embedded fonts)", len(cssStr))
	}

	t.Logf("Weather Icons CSS size: %d bytes", len(cssStr))
}
