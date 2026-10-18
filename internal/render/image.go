// Package render provides HTML template rendering and image generation functionality.
package render

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otelmetric "go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/chromedp/chromedp"
)

// ImageConfig holds configuration for image rendering.
type ImageConfig struct {
	Width              int
	Height             int
	Rotate             int
	OutputPath         string
	DataHash           string        // SHA-256 hash of input data for change detection
	FontLoadTimeout    time.Duration // Maximum time to wait for fonts, 0 uses default (5s)
	FontLoadMaxRetries int           // Maximum retry attempts, 0 uses default (5)
	JPEGQuality        int           // JPEG quality (1-100), 0 uses default (85)
}

// validateOutputPath ensures the output path is safe and doesn't contain path traversal attempts.
func validateOutputPath(path string) error {
	// Check for literal .. in the path before cleaning (basic check)
	if strings.Contains(path, "..") {
		return fmt.Errorf("invalid output path: contains path traversal")
	}

	// Clean the path to resolve any . components
	cleaned := filepath.Clean(path)

	// Ensure it's a valid path
	if !filepath.IsAbs(cleaned) {
		// Convert to absolute path to validate it
		_, err := filepath.Abs(cleaned)
		if err != nil {
			return fmt.Errorf("resolving output path: %w", err)
		}
	}

	return nil
}

// Result attribute values for the dashboard.render.updates counter.
const (
	renderResultWritten = "written"
	renderResultSkipped = "skipped"
)

// Image takes HTML and CSS content and renders it to a JPEG image.
// It only writes the file if the content has changed (based on input data SHA-256 hash).
// If config.DataHash is provided, it compares against stored hash before rendering.
func Image(ctx context.Context, html, css string, config *ImageConfig) error {
	ctx, span := tracer().Start(ctx, "render.image")
	defer span.End()

	counter, meterErr := meter().Int64Counter("dashboard.render.updates",
		otelmetric.WithDescription("Dashboard image renders, by result (written or skipped)."))
	if meterErr != nil {
		slog.Warn("Failed to create dashboard.render.updates counter", "error", meterErr)

		counter = nil
	}

	// Validate output path to prevent path traversal
	if pathErr := validateOutputPath(config.OutputPath); pathErr != nil {
		recordError(span, pathErr)

		return pathErr
	}

	// Check if data has changed before rendering
	if config.DataHash != "" {
		unchanged, hashErr := checkDataUnchanged(config.OutputPath, config.DataHash)
		switch {
		case hashErr != nil:
			logHashError(config.OutputPath, hashErr)
		case unchanged:
			recordRenderUpdate(ctx, span, counter, renderResultSkipped)

			return touchHashFileAndReturn(config.OutputPath)
		}
	}

	buf, err := ImageBytes(ctx, html, css, config)
	if err != nil {
		recordError(span, err)

		return err
	}

	// Write image file
	if err := writeImageFile(buf, config.OutputPath); err != nil {
		recordError(span, err)

		return err
	}

	// Store data hash if provided
	if config.DataHash != "" {
		if err := saveDataHash(config.OutputPath, config.DataHash); err != nil {
			slog.Warn("Failed to write hash file", "output_path", config.OutputPath, "error", err)
		}
	}

	recordRenderUpdate(ctx, span, counter, renderResultWritten)

	return nil
}

// recordRenderUpdate marks the render span's changed flag and counts the
// update by result. counter may be nil when the meter could not create it.
func recordRenderUpdate(ctx context.Context, span trace.Span, counter otelmetric.Int64Counter, result string) {
	span.SetAttributes(attribute.Bool("dashboard.render.changed", result == renderResultWritten))

	if counter == nil {
		return
	}

	counter.Add(ctx, 1, otelmetric.WithAttributes(
		attribute.String("dashboard.render.result", result),
	))
}

// checkDataUnchanged checks if the data hash matches the stored hash.
func checkDataUnchanged(outputPath, dataHash string) (bool, error) {
	hashPath := outputPath + ".hash"

	existingHash, err := os.ReadFile(hashPath) // #nosec G304 -- Path validated by validateOutputPath
	if err != nil {
		return false, err
	}

	if string(existingHash) == dataHash {
		slog.Info("Data unchanged, skipping render", "hash", dataHash[:12], "output_path", outputPath)

		return true, nil
	}

	return false, nil
}

