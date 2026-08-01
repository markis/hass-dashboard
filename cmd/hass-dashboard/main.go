package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/markis/hass-dashboard/internal/clients"
	"github.com/markis/hass-dashboard/internal/models"
	"github.com/markis/hass-dashboard/internal/render"
)

// Config holds application configuration loaded from YAML file.
type Config struct {
	Google          GoogleConfig      `yaml:"google"`
	OpenWeatherMap  OpenWeatherConfig `yaml:"openweathermap"`
	Output          OutputConfig      `yaml:"output"`
	Timezone        string            `yaml:"timezone"`
	RefreshInterval int               `yaml:"refresh_interval"` // seconds
}

// GoogleConfig holds Google Calendar configuration. Calendars are read via a
// service account with Workspace domain-wide delegation.
type GoogleConfig struct {
	CredentialsFile string   `yaml:"credentials_file"`
	Impersonate     string   `yaml:"impersonate"`
	Calendars       []string `yaml:"calendars"`
}

// OpenWeatherConfig holds OpenWeatherMap configuration.
type OpenWeatherConfig struct {
	Key       string  `yaml:"api_key"`
	Latitude  float64 `yaml:"latitude"`
	Longitude float64 `yaml:"longitude"`
}

// OutputConfig holds output rendering configuration.
type OutputConfig struct {
	Path   string `yaml:"path"`
	Width  int    `yaml:"width"`
	Height int    `yaml:"height"`
	Rotate int    `yaml:"rotate"`
}

func main() {
	configPath := flag.String("config", "config.yaml", "path to configuration file")

	flag.Parse()

	config, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	loc, err := time.LoadLocation(config.Timezone)
	if err != nil {
		log.Fatalf("Invalid timezone %q: %v", config.Timezone, err)
	}

	// Set up context for client construction (service account auth may make a
	// network call to exchange the JWT for an access token).
	ctx, cancel := context.WithCancel(context.Background())

	// Create clients (before signal handling setup to allow fatal errors)
	calendarClient, err := clients.NewCalendarClient(
		ctx,
		config.Google.CredentialsFile,
		config.Google.Impersonate,
		loc,
	)
	if err != nil {
		cancel()

		log.Fatalf("Failed to create calendar client: %v", err)
	}

	defer cancel()

	weatherClient := clients.NewWeatherClient(config.OpenWeatherMap.Key, loc)

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		cancel()
	}()

	log.Println("Starting dashboard generator...")

	// Generate immediately, then on interval
	refreshInterval := time.Duration(config.RefreshInterval) * time.Second
	ticker := time.NewTicker(refreshInterval)

	defer ticker.Stop()

	for {
		if err := generateDashboard(ctx, config, calendarClient, weatherClient, loc); err != nil {
			log.Printf("Error generating dashboard: %v", err)
		} else {
			log.Printf("Dashboard generated successfully at %s", config.Output.Path)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Continue to next iteration
		}
	}
}

func generateDashboard(
	ctx context.Context,
	config *Config,
	calendarClient *clients.CalendarClient,
	weatherClient *clients.WeatherClient,
	loc *time.Location,
) error {
	now := time.Now().In(loc)
	dates, endDate := render.GetCalendarDates(now, 4)
	startDate := dates[0]

	// Fetch data concurrently
	type result struct {
		weather *render.TemplateData
		err     error
	}

	weatherChan := make(chan result, 1)

	go func() {
		weather, err := weatherClient.GetWeather(
			ctx,
			config.OpenWeatherMap.Latitude,
			config.OpenWeatherMap.Longitude,
		)
		if err != nil {
			weatherChan <- result{err: err}

			return
		}

		weatherChan <- result{weather: &render.TemplateData{Weather: weather}}
	}()

	// Fetch calendar events
	allEvents, err := calendarClient.GetCalendars(ctx, config.Google.Calendars, startDate, endDate)
	if err != nil {
		return err
	}

	// Filter out events that have already ended
	activeEvents := render.FilterPastEvents(allEvents, now)

	// Deduplicate events on the same day with the same time and title
	activeEvents = render.DeduplicateEvents(activeEvents)

	// Wait for weather
	weatherResult := <-weatherChan
	if weatherResult.err != nil {
		return weatherResult.err
	}

	weather := weatherResult.weather.Weather

	// Prepare hourly data (skip first, take every other up to 6)
	hourly := make([]models.HourlyForecast, 0, 6)

	for i := 1; i < len(weather.Hourly) && len(hourly) < 6; i += 2 {
		hourly = append(hourly, weather.Hourly[i])
	}

	// Generate dates with event counts (using active events only)
	datesWithEvents := render.GenerateDatesWithEvents(dates, activeEvents, now)

	// Draw hourly forecast SVG
	hourlySVG := render.DrawWeatherForecast(weather.Hourly, 400, 40, 5)

	// Prepare template data
	templateData := &render.TemplateData{
		Weather:         weather,
		Hourly:          hourly,
		DatesWithEvents: datesWithEvents,
		Events:          activeEvents,
		SortedDates:     render.SortedEventDates(activeEvents),
		HourlySVG:       hourlySVG,
		WeatherIconsCSS: render.GetWeatherIconsCSS(),
	}

	// Render HTML (hash is computed from HTML for change detection)
	html, css, hash, err := render.HTML(templateData)
	if err != nil {
		return err
	}

	// Render to image with HTML hash for change detection
	imageConfig := &render.ImageConfig{
		Width:      config.Output.Width,
		Height:     config.Output.Height,
		Rotate:     config.Output.Rotate,
		OutputPath: config.Output.Path,
		DataHash:   hash,
	}

	return render.Image(ctx, html, css, imageConfig)
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // Config path is from command-line flag
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	config := &Config{
		// Set defaults
		Timezone:        "America/New_York",
		RefreshInterval: 600,
		Output: OutputConfig{
			Path:   "output.jpg",
			Width:  820,
			Height: 1200,
			Rotate: 270,
		},
	}

	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	return config, nil
}
