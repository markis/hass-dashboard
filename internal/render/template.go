package render

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"sort"
	"time"

	"github.com/markis/hass-dashboard/internal/models"
)

//go:embed templates/dashboard.html
var dashboardTemplate string

//go:embed templates/style.css
var dashboardCSS string

//go:embed templates/weather-icons-embedded.css
var weatherIconsCSS string

// TemplateData holds all data needed to render the dashboard.
type TemplateData struct {
	Weather         *models.Weather
	Hourly          []models.HourlyForecast
	DatesWithEvents []models.DateCount
	Events          map[time.Time][]models.Event
	SortedDates     []time.Time
	HourlySVG       template.CSS
	WeatherIconsCSS template.CSS
}

// GetWeatherIconsCSS returns the embedded Weather Icons CSS.
func GetWeatherIconsCSS() template.CSS {
	// #nosec G203 -- CSS is embedded at compile time from trusted source, not user input
	return template.CSS(weatherIconsCSS)
}

// HTML generates the dashboard HTML from template data.
func HTML(data *TemplateData) (string, string, error) {
	funcMap := template.FuncMap{
		"formatHour": func(t time.Time) string {
			hour := t.Format("3PM")
			// Remove leading zero from hour
			if hour[0] == '0' {
				return hour[1:]
			}

			return hour
		},
		"formatDay": func(t time.Time) string {
			return t.Format("Mon")
		},
		"formatDayNumber": func(t time.Time) string {
			return t.Format("02")
		},
		"formatWeekday": func(t time.Time) string {
			return t.Format("Monday")
		},
		"formatFullDate": func(t time.Time) string {
			return t.Format("January 02")
		},
		"formatTime": func(t time.Time) string {
			return t.Format("03:04 PM")
		},
	}

	tmpl, err := template.New("dashboard").Funcs(funcMap).Parse(dashboardTemplate)
	if err != nil {
		return "", "", fmt.Errorf("parsing template: %w", err)
	}

	var buf bytes.Buffer

	if err := tmpl.Execute(&buf, data); err != nil {
		return "", "", fmt.Errorf("executing template: %w", err)
	}

	return buf.String(), dashboardCSS, nil
}

// GetCalendarDates generates a range of dates starting from the beginning of the current week.
func GetCalendarDates(currentDate time.Time, weeks int) ([]time.Time, time.Time) {
	// Get start of week (Sunday)
	weekday := int(currentDate.Weekday())
	start := currentDate.AddDate(0, 0, -weekday)
	start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, currentDate.Location())

	end := start.AddDate(0, 0, weeks*7)

	dates := make([]time.Time, 0, weeks*7)

	for d := start; d.Before(end); d = d.AddDate(0, 0, 1) {
		dates = append(dates, d)
	}

	return dates, end
}

// GenerateDatesWithEvents creates DateCount objects for each date.
func GenerateDatesWithEvents(
	dates []time.Time,
	events map[time.Time][]models.Event,
	today time.Time,
) []models.DateCount {
	todayDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, today.Location())

	result := make([]models.DateCount, 0, len(dates))

	for _, dt := range dates {
		dateKey := time.Date(dt.Year(), dt.Month(), dt.Day(), 0, 0, 0, 0, dt.Location())
		eventCount := len(events[dateKey])
		result = append(result, models.DateCount{
			Day:     dt,
			Events:  eventCount,
			IsPast:  dt.Before(todayDate),
			IsToday: dt.Equal(todayDate),
		})
	}

	return result
}

