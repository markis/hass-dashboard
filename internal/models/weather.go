package models

import "time"

// OneCallWeatherData represents the full response from OpenWeatherMap One Call API.
type OneCallWeatherData struct {
	Lat            float64             `json:"lat"`
	Lon            float64             `json:"lon"`
	Timezone       string              `json:"timezone"`
	TimezoneOffset int                 `json:"timezone_offset"`
	Current        OneCallCurrentData  `json:"current"`
	Hourly         []OneCallHourlyData `json:"hourly"`
	Daily          []OneCallDailyData  `json:"daily"`
}

// OneCallCurrentData represents current weather conditions.
type OneCallCurrentData struct {
	Dt         int64                `json:"dt"`
	Sunrise    int64                `json:"sunrise"`
	Sunset     int64                `json:"sunset"`
	Temp       float64              `json:"temp"`
	FeelsLike  float64              `json:"feels_like"`
	Pressure   int                  `json:"pressure"`
	Humidity   int                  `json:"humidity"`
	DewPoint   float64              `json:"dew_point"`
	UVI        float64              `json:"uvi"`
	Clouds     int                  `json:"clouds"`
	Visibility int                  `json:"visibility"`
	WindSpeed  float64              `json:"wind_speed"`
	WindDeg    int                  `json:"wind_deg"`
	Weather    []OneCallWeatherInfo `json:"weather"`
}

// OneCallHourlyData represents hourly forecast data.
type OneCallHourlyData struct {
	Dt         int64                `json:"dt"`
	Temp       float64              `json:"temp"`
	FeelsLike  float64              `json:"feels_like"`
	Pressure   int                  `json:"pressure"`
	Humidity   int                  `json:"humidity"`
	DewPoint   float64              `json:"dew_point"`
	UVI        float64              `json:"uvi"`
	Clouds     int                  `json:"clouds"`
	Visibility int                  `json:"visibility"`
	WindSpeed  float64              `json:"wind_speed"`
	WindDeg    int                  `json:"wind_deg"`
	WindGust   float64              `json:"wind_gust"`
	Weather    []OneCallWeatherInfo `json:"weather"`
	Pop        float64              `json:"pop"`
}

// OneCallDailyData represents daily forecast data.
type OneCallDailyData struct {
	Dt        int64                `json:"dt"`
	Sunrise   int64                `json:"sunrise"`
	Sunset    int64                `json:"sunset"`
	Moonrise  int64                `json:"moonrise"`
	Moonset   int64                `json:"moonset"`
	MoonPhase float64              `json:"moon_phase"`
	Temp      OneCallTempData      `json:"temp"`
	FeelsLike OneCallFeelsLikeData `json:"feels_like"`
	Pressure  int                  `json:"pressure"`
	Humidity  int                  `json:"humidity"`
	DewPoint  float64              `json:"dew_point"`
	WindSpeed float64              `json:"wind_speed"`
	WindDeg   int                  `json:"wind_deg"`
	WindGust  float64              `json:"wind_gust"`
	Weather   []OneCallWeatherInfo `json:"weather"`
	Clouds    int                  `json:"clouds"`
	Pop       float64              `json:"pop"`
	Rain      float64              `json:"rain"`
	UVI       float64              `json:"uvi"`
}

// OneCallWeatherInfo represents weather condition info.
type OneCallWeatherInfo struct {
	ID          int    `json:"id"`
	Main        string `json:"main"`
	Description string `json:"description"`
	Icon        string `json:"icon"`
}

// OneCallTempData represents temperature data for a day.
type OneCallTempData struct {
	Day   float64 `json:"day"`
	Min   float64 `json:"min"`
	Max   float64 `json:"max"`
	Night float64 `json:"night"`
	Eve   float64 `json:"eve"`
	Morn  float64 `json:"morn"`
}

// OneCallFeelsLikeData represents "feels like" temperature data.
type OneCallFeelsLikeData struct {
	Day   float64 `json:"day"`
	Night float64 `json:"night"`
	Eve   float64 `json:"eve"`
	Morn  float64 `json:"morn"`
}

// Weather represents processed weather data for the dashboard.
type Weather struct {
	Temperature  int
	HighTemp     int
	LowTemp      int
	Condition    string
	WeatherClass string
	Forecasts    []Forecast
	Hourly       []HourlyForecast
}

