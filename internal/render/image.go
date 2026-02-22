// Package render provides HTML template rendering and image generation functionality.
package render

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// ImageConfig holds configuration for image rendering.
type ImageConfig struct {
	Width              int
	Height             int
	Rotate             int
	OutputPath         string
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

// Image takes HTML and CSS content and renders it to a JPEG image.
func Image(ctx context.Context, html, css string, config ImageConfig) error {
	// Validate output path to prevent path traversal
	if err := validateOutputPath(config.OutputPath); err != nil {
		return err
	}

	fullHTML := buildFullHTML(html, css)

	buf, err := renderHTMLToImage(ctx, fullHTML, config)
	if err != nil {
		return err
	}

	// Convert PNG screenshot to JPEG
	buf, err = convertPNGToJPEG(buf, config.JPEGQuality)
	if err != nil {
		return fmt.Errorf("converting to JPEG: %w", err)
	}

	// Rotate if needed
	if config.Rotate != 0 {
		buf, err = rotateImage(buf, config.JPEGQuality)
		if err != nil {
			return fmt.Errorf("rotating image: %w", err)
		}
	}

	return writeImageFile(buf, config.OutputPath)
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
func renderHTMLToImage(ctx context.Context, fullHTML string, config ImageConfig) ([]byte, error) {
	opts := getChromeOptions(config)

	log.Printf("Starting Chrome with user data dir: /tmp/chrome-data")

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	taskCtx, taskCancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	defer taskCancel()

	// Set timeout
	taskCtx, cancel := context.WithTimeout(taskCtx, 30*time.Second)
	defer cancel()

	var buf []byte

	// Set defaults for font loading if not configured
	fontTimeout := config.FontLoadTimeout
	if fontTimeout == 0 {
		fontTimeout = 5 * time.Second
	}

	maxRetries := config.FontLoadMaxRetries
	if maxRetries == 0 {
		maxRetries = 5
	}

	if err := chromedp.Run(taskCtx,
		chromedp.Navigate("about:blank"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.Evaluate(`document.write(`+jsonString(fullHTML)+`); document.close();`, nil).Do(ctx)
		}),
		waitForFontsToLoad(fontTimeout, maxRetries),
		chromedp.FullScreenshot(&buf, 100),
	); err != nil {
		return nil, fmt.Errorf("taking screenshot: %w", err)
	}

	return buf, nil
}

// getChromeOptions returns Chrome options for headless rendering.
func getChromeOptions(config ImageConfig) []chromedp.ExecAllocatorOption {
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
func waitForFontsToLoad(timeout time.Duration, maxRetries int) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		baseDelay := max(timeout/time.Duration(maxRetries*2), 100*time.Millisecond)

		start := time.Now()
		for attempt := range maxRetries {
			// Check if we've exceeded timeout
			if time.Since(start) > timeout {
				log.Printf("Warning: Font loading timeout exceeded after %v, proceeding anyway", timeout)
				checkAndLogNetworkErrors(ctx)

				return nil
			}

			var fontsReady bool

			// Check if fonts are ready using document.fonts API
			err := chromedp.Evaluate(`
				(async () => {
					try {
						await document.fonts.ready;
						return document.fonts.status === 'loaded';
					} catch (e) {
						console.error('Font loading check failed:', e);
						return false;
					}
				})()
			`, &fontsReady).Do(ctx)
			if err != nil {
				log.Printf("Warning: Failed to check font loading status (attempt %d/%d): %v", attempt+1, maxRetries, err)
			} else if fontsReady {
				log.Printf("Fonts loaded successfully after %d attempt(s) in %v", attempt+1, time.Since(start))

				return nil
			}

			// Exponential backoff
			// #nosec G115 -- attempt is bounded by maxRetries config value, overflow not possible in practice
			delay := min(baseDelay*time.Duration(1<<uint(attempt)), timeout/2)

			if attempt < maxRetries-1 {
				log.Printf("Waiting for fonts to load, retrying in %v (attempt %d/%d)", delay, attempt+1, maxRetries)
				time.Sleep(delay)
			}
		}

		log.Printf("Warning: Fonts may not be fully loaded after %d retries, proceeding anyway", maxRetries)
		checkAndLogNetworkErrors(ctx)

		return nil // Don't fail, just warn
	}
}

// checkAndLogNetworkErrors checks for network-related errors in the console
// and logs them as warnings to help diagnose font loading issues.
func checkAndLogNetworkErrors(ctx context.Context) {
	var hasNetworkErrors bool

	// Check for failed resource loads
	err := chromedp.Evaluate(`
		(async () => {
			const resources = performance.getEntriesByType('resource');
			const failed = resources.filter(r => 
				(r.name.includes('weather-icons') || r.name.includes('fonts.googleapis')) &&
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
			"WARNING: Network errors detected - CDN resources (fonts/icons) may have failed to load. " +
				"Check network connectivity.",
		)
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
