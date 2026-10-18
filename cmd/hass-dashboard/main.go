package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otelmetric "go.opentelemetry.io/otel/metric"
	"gopkg.in/yaml.v3"

	"github.com/markis/hass-dashboard/internal/clients"
	"github.com/markis/hass-dashboard/internal/models"
	"github.com/markis/hass-dashboard/internal/render"
	"github.com/markis/hass-dashboard/internal/serve"
	"github.com/markis/hass-dashboard/internal/telemetry"
)

// Default configuration values applied when the config file omits them.
const (
	defaultTimezone        = "America/New_York"
	defaultOutputPath      = "output.jpg"
	defaultRefreshInterval = 600
	defaultOutputWidth     = 820
	defaultOutputHeight    = 1200
	defaultOutputRotate    = 270
	defaultServeListen     = ":8080"
)

// Attribute values used by the generation-loop metrics and spans.
const (
	outcomeSuccess = "success"
	outcomeError   = "error"

	stageHTML     = "html"
	stageImage    = "image"
	stageCalendar = "calendar"
	stageWeather  = "weather"
)

// instrumentationScope is the scope name shared by the main package's tracer
// and meter.
const instrumentationScope = "github.com/markis/hass-dashboard/cmd/hass-dashboard"

// Config holds application configuration loaded from YAML file.
type Config struct {
	Google          GoogleConfig      `yaml:"google"`
	OpenWeatherMap  OpenWeatherConfig `yaml:"openweathermap"`
	Output          OutputConfig      `yaml:"output"`
	Otel            telemetry.Config  `yaml:"otel"`
	Timezone        string            `yaml:"timezone"`
	RefreshInterval int               `yaml:"refresh_interval"` // seconds
	Serve           serve.Config      `yaml:"serve"`
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

// cycleMetrics instruments dashboard generation with a cycle counter and
// cycle/stage duration histograms.
type cycleMetrics struct {
	cycles       otelmetric.Int64Counter
	cycleSeconds otelmetric.Float64Histogram
	stageSeconds otelmetric.Float64Histogram
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

	// Telemetry must be initialized before instrumented components are built.
	shutdownTelemetry, err := telemetry.Init(ctx, config.Otel)
	if err != nil {
		cancel()

		log.Fatalf("Failed to initialize telemetry: %v", err)
	}

	// Create clients (before signal handling setup to allow fatal errors)
	calendarClient, err := clients.NewCalendarClient(
		ctx,
		config.Google.CredentialsFile,
		config.Google.Impersonate,
		loc,
	)
	if err != nil {
		stopTelemetry(shutdownTelemetry)
		cancel()

		log.Fatalf("Failed to create calendar client: %v", err)
	}

	weatherClient := clients.NewWeatherClient(config.OpenWeatherMap.Key, loc)

	metrics, err := newCycleMetrics(otel.Meter(instrumentationScope))
	if err != nil {
		stopTelemetry(shutdownTelemetry)
		cancel()

		log.Fatalf("Failed to create cycle metrics: %v", err)
	}

	// HTTP server is read-only image serving: it starts before the generation
	// loop so the dashboard URL is addressable right away, serves whatever
	// already exists on disk, and 404s until the first render lands.
	var imageServer *serve.Server
	if config.Serve.Enabled {
		imageServer, err = startImageServer(config)
		if err != nil {
			stopTelemetry(shutdownTelemetry)
			cancel()

			log.Fatalf("Failed to create HTTP server: %v", err)
		}
	}

	defer stopTelemetry(shutdownTelemetry)
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down...")
		cancel()
	}()

	log.Println("Starting dashboard generator...")

	// The generation loop runs concurrently with serving: the display polls
	// at its own cadence and never triggers a render.
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)

		runGenerationLoop(ctx, metrics, config, calendarClient, weatherClient, loc)
	}()

	<-ctx.Done()
	log.Println("Shutting down...")

	// Drain serving first so serving spans end before telemetry flushes, then
	// let the generation loop finish its current cycle (if any).
	stopImageServer(imageServer)

	<-loopDone
}

// startImageServer builds the read-only HTTP image server and launches its
// listen loop. It returns after the server is configured, not bound; binding
// failures are logged as warnings by the listener goroutine.
func startImageServer(config *Config) (*serve.Server, error) {
	imageServer, err := serve.New(config.Serve.Listen, config.Output.Path)
	if err != nil {
		return nil, err
	}

	log.Printf("Serving dashboard over HTTP on %s at %s", config.Serve.Listen, imageServer.Route())

	go func() {
		if serveErr := imageServer.ListenAndServe(); !errors.Is(serveErr, http.ErrServerClosed) {
			log.Printf("Warning: dashboard HTTP server: %v", serveErr)
		}
	}()

	return imageServer, nil
}

