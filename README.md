# hass-dashboard

A dashboard image generator for Home Assistant that displays calendar events and weather forecasts. Designed for e-ink displays and other low-refresh-rate screens.

## Features

- Weather display with current conditions, hourly forecast, and 7-day outlook
- Calendar integration with Home Assistant calendars
- Automatic image generation at configurable intervals
- Configurable output dimensions and rotation
- Built-in health check for monitoring
- Docker support for easy deployment

## Requirements

- Home Assistant instance with calendar entities
- OpenWeatherMap API key (One Call API 3.0)
- Chrome/Chromium (for image rendering)

## Installation

### Docker (Recommended)

```bash
docker pull ghcr.io/markis/hass-dashboard:latest
```

Create a `config.yaml` file (see [Configuration](#configuration)) and run:

```bash
docker run -v ./config.yaml:/app/config.yaml:ro -v ./output:/output ghcr.io/markis/hass-dashboard:latest
```

Or use Docker Compose:

```yaml
services:
  dashboard:
    image: ghcr.io/markis/hass-dashboard:latest
    container_name: hass-dashboard
    volumes:
      - ./config.yaml:/app/config.yaml:ro
      - ./output:/output
    healthcheck:
      test: ["CMD", "/app/scripts/healthcheck.sh", "--config", "/app/config.yaml"]
      interval: 60s
      timeout: 5s
      retries: 3
      start_period: 30s
```

### From Source

```bash
go build -o hass-dashboard ./cmd/hass-dashboard
./hass-dashboard --config config.yaml
```

## Configuration

Copy `config.example.yaml` to `config.yaml` and update the values:

```yaml
home_assistant:
  # URL to your Home Assistant instance API
  url: "http://homeassistant.local:8123/api/"
  # Long-lived access token from Home Assistant
  # Generate at: Profile -> Security -> Long-Lived Access Tokens
  token: "your-long-lived-access-token"
  # List of calendar entity IDs to display
  calendars:
    - calendar.family
    - calendar.work
    - calendar.holidays

openweathermap:
  # API key from OpenWeatherMap (One Call API 3.0)
  # Get one at: https://openweathermap.org/api
  api_key: "your-openweathermap-api-key"
  # Coordinates for weather data
  latitude: 40.7128
  longitude: -74.0060

output:
  # Path to save the generated dashboard image
  path: "/output/dashboard.png"
  # Image dimensions in pixels
  width: 820
  height: 1200
  # Rotation in degrees (0, 90, 180, 270)
  rotate: 270

# Timezone for date/time display (IANA timezone name)
timezone: "America/New_York"

# How often to regenerate the dashboard (in seconds)
refresh_interval: 600
```

### Configuration Options

| Option | Description | Default |
|--------|-------------|---------|
| `home_assistant.url` | Home Assistant API URL | - |
| `home_assistant.token` | Long-lived access token | - |
| `home_assistant.calendars` | List of calendar entity IDs | - |
| `openweathermap.api_key` | OpenWeatherMap API key | - |
| `openweathermap.latitude` | Location latitude | - |
| `openweathermap.longitude` | Location longitude | - |
| `output.path` | Output image path | `output.png` |
| `output.width` | Image width in pixels | `820` |
| `output.height` | Image height in pixels | `1200` |
| `output.rotate` | Rotation in degrees | `270` |
| `timezone` | IANA timezone name | `America/New_York` |
| `refresh_interval` | Refresh interval in seconds | `600` |

## Usage

```bash
./hass-dashboard --config config.yaml
```

The application will generate a dashboard image immediately and then regenerate it at the configured interval. The generated PNG image can be served to e-ink displays or other devices.

## Health Check

The container includes a built-in health check script that monitors the output file to ensure the dashboard is generating properly.

### How It Works

The health check script (`scripts/healthcheck.sh`):
- Reads `output.path` and `refresh_interval` from your config file
- Checks if the output file exists
- Verifies the file has been updated within `2 × refresh_interval` seconds
- Returns exit code 0 (healthy) or 1 (unhealthy)

### Manual Usage

You can run the health check manually:

```bash
# From the container
/app/scripts/healthcheck.sh --config /app/config.yaml

# From the host (if you have yq installed)
./scripts/healthcheck.sh --config config.yaml
```

**Silent on success**, outputs error messages on failure:
```
ERROR: Output file is stale (age: 1500s, threshold: 1200s): /output/dashboard.png
```

### Docker Health Check

The health check is automatically configured in the Docker Compose file. View status:

```bash
# Check container health status
docker ps

# View health check logs
docker inspect hass-dashboard | jq '.[0].State.Health'
```

Health check parameters:
- **Interval**: 60 seconds between checks
- **Timeout**: 5 seconds per check
- **Retries**: 3 failed checks before marking unhealthy
- **Start period**: 30 seconds grace period after container start

### Requirements

The health check requires `yq` (YAML processor), which is included in the Docker image.

## License

MIT
