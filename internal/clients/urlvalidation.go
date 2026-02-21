package clients

import (
	"fmt"
	"net/url"
)

// validateHTTPURL ensures a URL from config is safe for HTTP requests.
// This prevents SSRF attacks by validating the URL scheme and structure.
func validateHTTPURL(rawURL string) (*url.URL, error) {
	if rawURL == "" {
		return nil, fmt.Errorf("URL cannot be empty")
	}

	parsedURL, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Only allow http and https schemes
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("invalid URL scheme %q: only http and https are allowed", parsedURL.Scheme)
	}

	// Ensure host is present
	if parsedURL.Host == "" {
		return nil, fmt.Errorf("URL must have a host")
	}

	return parsedURL, nil
}

// validateOpenWeatherMapURL validates and constructs the OpenWeatherMap API URL.
// Uses an allow-list approach - only allows the official API endpoint.
func validateOpenWeatherMapURL(apiKey string, lat, lon float64) (string, error) {
	if apiKey == "" {
		return "", fmt.Errorf("OpenWeatherMap API key is required")
	}

	// Validate latitude and longitude ranges
	if lat < -90 || lat > 90 {
		return "", fmt.Errorf("invalid latitude: must be between -90 and 90")
	}

	if lon < -180 || lon > 180 {
		return "", fmt.Errorf("invalid longitude: must be between -180 and 180")
	}

	// Use only the official OpenWeatherMap API endpoint (allow-list)
	const baseURL = "https://api.openweathermap.org/data/3.0/onecall"

	// Build URL with validated parameters
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		// This should never happen with a hardcoded URL
		return "", fmt.Errorf("internal error: failed to parse base URL: %w", err)
	}

	query := parsedURL.Query()
	query.Set("lat", fmt.Sprintf("%.4f", lat))
	query.Set("lon", fmt.Sprintf("%.4f", lon))
	query.Set("appid", apiKey)
	query.Set("units", "imperial")
	parsedURL.RawQuery = query.Encode()

	return parsedURL.String(), nil
}
