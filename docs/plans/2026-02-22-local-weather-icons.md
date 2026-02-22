# Local Weather Icons Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Bundle Weather Icons locally in Docker image to eliminate CDN dependency and improve icon loading reliability.

**Architecture:** Download Weather Icons during Docker build, convert fonts to base64, embed CSS with data URIs in Go template, and inject into HTML at render time.

**Tech Stack:** Go embed, Docker multi-stage builds, base64 encoding, HTML templates

---

## Task 1: Download and Prepare Weather Icons

**Files:**
- Create: `static/fonts/.gitkeep`
- Create: `scripts/download-weather-icons.sh`
- Modify: `.gitignore`

**Step 1: Create static directory structure**

```bash
mkdir -p static/fonts
touch static/fonts/.gitkeep
```

**Step 2: Create download script**

Create `scripts/download-weather-icons.sh`:

```bash
#!/bin/sh
set -e

WEATHER_ICONS_VERSION="2.0.10"
DOWNLOAD_DIR="/tmp/weather-icons"
OUTPUT_DIR="static/fonts"

echo "Downloading Weather Icons ${WEATHER_ICONS_VERSION}..."

# Download from CDN
mkdir -p "${DOWNLOAD_DIR}"
cd "${DOWNLOAD_DIR}"

# Download font files
curl -fsSL "https://cdnjs.cloudflare.com/ajax/libs/weather-icons/${WEATHER_ICONS_VERSION}/font/weathericons-regular-webfont.woff2" \
  -o weathericons-regular-webfont.woff2

curl -fsSL "https://cdnjs.cloudflare.com/ajax/libs/weather-icons/${WEATHER_ICONS_VERSION}/font/weathericons-regular-webfont.woff" \
  -o weathericons-regular-webfont.woff

# Download CSS
curl -fsSL "https://cdnjs.cloudflare.com/ajax/libs/weather-icons/${WEATHER_ICONS_VERSION}/css/weather-icons.min.css" \
  -o weather-icons.min.css

echo "Download complete. Files:"
ls -lh

# Copy to output directory
mkdir -p "${OUTPUT_DIR}"
cp *.woff* "${OUTPUT_DIR}/"
cp weather-icons.min.css "${OUTPUT_DIR}/"

echo "Weather Icons installed to ${OUTPUT_DIR}"
```

**Step 3: Make script executable**

```bash
chmod +x scripts/download-weather-icons.sh
```

**Step 4: Update .gitignore**

Add to `.gitignore`:
```
static/fonts/*.woff*
static/fonts/*.css
```

**Step 5: Test download script locally**

Run: `./scripts/download-weather-icons.sh`
Expected: Files downloaded to `static/fonts/`

**Step 6: Commit**

```bash
git add static/fonts/.gitkeep scripts/download-weather-icons.sh .gitignore
git commit -m "feat: add script to download Weather Icons fonts"
```

---

## Task 2: Create Font-to-Base64 Conversion Script

**Files:**
- Create: `scripts/fonts-to-css.sh`

**Step 1: Create conversion script**

Create `scripts/fonts-to-css.sh`:

```bash
#!/bin/sh
set -e

FONTS_DIR="static/fonts"
OUTPUT_FILE="static/fonts/weather-icons-embedded.css"

echo "Converting fonts to base64 data URIs..."

# Read the base CSS
cp "${FONTS_DIR}/weather-icons.min.css" "${OUTPUT_FILE}"

# Convert WOFF2 to base64
WOFF2_BASE64=$(base64 -w 0 "${FONTS_DIR}/weathericons-regular-webfont.woff2" 2>/dev/null || base64 "${FONTS_DIR}/weathericons-regular-webfont.woff2")

# Convert WOFF to base64
WOFF_BASE64=$(base64 -w 0 "${FONTS_DIR}/weathericons-regular-webfont.woff" 2>/dev/null || base64 "${FONTS_DIR}/weathericons-regular-webfont.woff")

# Create embedded CSS with data URIs
cat > "${OUTPUT_FILE}" << EOF
@font-face {
  font-family: 'weathericons';
  src: url(data:font/woff2;charset=utf-8;base64,${WOFF2_BASE64}) format('woff2'),
       url(data:font/woff;charset=utf-8;base64,${WOFF_BASE64}) format('woff');
  font-weight: normal;
  font-style: normal;
}

.wi {
  display: inline-block;
  font-family: 'weathericons';
  font-style: normal;
  font-weight: normal;
  line-height: 1;
  -webkit-font-smoothing: antialiased;
  -moz-osx-font-smoothing: grayscale;
}
EOF

# Append icon classes from original CSS (skip @font-face)
grep "^\.wi-" "${FONTS_DIR}/weather-icons.min.css" >> "${OUTPUT_FILE}" || true

echo "Embedded CSS created at ${OUTPUT_FILE}"
ls -lh "${OUTPUT_FILE}"
```

