# Design: Local Weather Icons in Docker Image

## Overview

Bundle Weather Icons library locally in the Docker image to eliminate CDN dependency and improve icon loading reliability.

## Problem Statement

Weather Icons are currently loaded from `cdnjs.cloudflare.com`, which occasionally fails to load, causing missing icons in generated dashboard images. This creates a network dependency that impacts reliability.

## Solution

Download Weather Icons during Docker build and embed them as base64 data URIs in the HTML template, eliminating all network dependencies for weather icon fonts.

## Architecture

### File Structure

```
static/
  fonts/
    weathericons-regular-webfont.woff2
    weathericons-regular-webfont.woff
  css/
    weather-icons.min.css (modified with local font paths)
```

Fonts will be downloaded during Docker build and copied into the image at `/app/static/`.

### Docker Build Changes

Modify `Containerfile` to:

1. Add a build stage that downloads Weather Icons from the official GitHub repository or CDN
2. Extract necessary font files (WOFF2 and WOFF formats for browser compatibility)
3. Convert fonts to base64 data URIs for embedding
4. Copy static assets into the runtime image

This occurs in the builder stage, keeping the runtime image clean and organized.

### CSS and Font Loading (Option B - Inline CSS)

**Approach:**
- Download Weather Icons CSS during build
- Convert font URLs to base64 data URIs
- Embed complete `@font-face` declarations directly in `dashboard.html`
- Remove CDN `@import` from `style.css`

**Benefits:**
- No file path issues with chromedp
- Self-contained HTML rendering
- Zero network requests or file system access needed
- Simplified chromedp configuration

**Implementation:**
- Create Go template variable for embedded font CSS
- Inject into `<style>` block in `dashboard.html`
- Base64-encoded fonts load directly from HTML

### Template Integration

**Current state:**
```css
/* style.css */
@import url("https://cdnjs.cloudflare.com/ajax/libs/weather-icons/2.0.10/css/weather-icons.min.css");
```

**New state:**
```html
<!-- dashboard.html -->
<style>
{{ .WeatherIconsCSS }}
</style>
```

Where `WeatherIconsCSS` contains the complete `@font-face` declarations with base64-encoded font data.

### Error Handling Improvements

Since fonts will be bundled:
- Remove retry logic from `internal/render/image.go` (lines 304-322) for weather-icons CDN checks
- Simplify font loading verification since there's no network dependency
- Keep retry logic for Font Awesome if it remains CDN-based

### Testing Strategy

1. Verify font rendering works in generated images
2. Test that icons display correctly for all weather conditions (sunny, cloudy, rainy, etc.)
3. Confirm no network requests are made for Weather Icons during image generation
4. Validate Docker image build completes successfully
5. Check that image size increase is acceptable (~100-200KB for fonts)

## Trade-offs

**Pros:**
- Zero network dependency for Weather Icons
- Improved reliability - fonts always available
- Faster loading - no external requests
- Works offline

**Cons:**
- Larger Docker image size (~100-200KB)
- Fonts frozen at build time (requires rebuild to update)
- More complex build process

## Implementation Steps

1. Create static directory structure
2. Add download step to Containerfile
3. Create script to convert fonts to base64 data URIs
4. Modify Go template rendering to include embedded CSS
5. Update dashboard.html template
6. Remove CDN import from style.css
7. Test font rendering
8. Clean up retry logic for weather-icons

## Success Criteria

- Weather icons render correctly in all generated images
- No network requests made for weather-icons during rendering
- Docker build succeeds without errors
- Image size increase is within acceptable range
- Existing functionality remains intact
