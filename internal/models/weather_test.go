package models

import (
	"testing"
	"time"
)

func TestWeatherToIconName(t *testing.T) {
	tests := []struct {
		name         string
		weatherCode  int
		wantCSSClass string
		wantName     string
	}{
		{"thunderstorm low", 200, "wi wi-thunderstorm", "Thunderstorm"},
		{"thunderstorm high", 299, "wi wi-thunderstorm", "Thunderstorm"},
		{"drizzle low", 300, "wi wi-sprinkle", "Drizzle"},
		{"drizzle high", 399, "wi wi-sprinkle", "Drizzle"},
		{"rain low", 500, "wi wi-rain", "Rain"},
		{"rain high", 599, "wi wi-rain", "Rain"},
		{"snow low", 600, "wi wi-snow", "Snow"},
		{"snow high", 699, "wi wi-snow", "Snow"},
		{"mist", 701, "wi wi-fog", "Mist"},
		{"smoke", 711, "wi wi-smoke", "Smoke"},
		{"haze", 721, "wi wi-day-haze", "Haze"},
		{"dust 731", 731, "wi wi-dust", "Dust"},
		{"dust 761", 761, "wi wi-dust", "Dust"},
		{"fog", 741, "wi wi-fog", "Fog"},
		{"sand", 751, "wi wi-sandstorm", "Sand"},
		{"ash", 762, "wi wi-volcano", "Ash"},
		{"squall", 771, "wi wi-strong-wind", "Squall"},
		{"tornado", 781, "wi wi-tornado", "Tornado"},
		{"clear", 800, "wi wi-day-sunny", "Clear"},
		{"few clouds", 801, "wi wi-cloud", "Few Clouds"},
		{"partly cloudy 802", 802, "wi wi-cloudy", "Partly Cloudy"},
		{"partly cloudy 803", 803, "wi wi-cloudy", "Partly Cloudy"},
		{"overcast", 804, "wi wi-cloudy", "Overcast"},
		{"unknown", 999, "wi wi-na", "Unknown"},
		{"unknown negative", -1, "wi wi-na", "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCSS, gotName := WeatherToIconName(tt.weatherCode)
			if gotCSS != tt.wantCSSClass {
				t.Errorf("WeatherToIconName(%d) cssClass = %q, want %q", tt.weatherCode, gotCSS, tt.wantCSSClass)
			}
			if gotName != tt.wantName {
				t.Errorf("WeatherToIconName(%d) name = %q, want %q", tt.weatherCode, gotName, tt.wantName)
			}
		})
	}
}

func TestWeatherFromOneCall(t *testing.T) {
	loc, _ := time.LoadLocation("America/New_York")
	now := time.Now().Unix()

	data := &OneCallWeatherData{
		Lat:      40.7128,
		Lon:      -74.0060,
		Timezone: "America/New_York",
		Current: OneCallCurrentData{
			Dt:   now,
			Temp: 72.5,
			Weather: []OneCallWeatherInfo{
				{ID: 800, Main: "Clear", Description: "clear sky"},
			},
		},
		Daily: []OneCallDailyData{
			{
				Dt: now,
				Temp: OneCallTempData{
					Max: 80.0,
					Min: 65.0,
				},
				Weather: []OneCallWeatherInfo{
					{ID: 800},
				},
			},
			{
				Dt: now + 86400,
				Temp: OneCallTempData{
					Max: 75.0,
					Min: 60.0,
				},
				Weather: []OneCallWeatherInfo{
					{ID: 500},
				},
			},
		},
		Hourly: []OneCallHourlyData{
			{
				Dt:   now,
				Temp: 72.0,
				Weather: []OneCallWeatherInfo{
					{ID: 800},
				},
			},
			{
				Dt:   now + 3600,
				Temp: 74.0,
				Weather: []OneCallWeatherInfo{
					{ID: 801},
				},
			},
		},
	}

	weather := &Weather{}
	weather.FromOneCall(data, loc)

	if weather.Temperature != 72 {
		t.Errorf("Temperature = %d, want 72", weather.Temperature)
	}

	if weather.HighTemp != 80 {
		t.Errorf("HighTemp = %d, want 80", weather.HighTemp)
	}

	if weather.LowTemp != 65 {
		t.Errorf("LowTemp = %d, want 65", weather.LowTemp)
	}

	if weather.Condition != "Clear" {
		t.Errorf("Condition = %q, want %q", weather.Condition, "Clear")
	}

	if weather.WeatherClass != "wi wi-day-sunny" {
		t.Errorf("WeatherClass = %q, want %q", weather.WeatherClass, "wi wi-day-sunny")
	}

	// First forecast should be removed (today's)
	if len(weather.Forecasts) != 1 {
		t.Errorf("len(Forecasts) = %d, want 1", len(weather.Forecasts))
	}

	if len(weather.Hourly) != 2 {
		t.Errorf("len(Hourly) = %d, want 2", len(weather.Hourly))
	}
}

func TestWeatherFromOneCallEmptyWeather(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	now := time.Now().Unix()

	data := &OneCallWeatherData{
		Current: OneCallCurrentData{
			Dt:      now,
			Temp:    50.0,
			Weather: []OneCallWeatherInfo{}, // Empty weather array
		},
		Daily:  []OneCallDailyData{},
		Hourly: []OneCallHourlyData{},
	}

	weather := &Weather{}
	weather.FromOneCall(data, loc)

	// Should default to unknown
	if weather.Condition != "Unknown" {
		t.Errorf("Condition = %q, want %q", weather.Condition, "Unknown")
	}
}

func TestWeatherFromOneCallNoForecasts(t *testing.T) {
	loc, _ := time.LoadLocation("UTC")
	now := time.Now().Unix()

	data := &OneCallWeatherData{
		Current: OneCallCurrentData{
			Dt:   now,
			Temp: 50.0,
			Weather: []OneCallWeatherInfo{
				{ID: 800},
			},
		},
		Daily:  []OneCallDailyData{},
		Hourly: []OneCallHourlyData{},
	}

	weather := &Weather{}
	weather.FromOneCall(data, loc)

	// Should have zero high/low when no daily data
	if weather.HighTemp != 0 {
		t.Errorf("HighTemp = %d, want 0", weather.HighTemp)
	}

	if weather.LowTemp != 0 {
		t.Errorf("LowTemp = %d, want 0", weather.LowTemp)
	}
}
