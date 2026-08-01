package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigDefaults(t *testing.T) {
	// Create a minimal config file
	configContent := `
google:
  credentials_file: "credentials.json"
  impersonate: ""
  calendars:
    - primary
openweathermap:
  api_key: "test-key"
  latitude: 40.7128
  longitude: -74.0060
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte(configContent), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	config, err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("loadConfig error: %v", err)
	}

	// Check defaults are applied
	if config.Timezone != "America/New_York" {
		t.Errorf("Timezone = %q, want %q", config.Timezone, "America/New_York")
	}

	if config.RefreshInterval != 600 {
		t.Errorf("RefreshInterval = %d, want 600", config.RefreshInterval)
	}

	if config.Output.Path != "output.jpg" {
		t.Errorf("Output.Path = %q, want %q", config.Output.Path, "output.jpg")
	}

	if config.Output.Width != 820 {
		t.Errorf("Output.Width = %d, want 820", config.Output.Width)
	}

	if config.Output.Height != 1200 {
		t.Errorf("Output.Height = %d, want 1200", config.Output.Height)
	}

	if config.Output.Rotate != 270 {
		t.Errorf("Output.Rotate = %d, want 270", config.Output.Rotate)
	}
}

func TestLoadConfigCustomValues(t *testing.T) {
	configContent := `
google:
  credentials_file: "/secrets/service-account.json"
  impersonate: "user@example.com"
  calendars:
    - primary
    - family@example.com
    - holidays@group.v.calendar.google.com
openweathermap:
  api_key: "owm-api-key"
  latitude: 51.5074
  longitude: -0.1278
output:
  path: "/data/dashboard.jpg"
  width: 1024
  height: 768
  rotate: 0
timezone: "Europe/London"
refresh_interval: 300
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte(configContent), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	config, err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("loadConfig error: %v", err)
	}

	// Check Google config
	if config.Google.CredentialsFile != "/secrets/service-account.json" {
		t.Errorf("Google.CredentialsFile = %q, want %q", config.Google.CredentialsFile, "/secrets/service-account.json")
	}

	if config.Google.Impersonate != "user@example.com" {
		t.Errorf("Google.Impersonate = %q, want %q", config.Google.Impersonate, "user@example.com")
	}

	if len(config.Google.Calendars) != 3 {
		t.Errorf("len(Google.Calendars) = %d, want 3", len(config.Google.Calendars))
	}

	// Check OpenWeatherMap config
	if config.OpenWeatherMap.Key != "owm-api-key" {
		t.Errorf("OpenWeatherMap.Key = %q, want %q", config.OpenWeatherMap.Key, "owm-api-key")
	}

	if config.OpenWeatherMap.Latitude != 51.5074 {
		t.Errorf("OpenWeatherMap.Latitude = %f, want 51.5074", config.OpenWeatherMap.Latitude)
	}

	if config.OpenWeatherMap.Longitude != -0.1278 {
		t.Errorf("OpenWeatherMap.Longitude = %f, want -0.1278", config.OpenWeatherMap.Longitude)
	}

	// Check Output config
	if config.Output.Path != "/data/dashboard.jpg" {
		t.Errorf("Output.Path = %q, want %q", config.Output.Path, "/data/dashboard.jpg")
	}

	if config.Output.Width != 1024 {
		t.Errorf("Output.Width = %d, want 1024", config.Output.Width)
	}

	if config.Output.Height != 768 {
		t.Errorf("Output.Height = %d, want 768", config.Output.Height)
	}

	if config.Output.Rotate != 0 {
		t.Errorf("Output.Rotate = %d, want 0", config.Output.Rotate)
	}

	// Check other config
	if config.Timezone != "Europe/London" {
		t.Errorf("Timezone = %q, want %q", config.Timezone, "Europe/London")
	}

	if config.RefreshInterval != 300 {
		t.Errorf("RefreshInterval = %d, want 300", config.RefreshInterval)
	}
}

func TestLoadConfigFileNotFound(t *testing.T) {
	_, err := loadConfig("/nonexistent/config.yaml")
	if err == nil {
		t.Error("expected error for missing config file")
	}
}

func TestLoadConfigInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	invalidContent := `
google:
  credentials_file: [invalid yaml
`
	if err := os.WriteFile(configPath, []byte(invalidContent), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	_, err := loadConfig(configPath)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}

func TestLoadConfigEmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte(""), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	config, err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("loadConfig error: %v", err)
	}

	// Defaults should still be applied
	if config.Timezone != "America/New_York" {
		t.Errorf("Timezone = %q, want %q", config.Timezone, "America/New_York")
	}

	if config.RefreshInterval != 600 {
		t.Errorf("RefreshInterval = %d, want 600", config.RefreshInterval)
	}
}

func TestLoadConfigPartialOverride(t *testing.T) {
	// Only override some values, others should use defaults
	configContent := `
google:
  credentials_file: "credentials.json"
  impersonate: ""
  calendars: []
openweathermap:
  api_key: "key"
output:
  width: 1920
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte(configContent), 0o600); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	config, err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("loadConfig error: %v", err)
	}

	// Overridden value
	if config.Output.Width != 1920 {
		t.Errorf("Output.Width = %d, want 1920", config.Output.Width)
	}

	// Default values should remain
	if config.Output.Height != 1200 {
		t.Errorf("Output.Height = %d, want 1200 (default)", config.Output.Height)
	}

	if config.Output.Path != "output.jpg" {
		t.Errorf("Output.Path = %q, want %q (default)", config.Output.Path, "output.jpg")
	}

	if config.Timezone != "America/New_York" {
		t.Errorf("Timezone = %q, want %q (default)", config.Timezone, "America/New_York")
	}
}
