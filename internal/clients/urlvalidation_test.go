package clients

import (
	"strings"
	"testing"
)

func TestValidateHTTPURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
	}{
		{
			name:    "valid http URL",
			url:     "http://homeassistant.local:8123/api/",
			wantErr: false,
		},
		{
			name:    "valid https URL",
			url:     "https://homeassistant.example.com/api/",
			wantErr: false,
		},
		{
			name:    "rejects file:// scheme",
			url:     "file:///etc/passwd",
			wantErr: true,
		},
		{
			name:    "rejects ftp:// scheme",
			url:     "ftp://example.com",
			wantErr: true,
		},
		{
			name:    "rejects javascript: scheme",
			url:     "javascript:alert(1)",
			wantErr: true,
		},
		{
			name:    "rejects empty URL",
			url:     "",
			wantErr: true,
		},
		{
			name:    "rejects URL without host",
			url:     "http://",
			wantErr: true,
		},
		{
			name:    "rejects malformed URL",
			url:     "not a url",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateHTTPURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateHTTPURL(%q) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			}
		})
	}
}

func TestValidateOpenWeatherMapURL(t *testing.T) {
	tests := []struct {
		name    string
		apiKey  string
		lat     float64
		lon     float64
		wantErr bool
	}{
		{
			name:    "valid coordinates and API key",
			apiKey:  "test-api-key",
			lat:     40.7128,
			lon:     -74.0060,
			wantErr: false,
		},
		{
			name:    "valid extreme north",
			apiKey:  "test-api-key",
			lat:     90.0,
			lon:     0.0,
			wantErr: false,
		},
		{
			name:    "valid extreme south",
			apiKey:  "test-api-key",
			lat:     -90.0,
			lon:     0.0,
			wantErr: false,
		},
		{
			name:    "valid extreme east",
			apiKey:  "test-api-key",
			lat:     0.0,
			lon:     180.0,
			wantErr: false,
		},
		{
			name:    "valid extreme west",
			apiKey:  "test-api-key",
			lat:     0.0,
			lon:     -180.0,
			wantErr: false,
		},
		{
			name:    "rejects invalid latitude (too high)",
			apiKey:  "test-api-key",
			lat:     91.0,
			lon:     0.0,
			wantErr: true,
		},
		{
			name:    "rejects invalid latitude (too low)",
			apiKey:  "test-api-key",
			lat:     -91.0,
			lon:     0.0,
			wantErr: true,
		},
		{
			name:    "rejects invalid longitude (too high)",
			apiKey:  "test-api-key",
			lat:     0.0,
			lon:     181.0,
			wantErr: true,
		},
		{
			name:    "rejects invalid longitude (too low)",
			apiKey:  "test-api-key",
			lat:     0.0,
			lon:     -181.0,
			wantErr: true,
		},
		{
			name:    "rejects empty API key",
			apiKey:  "",
			lat:     40.7128,
			lon:     -74.0060,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, err := validateOpenWeatherMapURL(tt.apiKey, tt.lat, tt.lon)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateOpenWeatherMapURL() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr {
				// Verify it's the official API endpoint
				if !strings.HasPrefix(url, "https://api.openweathermap.org/data/3.0/onecall") {
					t.Errorf("URL doesn't use official OpenWeatherMap endpoint: %s", url)
				}
				// Verify API key is in URL
				if !strings.Contains(url, "appid="+tt.apiKey) {
					t.Errorf("API key not found in URL: %s", url)
				}
			}
		})
	}
}
