package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/markis/home-dashboard/internal/models"
)

// WeatherClient fetches weather data from OpenWeatherMap.
type WeatherClient struct {
	httpClient  *http.Client
	apiKey      string
	location    *time.Location
	cacheMu     sync.RWMutex
	cache       map[string]*weatherCacheEntry
	cacheExpiry time.Duration
}

type weatherCacheEntry struct {
	weather   *models.Weather
	fetchedAt time.Time
}

// NewWeatherClient creates a new weather client.
func NewWeatherClient(apiKey string, loc *time.Location) *WeatherClient {
	return &WeatherClient{
		httpClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
		apiKey:      apiKey,
		location:    loc,
		cache:       make(map[string]*weatherCacheEntry),
		cacheExpiry: 10 * time.Minute,
	}
}

// GetWeather fetches weather data for a location. Cached entries (including
// stale fallbacks served after a fetch error) are attributed via the
// weather.cache.hit span attribute.
func (c *WeatherClient) GetWeather(ctx context.Context, lat, lon float64) (*models.Weather, error) {
	ctx, span := tracer().Start(ctx, "weather.get")
	defer span.End()

	cached := false
	defer func() {
		span.SetAttributes(attribute.Bool("weather.cache.hit", cached))
	}()

	cacheKey := fmt.Sprintf("%.4f,%.4f", lat, lon)

	// Check cache
	c.cacheMu.RLock()

	if entry, ok := c.cache[cacheKey]; ok {
		if time.Since(entry.fetchedAt) < c.cacheExpiry {
			c.cacheMu.RUnlock()

			cached = true

			return entry.weather, nil
		}
	}

	c.cacheMu.RUnlock()

	// Fetch fresh data
	weather, fetchErr := c.fetchWeather(ctx, lat, lon)
	if fetchErr != nil {
		span.RecordError(fetchErr)

		// Return cached data if available, even if stale
		c.cacheMu.RLock()

		if entry, ok := c.cache[cacheKey]; ok {
			c.cacheMu.RUnlock()

			cached = true

			return entry.weather, nil
		}

		c.cacheMu.RUnlock()

		span.SetStatus(codes.Error, fetchErr.Error())

		return nil, fetchErr
	}

	// Update cache
	c.cacheMu.Lock()
	c.cache[cacheKey] = &weatherCacheEntry{
		weather:   weather,
		fetchedAt: time.Now(),
	}
	c.cacheMu.Unlock()

	return weather, nil
}

func (c *WeatherClient) fetchWeather(ctx context.Context, lat, lon float64) (*models.Weather, error) {
	// Construct URL with query parameters (hardcoded base URL for security)
	const baseURL = "https://api.openweathermap.org/data/3.0/onecall"

	reqURL := fmt.Sprintf("%s?lat=%.6f&lon=%.6f&appid=%s&units=imperial&exclude=minutely",
		baseURL, lat, lon, c.apiKey)

	// Use http.NewRequest to validate URL and break taint chain
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	slog.Info("Fetching weather data", "lat", lat, "lon", lon)

	// #nosec G704 -- URL validated by http.NewRequest, hardcoded base URL
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		//nolint:errcheck // Best effort for error logging
		body, _ := io.ReadAll(resp.Body)
		// #nosec G706 -- Response is sanitized
		slog.Error("Weather API returned unexpected status", "status_code", resp.StatusCode,
			"body", sanitizeLogData(string(body)))

		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var data models.OneCallWeatherData
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	weather := &models.Weather{}
	weather.FromOneCall(&data, c.location)

	return weather, nil
}