// stopImageServer drains in-flight serving requests (no-op when serving is
// disabled) bounded by the serving shutdown timeout.
func stopImageServer(imageServer *serve.Server) {
	if imageServer == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), serve.ShutdownTimeout)
	defer cancel()

	if err := imageServer.Shutdown(ctx); err != nil {
		log.Printf("Warning: HTTP server shutdown: %v", err)
	}
}

// runGenerationLoop regenerates the dashboard immediately and then on every
// refresh interval until ctx is canceled. It is read-write to the output file
// only; serving reads the same file without ever rendering on demand.
func runGenerationLoop(
	ctx context.Context,
	metrics *cycleMetrics,
	config *Config,
	calendarClient *clients.CalendarClient,
	weatherClient *clients.WeatherClient,
	loc *time.Location,
) {
	// Generate immediately, then on interval
	refreshInterval := time.Duration(config.RefreshInterval) * time.Second
	ticker := time.NewTicker(refreshInterval)

	defer ticker.Stop()

	for {
		if cycleErr := runCycle(ctx, metrics, config, calendarClient, weatherClient, loc); cycleErr != nil {
			log.Printf("Error generating dashboard: %v", cycleErr)
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

// runCycle instruments one generation cycle with a root span, records its
// outcome on the cycle metrics, and delegates to generateDashboard.
func runCycle(
	ctx context.Context,
	metrics *cycleMetrics,
	config *Config,
	calendarClient *clients.CalendarClient,
	weatherClient *clients.WeatherClient,
	loc *time.Location,
) error {
	started := time.Now()

	cycleCtx, span := otel.Tracer(instrumentationScope).Start(ctx, "dashboard.generate")

	err := generateDashboard(cycleCtx, metrics, config, calendarClient, weatherClient, loc)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	span.SetAttributes(attribute.String("dashboard.outcome", outcomeFor(err)))
	span.End()

	metrics.observeCycle(ctx, err, time.Since(started))

	return err
}

// stopTelemetry flushes and releases the telemetry providers on exit.
func stopTelemetry(shutdownTelemetry telemetry.ShutdownFunc) {
	shutdownCtx, stopCancel := context.WithTimeout(context.Background(), telemetry.ShutdownTimeout)
	defer stopCancel()

	if err := shutdownTelemetry(shutdownCtx); err != nil {
		log.Printf("Warning: telemetry shutdown: %v", err)
	}
}

// outcomeFor maps an error to the cycle outcome attribute value.
func outcomeFor(err error) string {
	if err != nil {
		return outcomeError
	}

	return outcomeSuccess
}

// generateDashboard fetches data and renders the dashboard image for one cycle.
func generateDashboard(
	ctx context.Context,
	metrics *cycleMetrics,
	config *Config,
	calendarClient *clients.CalendarClient,
	weatherClient *clients.WeatherClient,
	loc *time.Location,
) error {
	now := time.Now().In(loc)
	dates, endDate := render.GetCalendarDates(now, 4)
	startDate := dates[0]

	events, weather, err := fetchDashboardData(
		ctx, metrics, config, calendarClient, weatherClient, startDate, endDate)
	if err != nil {
		return err
	}

	// Filter out events that have already ended
	activeEvents := render.FilterPastEvents(events, now)

	// Deduplicate events on the same day with the same time and title
	activeEvents = render.DeduplicateEvents(activeEvents)

	html, css, hash, err := buildTemplate(ctx, metrics, weather, activeEvents, dates, now)
	if err != nil {
		return err
	}

	imageStart := time.Now()

	imageConfig := &render.ImageConfig{
		Width:      config.Output.Width,
		Height:     config.Output.Height,
		Rotate:     config.Output.Rotate,
		OutputPath: config.Output.Path,
		DataHash:   hash,
	}

	imageErr := render.Image(ctx, html, css, imageConfig)
	metrics.observeStage(ctx, stageImage, time.Since(imageStart))

	return imageErr
}

// fetchDashboardData runs the weather fetch concurrently with the calendar
// fetch and records both stage durations.
func fetchDashboardData(
	ctx context.Context,
	metrics *cycleMetrics,
	config *Config,
	calendarClient *clients.CalendarClient,
	weatherClient *clients.WeatherClient,
	startDate, endDate time.Time,
) (map[time.Time][]models.Event, *models.Weather, error) {
	type weatherFetch struct {
		weather *models.Weather
		err     error
	}

	weatherChan := make(chan weatherFetch, 1)

	go func() {
		stageStart := time.Now()

		weather, weatherErr := weatherClient.GetWeather(
			ctx,
			config.OpenWeatherMap.Latitude,
			config.OpenWeatherMap.Longitude,
		)
		if weatherErr != nil {
			weatherChan <- weatherFetch{err: weatherErr}

			return
		}

		metrics.observeStage(ctx, stageWeather, time.Since(stageStart))

		weatherChan <- weatherFetch{weather: weather}
	}()

	stageStart := time.Now()

	events, err := calendarClient.GetCalendars(ctx, config.Google.Calendars, startDate, endDate)
	if err != nil {
		return nil, nil, fmt.Errorf("fetching calendars: %w", err)
	}

	metrics.observeStage(ctx, stageCalendar, time.Since(stageStart))

	fetched := <-weatherChan
	if fetched.err != nil {
		return nil, nil, fmt.Errorf("fetching weather: %w", fetched.err)
	}

	return events, fetched.weather, nil
}

// buildTemplate prepares the hourly forecast data, event dates, and SVG, then
// renders the dashboard HTML (hash is computed from HTML for change detection).
func buildTemplate(
	ctx context.Context,
	metrics *cycleMetrics,
	weather *models.Weather,
	activeEvents map[time.Time][]models.Event,
	dates []time.Time,
	now time.Time,
) (string, string, string, error) {
	stageStart := time.Now()

	// Prepare hourly data (skip first, take every other up to 6)
	hourly := make([]models.HourlyForecast, 0, 6)

	for i := 1; i < len(weather.Hourly) && len(hourly) < 6; i += 2 {
		hourly = append(hourly, weather.Hourly[i])
	}

	templateData := &render.TemplateData{
		Weather:         weather,
		Hourly:          hourly,
		DatesWithEvents: render.GenerateDatesWithEvents(dates, activeEvents, now),
		Events:          activeEvents,
		SortedDates:     render.SortedEventDates(activeEvents),
		HourlySVG:       render.DrawWeatherForecast(weather.Hourly, 400, 40, 5),
		WeatherIconsCSS: render.GetWeatherIconsCSS(),
	}

	html, css, hash, err := render.HTML(ctx, templateData)
	if err != nil {
		return "", "", "", err
	}

	metrics.observeStage(ctx, stageHTML, time.Since(stageStart))

	return html, css, hash, nil
}

// newCycleMetrics creates the generation-loop instruments on the given meter.
func newCycleMetrics(meter otelmetric.Meter) (*cycleMetrics, error) {
	cycles, err := meter.Int64Counter("dashboard.cycles", otelmetric.WithDescription(
		"Dashboard generation cycles finished, by outcome."))
	if err != nil {
		return nil, fmt.Errorf("creating dashboard.cycles counter: %w", err)
	}

	cycleSeconds, err := meter.Float64Histogram("dashboard.cycle.duration",
		otelmetric.WithUnit("s"), otelmetric.WithDescription(
			"Duration of full dashboard generation cycles."))
	if err != nil {
		return nil, fmt.Errorf("creating dashboard.cycle.duration histogram: %w", err)
	}

	stageSeconds, err := meter.Float64Histogram("dashboard.stage.duration",
		otelmetric.WithUnit("s"), otelmetric.WithDescription(
			"Duration of generation stages (weather, calendar, html, image)."))
	if err != nil {
		return nil, fmt.Errorf("creating dashboard.stage.duration histogram: %w", err)
	}

	return &cycleMetrics{
		cycles:       cycles,
		cycleSeconds: cycleSeconds,
		stageSeconds: stageSeconds,
	}, nil
}

// observeCycle records one finished cycle with its outcome and duration.
func (m *cycleMetrics) observeCycle(ctx context.Context, cycleErr error, elapsed time.Duration) {
	attrs := otelmetric.WithAttributes(attribute.String("dashboard.outcome", outcomeFor(cycleErr)))

	m.cycles.Add(ctx, 1, attrs)
	m.cycleSeconds.Record(ctx, elapsed.Seconds(), attrs)
}

// observeStage records the duration of one generation stage (weather,
// calendar, html, or image).
func (m *cycleMetrics) observeStage(ctx context.Context, stage string, elapsed time.Duration) {
	m.stageSeconds.Record(ctx, elapsed.Seconds(), otelmetric.WithAttributes(
		attribute.String("dashboard.stage", stage),
	))
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // Config path is from command-line flag
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	config := &Config{
		// Set defaults
		Timezone:        defaultTimezone,
		RefreshInterval: defaultRefreshInterval,
		Output: OutputConfig{
			Path:   defaultOutputPath,
			Width:  defaultOutputWidth,
			Height: defaultOutputHeight,
			Rotate: defaultOutputRotate,
		},
		Serve: serve.Config{
			Listen: defaultServeListen,
		},
	}

	if err := yaml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	return config, nil
}