// Forecast represents a daily forecast.
type Forecast struct {
	Date         time.Time
	HighTemp     int
	LowTemp      int
	Condition    string
	WeatherClass string
}

// HourlyForecast represents an hourly forecast.
type HourlyForecast struct {
	Date         time.Time
	Temp         int
	Condition    string
	WeatherClass string
}

// Weather code ranges for classification.
const (
	Thunderstorm = 200
	Drizzle      = 300
	Rain         = 500
	Snow         = 600
)

// WeatherToIconName converts a weather code to CSS class and condition name.
//
//nolint:gocyclo,cyclop,nonamedreturns // Weather code mapping requires many cases, named returns for clarity
func WeatherToIconName(weatherCode int) (cssClass, name string) {
	switch {
	case weatherCode >= Thunderstorm && weatherCode < Thunderstorm+100:
		return "wi wi-thunderstorm", "Thunderstorm"
	case weatherCode >= Drizzle && weatherCode < Drizzle+100:
		return "wi wi-sprinkle", "Drizzle"
	case weatherCode >= Rain && weatherCode < Rain+100:
		return "wi wi-rain", "Rain"
	case weatherCode >= Snow && weatherCode < Snow+100:
		return "wi wi-snow", "Snow"
	case weatherCode == 701:
		return "wi wi-fog", "Mist"
	case weatherCode == 711:
		return "wi wi-smoke", "Smoke"
	case weatherCode == 721:
		return "wi wi-day-haze", "Haze"
	case weatherCode == 731 || weatherCode == 761:
		return "wi wi-dust", "Dust"
	case weatherCode == 741:
		return "wi wi-fog", "Fog"
	case weatherCode == 751:
		return "wi wi-sandstorm", "Sand"
	case weatherCode == 762:
		return "wi wi-volcano", "Ash"
	case weatherCode == 771:
		return "wi wi-strong-wind", "Squall"
	case weatherCode == 781:
		return "wi wi-tornado", "Tornado"
	case weatherCode == 800:
		return "wi wi-day-sunny", "Clear"
	case weatherCode == 801:
		return "wi wi-cloud", "Few Clouds"
	case weatherCode == 802 || weatherCode == 803:
		return "wi wi-cloudy", "Partly Cloudy"
	case weatherCode == 804:
		return "wi wi-cloudy", "Overcast"
	default:
		return "wi wi-na", "Unknown"
	}
}

// FromOneCall creates a Weather from OpenWeatherMap One Call API response.
func (w *Weather) FromOneCall(data *OneCallWeatherData, loc *time.Location) {
	weatherID := 0
	if len(data.Current.Weather) > 0 {
		weatherID = data.Current.Weather[0].ID
	}

	w.WeatherClass, w.Condition = WeatherToIconName(weatherID)
	w.Temperature = int(data.Current.Temp)

	// Process daily forecasts
	w.Forecasts = make([]Forecast, 0, len(data.Daily))

	for idx := range data.Daily {
		daily := &data.Daily[idx]
		wID := 0

		if len(daily.Weather) > 0 {
			wID = daily.Weather[0].ID
		}

		cssClass, condition := WeatherToIconName(wID)
		w.Forecasts = append(w.Forecasts, Forecast{
			Date:         time.Unix(daily.Dt, 0).In(loc),
			HighTemp:     int(daily.Temp.Max),
			LowTemp:      int(daily.Temp.Min),
			Condition:    condition,
			WeatherClass: cssClass,
		})
	}

	// Set today's high/low from first forecast and remove it
	if len(w.Forecasts) > 0 {
		w.HighTemp = w.Forecasts[0].HighTemp
		w.LowTemp = w.Forecasts[0].LowTemp
		w.Forecasts = w.Forecasts[1:]
	}

	// Process hourly forecasts
	w.Hourly = make([]HourlyForecast, 0, len(data.Hourly))

	for idx := range data.Hourly {
		hourly := &data.Hourly[idx]
		wID := 0

		if len(hourly.Weather) > 0 {
			wID = hourly.Weather[0].ID
		}

		cssClass, condition := WeatherToIconName(wID)
		w.Hourly = append(w.Hourly, HourlyForecast{
			Date:         time.Unix(hourly.Dt, 0).In(loc),
			Temp:         int(hourly.Temp),
			Condition:    condition,
			WeatherClass: cssClass,
		})
	}
}