**Step 2: Make script executable**

```bash
chmod +x scripts/fonts-to-css.sh
```

**Step 3: Update .gitignore**

Add to `.gitignore`:
```
static/fonts/weather-icons-embedded.css
```

**Step 4: Test conversion script**

Run: `./scripts/download-weather-icons.sh && ./scripts/fonts-to-css.sh`
Expected: `static/fonts/weather-icons-embedded.css` created with base64 fonts

**Step 5: Verify CSS file**

Run: `head -20 static/fonts/weather-icons-embedded.css`
Expected: See `@font-face` with `data:font/woff2;base64,...`

**Step 6: Commit**

```bash
git add scripts/fonts-to-css.sh .gitignore
git commit -m "feat: add script to convert fonts to base64 embedded CSS"
```

---

## Task 3: Update Containerfile

**Files:**
- Modify: `Containerfile`

**Step 1: Add font download to builder stage**

In `Containerfile`, after line 10 (`COPY internal ./internal`), add:

```dockerfile
COPY scripts ./scripts
COPY static ./static
RUN apk add --no-cache curl && \
    ./scripts/download-weather-icons.sh && \
    ./scripts/fonts-to-css.sh
```

**Step 2: Copy embedded CSS to runtime stage**

In `Containerfile`, after line 43 (`COPY scripts/healthcheck.sh /app/scripts/healthcheck.sh`), add:

```dockerfile
COPY --from=builder /src/static/fonts/weather-icons-embedded.css /app/static/fonts/weather-icons-embedded.css
```

**Step 3: Verify Containerfile syntax**

The builder stage should now look like:
```dockerfile
FROM golang:1.25 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY scripts ./scripts
COPY static ./static
RUN apk add --no-cache curl && \
    ./scripts/download-weather-icons.sh && \
    ./scripts/fonts-to-css.sh
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /hass-dashboard ./cmd/hass-dashboard
```

**Step 4: Commit**

```bash
git add Containerfile
git commit -m "feat: add Weather Icons font download to Docker build"
```

---

## Task 4: Embed CSS in Go Template

**Files:**
- Modify: `internal/render/template.go`

**Step 1: Add embed directive for weather icons CSS**

After line 18 in `internal/render/template.go`, add:

```go
//go:embed templates/weather-icons-embedded.css
var weatherIconsCSS string
```

**Step 2: Add WeatherIconsCSS to TemplateData**

Modify the `TemplateData` struct (around line 21-28):

```go
// TemplateData holds all data needed to render the dashboard.
type TemplateData struct {
	Weather         *models.Weather
	Hourly          []models.HourlyForecast
	DatesWithEvents []models.DateCount
	Events          map[time.Time][]models.Event
	SortedDates     []time.Time
	HourlySVG       template.CSS
	WeatherIconsCSS template.CSS
}
```

**Step 3: Populate WeatherIconsCSS in HTML function**

We need to find where `TemplateData` is created and ensure `WeatherIconsCSS` is set. First, let's check:

Run: `grep -n "TemplateData{" internal/render/*.go cmd/hass-dashboard/*.go`
Expected: Find where TemplateData is instantiated

