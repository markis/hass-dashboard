package clients

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/markis/hass-dashboard/internal/models"
)

const openWeatherBaseURL = "https://api.openweathermap.org/data/3.0/onecall"

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
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		apiKey:      apiKey,
		location:    loc,
		cache:       make(map[string]*weatherCacheEntry),
		cacheExpiry: 10 * time.Minute,
	}
}

// GetWeather fetches weather data for a location.
func (c *WeatherClient) GetWeather(ctx context.Context, lat, lon float64) (*models.Weather, error) {
	cacheKey := fmt.Sprintf("%.4f,%.4f", lat, lon)

	// Check cache
	c.cacheMu.RLock()

	if entry, ok := c.cache[cacheKey]; ok {
		if time.Since(entry.fetchedAt) < c.cacheExpiry {
			c.cacheMu.RUnlock()

			return entry.weather, nil
		}
	}

	c.cacheMu.RUnlock()

	// Fetch fresh data
	weather, err := c.fetchWeather(ctx, lat, lon)
	if err != nil {
		// Return cached data if available, even if stale
		c.cacheMu.RLock()

		if entry, ok := c.cache[cacheKey]; ok {
			c.cacheMu.RUnlock()

			return entry.weather, nil
		}

		c.cacheMu.RUnlock()

		return nil, err
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
	params := url.Values{}
	params.Set("lat", fmt.Sprintf("%.6f", lat))
	params.Set("lon", fmt.Sprintf("%.6f", lon))
	params.Set("appid", c.apiKey)
	params.Set("units", "imperial")
	params.Set("exclude", "minutely")

	reqURL := openWeatherBaseURL + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	log.Printf("Fetching weather data for lat=%.4f, lon=%.4f", lat, lon)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body) //nolint:errcheck // Best effort for error logging
		log.Printf("Weather API error (status %d): %s", resp.StatusCode, string(body))

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