// saveDataHash saves the data hash to a file.
func saveDataHash(outputPath, dataHash string) error {
	hashPath := outputPath + ".hash"
	// #nosec G306 -- readable permissions required for hash file
	return os.WriteFile(hashPath, []byte(dataHash), 0o644)
}

// touchHashFile updates the modification time of the hash file.
// This is used to keep healthchecks happy when data is unchanged.
func touchHashFile(outputPath string) error {
	hashPath := outputPath + ".hash"
	now := time.Now()

	return os.Chtimes(hashPath, now, now)
}

// logHashError distinguishes the expected first-render case (no previous
// hash on disk yet) from real hash-check failures.
func logHashError(outputPath string, hashErr error) {
	if errors.Is(hashErr, os.ErrNotExist) {
		// First render (fresh output dir): nothing to compare against.
		slog.Debug("No previous hash, rendering fresh")

		return
	}

	slog.Warn("Failed to check hash file", "output_path", outputPath, "error", hashErr)
}

// touchHashFileAndReturn touches the hash file and returns nil, or logs and returns error.
func touchHashFileAndReturn(outputPath string) error {
	// Touch hash file to update timestamp for healthchecks
	// (we don't touch the image file to preserve serving 304s)
	if err := touchHashFile(outputPath); err != nil {
		slog.Warn("Failed to update hash file timestamp", "error", err)
	}

	return nil
}

// ImageBytes takes HTML and CSS content and renders it to JPEG bytes.
// This function does not write to disk, allowing callers to inspect or compare the output.
func ImageBytes(ctx context.Context, html, css string, config *ImageConfig) ([]byte, error) {
	fullHTML := buildFullHTML(html, css)

	buf, err := renderHTMLToImage(ctx, fullHTML, config)
	if err != nil {
		return nil, err
	}

	// Convert PNG screenshot to JPEG
	buf, err = convertPNGToJPEG(buf, config.JPEGQuality)
	if err != nil {
		return nil, fmt.Errorf("converting to JPEG: %w", err)
	}

	// Rotate if needed
	if config.Rotate != 0 {
		buf, err = rotateImage(buf, config.JPEGQuality)
		if err != nil {
			return nil, fmt.Errorf("rotating image: %w", err)
		}
	}

	return buf, nil
}

