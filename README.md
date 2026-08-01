# hass-dashboard

A dashboard image generator that displays calendar events and weather forecasts. Designed for e-ink displays and other low-refresh-rate screens.

## Features

- Weather display with current conditions, hourly forecast, and 7-day outlook
- Calendar integration with Google Calendar (via service account)
- Automatic image generation at configurable intervals
- Configurable output dimensions and rotation
- Built-in health check for monitoring
- Docker support for easy deployment

## Requirements

- Google Cloud project with the Google Calendar API enabled and a service account authorized for your Workspace domain (see [Google Calendar Setup](#google-calendar-setup))
- OpenWeatherMap API key (One Call API 3.0)
- Chrome/Chromium (for image rendering)

## Installation

### Docker (Recommended)

```bash
docker pull ghcr.io/markis/hass-dashboard:latest
```

Create a `config.yaml` file (see [Configuration](#configuration)) and run:

```bash
docker run -v ./config.yaml:/app/config.yaml:ro -v ./output:/output -v ./credentials.json:/app/credentials.json:ro ghcr.io/markis/hass-dashboard:latest
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
      - ./credentials.json:/app/credentials.json:ro
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

### Kubernetes

Mount the service account key from a Secret and the config from a ConfigMap:

```bash
# Create the secret from your downloaded service account key JSON
kubectl create secret generic google-calendar-creds \
  --from-file=credentials.json=./credentials.json
```

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: hass-dashboard-config
data:
  config.yaml: |
    google:
      credentials_file: "/secrets/credentials.json"
      impersonate: "you@example.com"
      calendars:
        - primary
    openweathermap:
      api_key: "your-openweathermap-api-key"
      latitude: 40.7128
      longitude: -74.0060
    output:
      path: "/output/dashboard.png"
      width: 820
      height: 1200
      rotate: 270
    timezone: "America/New_York"
    refresh_interval: 600
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hass-dashboard
spec:
  replicas: 1
  selector:
    matchLabels:
      app: hass-dashboard
  template:
    metadata:
      labels:
        app: hass-dashboard
    spec:
      containers:
        - name: hass-dashboard
          image: ghcr.io/markis/hass-dashboard:latest
          args: ["--config", "/app/config.yaml"]
          volumeMounts:
            - name: config
              mountPath: /app/config.yaml
              subPath: config.yaml
              readOnly: true
            - name: credentials
              mountPath: /secrets
              readOnly: true
            - name: output
              mountPath: /output
      volumes:
        - name: config
          configMap:
            name: hass-dashboard-config
        - name: credentials
          secret:
            secretName: google-calendar-creds
        - name: output
          emptyDir: {}
```

> The OpenWeatherMap API key can also be sourced from a Secret by mounting it into the config or an env var as your setup prefers.

## Google Calendar Setup

Calendars are read directly from the Google Calendar API using a **service account with Workspace domain-wide delegation**. Set this up once:

1. **Create a service account.** In the [Google Cloud Console](https://console.cloud.google.com/), create (or pick) a project, then **IAM & Admin → Service Accounts → Create service account**. Grant it no project roles; it only needs Calendar API access.
2. **Enable the Google Calendar API.** APIs & Services → Library → search "Google Calendar API" → Enable.
3. **Download the service account key.** On the service account, **Keys → Add Key → Create new key → JSON**. Save the downloaded file as `credentials.json` (this is the only credential the app needs — JWTs are self-signed, so there is no OAuth `token.json`).
4. **Authorize domain-wide delegation.** In the [Google Workspace Admin Console](https://admin.google.com/), go to **Security → API Controls → Manage Domain-wide Delegation** and add the service account's **Client ID** (the `client_id` / numeric ID in the key JSON) with the scope:
   ```
   https://www.googleapis.com/auth/calendar.readonly
   ```
5. **Share calendars with the service account** (if they aren't the impersonated user's own). Open each calendar's settings in Google Calendar → *Share with specific people* → add the service account email (`<name>@<project>.iam.gserviceaccount.com`) with *Make changes and manage sharing* (or at least *See all event details*).
6. **Find your calendar IDs.** For each calendar, Calendar → Settings → *Integrate calendar* → *Calendar ID*. Use `primary` for the impersonated user's primary calendar.

> **Why a service account?** With domain-wide delegation the service account impersonates a Workspace user and reads their calendars without interactive browser consent — ideal for an unattended daemon. (A plain OAuth flow would require a one-time browser login and token caching; an API key only works for fully public calendars.)

## Configuration

Copy `config.example.yaml` to `config.yaml` and update the values:

```yaml
google:
  # Path to the service account key JSON (downloaded from Google Cloud).
  # In Kubernetes, mount this from a Secret.
  credentials_file: "credentials.json"
  # Workspace user email the service account impersonates (domain-wide
  # delegation). Required when using delegation; the user must have access to
  # all calendars listed below.
  impersonate: "you@example.com"
  # Calendar IDs to display. Use "primary" for the impersonated user's primary
  # calendar, or the calendar's email address (Calendar settings -> Integrate).
  calendars:
    - primary
    - family@example.com
    - holidays@group.v.calendar.google.com

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
| `google.credentials_file` | Path to the Google service account key JSON | - |
| `google.impersonate` | Workspace user email to impersonate (domain-wide delegation) | - |
| `google.calendars` | List of Google Calendar IDs (`primary` or calendar email) | - |
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