This will be completed after finding the instantiation location.

**Step 4: Verify code compiles**

Run: `go build ./cmd/hass-dashboard`
Expected: Build fails because WeatherIconsCSS is not set (we'll fix this next)

---

## Task 5: Set WeatherIconsCSS in Template Data

**Files:**
- Modify: `cmd/hass-dashboard/main.go` (likely around line 170-180 based on previous context)
- Modify: `internal/render/template.go`

**Step 1: Find TemplateData instantiation**

Run: `grep -B5 -A10 "TemplateData{" cmd/hass-dashboard/main.go`
Expected: Find where template data is created

**Step 2: Add weatherIconsCSS to template.go**

In `internal/render/template.go`, create a function to get the embedded CSS:

```go
// GetWeatherIconsCSS returns the embedded Weather Icons CSS.
func GetWeatherIconsCSS() template.CSS {
	return template.CSS(weatherIconsCSS)
}
```

**Step 3: Update TemplateData creation in main.go**

Modify the TemplateData instantiation to include:

```go
WeatherIconsCSS: render.GetWeatherIconsCSS(),
```

**Step 4: Verify code compiles**

Run: `go build ./cmd/hass-dashboard`
Expected: Successful build

**Step 5: Commit**

```bash
git add internal/render/template.go cmd/hass-dashboard/main.go
git commit -m "feat: embed Weather Icons CSS in template data"
```

---

## Task 6: Update HTML Template

**Files:**
- Modify: `internal/render/templates/dashboard.html`

**Step 1: Add Weather Icons CSS to template**

In `dashboard.html`, modify the `<style>` block at the top (lines 1-5):

```html
<style>
{{ .WeatherIconsCSS }}
  .hourly-forecast {
    background-image: {{ .HourlySVG }};
  }
</style>
```

**Step 2: Verify template syntax**

Run: `go build ./cmd/hass-dashboard`
Expected: Successful build

**Step 3: Commit**

```bash
git add internal/render/templates/dashboard.html
git commit -m "feat: inject embedded Weather Icons CSS into dashboard template"
```

---

## Task 7: Remove CDN Import from style.css

**Files:**
- Modify: `internal/render/templates/style.css`

**Step 1: Remove CDN import**

In `style.css`, delete line 1:

```css
@import url("https://cdnjs.cloudflare.com/ajax/libs/weather-icons/2.0.10/css/weather-icons.min.css");
```

**Step 2: Verify build**

Run: `go build ./cmd/hass-dashboard`
Expected: Successful build

**Step 3: Commit**

```bash
git add internal/render/templates/style.css
git commit -m "feat: remove Weather Icons CDN dependency from CSS"
```

---

## Task 8: Copy Embedded CSS to Templates Directory

**Files:**
- Create: `internal/render/templates/weather-icons-embedded.css`

**Step 1: Run font scripts to generate CSS**

```bash
./scripts/download-weather-icons.sh
./scripts/fonts-to-css.sh
```

**Step 2: Copy embedded CSS to templates**

```bash
cp static/fonts/weather-icons-embedded.css internal/render/templates/weather-icons-embedded.css
```

**Step 3: Update .gitignore to track the template CSS**

The embedded CSS in `internal/render/templates/` should be tracked, but not in `static/fonts/`.

Ensure `.gitignore` has:
```
static/fonts/*.css
```

But NOT:
```
internal/render/templates/*.css
```

**Step 4: Verify embed works**

Run: `go build ./cmd/hass-dashboard`
Expected: Successful build with embedded CSS

**Step 5: Commit**

```bash
git add internal/render/templates/weather-icons-embedded.css
git commit -m "feat: add embedded Weather Icons CSS to templates"
```

---

## Task 9: Clean Up Weather Icons Network Check

**Files:**
- Modify: `internal/render/image.go:294-322`

**Step 1: Update checkAndLogNetworkErrors function**

Replace the function around line 294-322 to only check for Font Awesome:

```go
// checkAndLogNetworkErrors checks for network-related errors in the console
// and logs them as warnings to help diagnose font loading issues.
func checkAndLogNetworkErrors(ctx context.Context) {
	var hasNetworkErrors bool

	// Check for failed resource loads (Font Awesome only - Weather Icons are embedded)
	err := chromedp.Evaluate(`
		(async () => {
			const resources = performance.getEntriesByType('resource');
			const failed = resources.filter(r => 
				r.name.includes('fonts.googleapis') &&
				r.transferSize === 0 && r.decodedBodySize === 0
			);
			return failed.length > 0;
		})()
	`, &hasNetworkErrors).Do(ctx)
	if err != nil {
		log.Printf("Warning: Could not check for network errors: %v", err)

		return
	}

	if hasNetworkErrors {
		log.Printf(
			"WARNING: Network errors detected - Font Awesome may have failed to load. " +
				"Check network connectivity.",
		)
	}
}
```

**Step 2: Verify code compiles**

Run: `go build ./cmd/hass-dashboard`
Expected: Successful build

**Step 3: Commit**

```bash
git add internal/render/image.go
git commit -m "refactor: remove Weather Icons from network error checks"
```

---

## Task 10: Test Docker Build

**Files:**
- None (testing only)

**Step 1: Build Docker image**

Run: `docker build -f Containerfile -t hass-dashboard:test .`
Expected: Build completes successfully

**Step 2: Check image size**

Run: `docker images hass-dashboard:test`
Expected: Image size increased by ~100-200KB

**Step 3: Run container and generate image**

Run: `docker run --rm -v $(pwd)/config.yaml:/app/config.yaml hass-dashboard:test`
Expected: Dashboard image generated successfully

**Step 4: Verify no network requests for Weather Icons**

Check logs for: "WARNING: Network errors detected - CDN resources (fonts/icons) may have failed to load"
Expected: Warning should not mention weather-icons, only Font Awesome if it fails

**Step 5: Visual inspection**

Open generated image and verify Weather Icons display correctly.
Expected: Icons render properly for all weather conditions

---

## Task 11: Update Tests

**Files:**
- Modify: `internal/render/template_test.go` (if tests exist for template rendering)

**Step 1: Check existing template tests**

Run: `grep -n "WeatherIconsCSS" internal/render/template_test.go`
Expected: May not exist yet

**Step 2: Add WeatherIconsCSS to test data**

If tests exist that create `TemplateData`, add:

```go
WeatherIconsCSS: render.GetWeatherIconsCSS(),
```

**Step 3: Run tests**

Run: `go test ./internal/render/... -v`
Expected: All tests pass

**Step 4: Commit if changes were needed**

```bash
git add internal/render/template_test.go
git commit -m "test: update template tests for embedded Weather Icons CSS"
```

---

## Task 12: Final Integration Test

**Files:**
- None (testing only)

**Step 1: Clean rebuild**

```bash
go clean
go build ./cmd/hass-dashboard
```

**Step 2: Run application locally**

Run: `./hass-dashboard --config config.yaml`
Expected: Image generated successfully

**Step 3: Verify Weather Icons in output**

Open the generated image file.
Expected: All weather icons display correctly

**Step 4: Check for network errors in logs**

Review application logs.
Expected: No warnings about weather-icons CDN failures

**Step 5: Test with network disabled**

Disable network and run again.
Expected: Weather Icons still work (Font Awesome may fail, which is acceptable)

---

## Success Criteria Checklist

- [ ] Docker build completes successfully
- [ ] Weather Icons render correctly in generated images
- [ ] No network requests for weather-icons during rendering
- [ ] Image size increase is ~100-200KB
- [ ] All existing tests pass
- [ ] Application runs successfully with embedded fonts
- [ ] Network error checks only reference Font Awesome, not Weather Icons

---

## Notes

- The embedded CSS file will be ~150-200KB due to base64 encoding
- Font Awesome still uses CDN - this can be addressed in a future task if needed
- If base64 command differs between macOS and Linux, the script handles both with fallback