// buildFullHTML creates a complete HTML document with preload hints.
func buildFullHTML(html, css string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <!-- Preload critical font resources for faster loading -->
    <link rel="preconnect" href="https://fonts.googleapis.com" crossorigin>
    <link rel="preconnect" href="https://cdnjs.cloudflare.com" crossorigin>
    <link href="https://fonts.googleapis.com/css2?family=Lato:wght@400;700&display=swap" rel="stylesheet">
    <style>%s</style>
</head>
<body>
%s
</body>
</html>`, css, html)
}

// renderHTMLToImage renders HTML to a screenshot using Chrome.
func renderHTMLToImage(ctx context.Context, fullHTML string, config *ImageConfig) ([]byte, error) {
	opts := getChromeOptions(config)

	slog.Info("Starting Chrome", "user_data_dir", "/tmp/chrome-data")

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	taskCtx, taskCancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(chromedpLogf))
	defer taskCancel()

	// Set timeout
	taskCtx, cancel := context.WithTimeout(taskCtx, 30*time.Second)
	defer cancel()

	// Set defaults for font loading if not configured
	fontTimeout := config.FontLoadTimeout
	if fontTimeout == 0 {
		fontTimeout = 5 * time.Second
	}

	maxRetries := config.FontLoadMaxRetries
	if maxRetries == 0 {
		maxRetries = 5
	}

	if err := chromedp.Do(taskCtx,
		chromedp.Navigate("about:blank"),
		chromedp.Evaluate[chromedp.Void](`document.write(`+jsonString(fullHTML)+`); document.close();`),
		waitForFontsToLoad(fontTimeout, maxRetries),
	); err != nil {
		return nil, fmt.Errorf("rendering page: %w", err)
	}

	buf, err := chromedp.Run(taskCtx, chromedp.FullScreenshot(100))
	if err != nil {
		return nil, fmt.Errorf("taking screenshot: %w", err)
	}

	return buf, nil
}

// getChromeOptions returns Chrome options for headless rendering.
func getChromeOptions(config *ImageConfig) []chromedp.ExecAllocatorOption {
	return []chromedp.ExecAllocatorOption{
		chromedp.NoFirstRun,
		chromedp.NoDefaultBrowserCheck,
		chromedp.Flag("headless", "new"),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-sandbox", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("disable-software-rasterizer", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("hide-scrollbars", true),
		chromedp.Flag("mute-audio", true),
		chromedp.Flag("lang", "en"),
		// Chrome 128+ crashpad fixes - completely disable crashpad to avoid EAGAIN
		chromedp.Flag("disable-crash-reporter", true),
		chromedp.Flag("disable-breakpad", true),
		chromedp.Flag("enable-crash-reporter", false),
		chromedp.Flag("crash-dumps-dir", "/dev/null"),
		chromedp.Flag("single-process", false), // Multi-process but limit spawning
		chromedp.Flag("disable-setuid-sandbox", true),
		chromedp.Env("CHROME_CRASHPAD_PIPE_NAME=", "BREAKPAD_DUMP_LOCATION=/dev/null"),
		chromedp.Env("XDG_CONFIG_HOME=/tmp/chrome-data", "XDG_CACHE_HOME=/tmp/chrome-data"),
		chromedp.UserDataDir("/tmp/chrome-data"),
		chromedp.WindowSize(config.Width, config.Height),
	}
}

// writeImageFile writes image data to a file atomically.
func writeImageFile(buf []byte, outputPath string) error {
	dir := filepath.Dir(outputPath)

	tempFile, err := os.CreateTemp(dir, "dashboard-*.jpg")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}

	tempPath := tempFile.Name()

	if _, err := tempFile.Write(buf); err != nil {
		//nolint:errcheck // Best effort cleanup
		_ = tempFile.Close()
		//nolint:errcheck // Best effort cleanup
		_ = os.Remove(tempPath) // #nosec G703 -- tempPath from os.CreateTemp, not user input

		return fmt.Errorf("writing temp file: %w", err)
	}

	//nolint:errcheck // Error not relevant after successful write
	_ = tempFile.Close()

	//nolint:gosec // 644 permissions required for external access
	if err := os.Chmod(tempPath, 0o644); err != nil {
		//nolint:errcheck // Best effort cleanup
		_ = os.Remove(tempPath) // #nosec G703 -- tempPath from os.CreateTemp, not user input

		return fmt.Errorf("setting file permissions: %w", err)
	}

	if err := os.Rename(tempPath, outputPath); err != nil { // #nosec G703 -- Path validated by validateOutputPath
		//nolint:errcheck // Best effort cleanup
		_ = os.Remove(tempPath) // #nosec G703 -- tempPath from os.CreateTemp, not user input

		return fmt.Errorf("renaming temp file: %w", err)
	}

	return nil
}

// convertPNGToJPEG converts PNG data to JPEG format.
func convertPNGToJPEG(pngData []byte, quality int) ([]byte, error) {
	// Set default quality
	if quality == 0 {
		quality = 85
	}

	// Decode PNG
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return nil, fmt.Errorf("decoding PNG: %w", err)
	}

	// Encode as JPEG
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, fmt.Errorf("encoding JPEG: %w", err)
	}

	return buf.Bytes(), nil
}

// waitForFontsToLoad waits for all fonts to be loaded with retry logic.
// It checks document.fonts.ready and retries with exponential backoff.
// Also detects network errors and logs warnings.
func waitForFontsToLoad(timeout time.Duration, maxRetries int) chromedp.Action[chromedp.Void] {
	return chromedp.Func(func(ctx context.Context, _ *chromedp.Target) error {
		baseDelay := max(timeout/time.Duration(maxRetries*2), 100*time.Millisecond)

		start := time.Now()
		for attempt := range maxRetries {
			// Check if we've exceeded timeout
			if time.Since(start) > timeout {
				slog.Warn("Font loading timeout exceeded, proceeding anyway", "timeout", timeout)
				checkAndLogNetworkErrors(ctx)

				return nil
			}

			// Check if fonts are ready using document.fonts API
			// Use synchronous status check - document.fonts.ready is a Promise we can't easily await here
			fontsReady, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`
			(() => {
				try {
					// Check if document.fonts.status is 'loaded'
					// This is synchronous and doesn't require awaiting the promise
					return document.fonts.status === 'loaded';
				} catch (e) {
					console.error('Font loading check failed:', e);
					return false;
				}
			})()
		`))
			if err != nil {
				slog.Warn("Failed to check font loading status", "attempt", attempt+1, "max_attempts", maxRetries, "error", err)
			} else if fontsReady {
				slog.Info("Fonts loaded", "attempts", attempt+1, "elapsed", time.Since(start))

				return nil
			}

			// Exponential backoff
			// #nosec G115 -- attempt is bounded by maxRetries config value, overflow not possible in practice
			delay := min(baseDelay*time.Duration(1<<uint(attempt)), timeout/2)

			if attempt < maxRetries-1 {
				slog.Info("Waiting for fonts to load", "retry_in", delay, "attempt", attempt+1, "max_attempts", maxRetries)
				time.Sleep(delay)
			}
		}

		slog.Warn("Fonts may not be fully loaded, proceeding anyway", "max_attempts", maxRetries)
		checkAndLogNetworkErrors(ctx)

		return nil // Don't fail, just warn
	})
}

// chromedpLogf routes chromedp's internal logging (CDP protocol traffic,
// page console messages) to slog's debug level, keeping default-level
// output limited to real events instead of protocol chatter.
func chromedpLogf(format string, args ...any) {
	slog.Debug(fmt.Sprintf(format, args...))
}

// checkAndLogNetworkErrors checks for network-related errors in the console
// and logs them as warnings to help diagnose font loading issues.
func checkAndLogNetworkErrors(ctx context.Context) {
	// Check for failed resource loads (Font Awesome only - Weather Icons are embedded)
	hasNetworkErrors, err := chromedp.Run(ctx, chromedp.Evaluate[bool](`
		(() => {
			const resources = performance.getEntriesByType('resource');
			const failed = resources.filter(r => 
				r.name.includes('fonts.googleapis') &&
				r.transferSize === 0 && r.decodedBodySize === 0
			);
			return failed.length > 0;
		})()
	`))
	if err != nil {
		slog.Warn("Failed to check for network errors", "error", err)

		return
	}

	if hasNetworkErrors {
		slog.Warn("Network errors detected, Font Awesome may have failed to load")
	}
}

// jsonString escapes a string for use in JavaScript.
func jsonString(s string) string {
	var builder strings.Builder

	builder.WriteByte('`')

	for _, char := range s {
		switch char {
		case '`':
			builder.WriteString("\\`")
		case '\\':
			builder.WriteString("\\\\")
		case '$':
			builder.WriteString("\\$")
		default:
			builder.WriteRune(char)
		}
	}

	builder.WriteByte('`')

	return builder.String()
}

// rotateImage rotates a JPEG image by 270 degrees clockwise.
func rotateImage(data []byte, quality int) ([]byte, error) {
	// Set default quality
	if quality == 0 {
		quality = 85
	}

	// Decode the JPEG image
	src, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decoding JPEG: %w", err)
	}

	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// For 270 degree rotation: new dimensions are swapped
	// 270 degrees clockwise = 90 degrees counter-clockwise
	dst := image.NewRGBA(image.Rect(0, 0, height, width))

	// Rotate 270 degrees clockwise:
	// dst(x, y) = src(y, width - 1 - x)
	for y := range height {
		for x := range width {
			dst.Set(y, width-1-x, src.At(x+bounds.Min.X, y+bounds.Min.Y))
		}
	}

	// Encode back to JPEG
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: quality}); err != nil {
		return nil, fmt.Errorf("encoding JPEG: %w", err)
	}

	return buf.Bytes(), nil
}