// DrawWeatherForecast creates an SVG line graph of hourly temperatures.
//
//nolint:gosec // G203: SVG is generated from trusted internal data, not user input
func DrawWeatherForecast(forecasts []models.HourlyForecast, width, height, padding int) template.CSS {
	if len(forecasts) == 0 {
		return ""
	}

	// Limit to first 12 hours
	if len(forecasts) > 12 {
		forecasts = forecasts[:12]
	}

	// Find min/max temps
	minTemp := forecasts[0].Temp
	maxTemp := forecasts[0].Temp

	for _, forecast := range forecasts {
		if forecast.Temp < minTemp {
			minTemp = forecast.Temp
		}

		if forecast.Temp > maxTemp {
			maxTemp = forecast.Temp
		}
	}

	// Avoid division by zero
	tempRange := maxTemp - minTemp
	if tempRange == 0 {
		tempRange = 1
	}

	// Get time range
	startTime := forecasts[0].Date
	endTime := forecasts[len(forecasts)-1].Date

	timeRange := endTime.Sub(startTime).Seconds()
	if timeRange == 0 {
		timeRange = 1
	}

	// Calculate scaling factors
	xScale := float64(width-padding*2) / timeRange
	yScale := float64(height-padding*2) / float64(tempRange)

	// Build path
	var pathData bytes.Buffer

	for idx, forecast := range forecasts {
		xCoord := float64(padding) + forecast.Date.Sub(startTime).Seconds()*xScale
		yCoord := float64(height-padding) - float64(forecast.Temp-minTemp)*yScale

		if idx == 0 {
			fmt.Fprintf(&pathData, "M%.1f %.1f", xCoord, yCoord)
		} else {
			fmt.Fprintf(&pathData, " L%.1f %.1f", xCoord, yCoord)
		}
	}

	svg := fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d">`+
			`<path d="%s" stroke="#CCC" stroke-width="5" fill="none"/></svg>`,
		width, height, pathData.String(),
	)

	// Base64 encode
	encoded := template.CSS(fmt.Sprintf(
		"url('data:image/svg+xml;base64,%s')",
		base64Encode([]byte(svg)),
	))

	return encoded
}

func base64Encode(data []byte) string {
	const base64Chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

	var result bytes.Buffer

	for i := 0; i < len(data); i += 3 {
		remaining := len(data) - i

		var bits uint32

		switch {
		case remaining >= 3:
			bits = uint32(data[i])<<16 | uint32(data[i+1])<<8 | uint32(data[i+2])
			result.WriteByte(base64Chars[bits>>18&0x3F])
			result.WriteByte(base64Chars[bits>>12&0x3F])
			result.WriteByte(base64Chars[bits>>6&0x3F])
			result.WriteByte(base64Chars[bits&0x3F])
		case remaining == 2:
			bits = uint32(data[i])<<16 | uint32(data[i+1])<<8
			result.WriteByte(base64Chars[bits>>18&0x3F])
			result.WriteByte(base64Chars[bits>>12&0x3F])
			result.WriteByte(base64Chars[bits>>6&0x3F])
			result.WriteByte('=')
		default:
			bits = uint32(data[i]) << 16
			result.WriteByte(base64Chars[bits>>18&0x3F])
			result.WriteByte(base64Chars[bits>>12&0x3F])
			result.WriteString("==")
		}
	}

	return result.String()
}

// HourlyForecastView is an alias for models.HourlyForecast for template rendering.
type HourlyForecastView = models.HourlyForecast

// FilterPastEvents removes events that have already ended.
// Returns a new map with only active events (events whose end time is in the future).
func FilterPastEvents(events map[time.Time][]models.Event, now time.Time) map[time.Time][]models.Event {
	filtered := make(map[time.Time][]models.Event)

	for date, eventsOnDate := range events {
		activeEvents := make([]models.Event, 0)

		for _, event := range eventsOnDate {
			// Only include events that haven't ended yet
			if event.End.After(now) {
				activeEvents = append(activeEvents, event)
			}
		}

		// Only include the date if there are active events
		if len(activeEvents) > 0 {
			filtered[date] = activeEvents
		}
	}

	return filtered
}

// SortedEventDates returns event dates sorted chronologically.
// It expects the events map to already be filtered by FilterPastEvents.
func SortedEventDates(events map[time.Time][]models.Event) []time.Time {
	dates := make([]time.Time, 0, len(events))

	for date := range events {
		if len(events[date]) > 0 {
			dates = append(dates, date)
		}
	}

	sort.Slice(dates, func(i, j int) bool {
		return dates[i].Before(dates[j])
	})

	return dates
}
