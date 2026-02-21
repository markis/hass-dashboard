// Package render provides HTML template rendering and image generation functionality.
package render

import (
	"bytes"
	"context"
	"fmt"
	"image"
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
	Width      int
	Height     int
	Rotate     int
	OutputPath string
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

// Image takes HTML and CSS content and renders it to a PNG image.
func Image(ctx context.Context, html, css string, config ImageConfig) error {
	// Validate output path to prevent path traversal
	if err := validateOutputPath(config.OutputPath); err != nil {
		return err
	}
	// Create a full HTML document
	fullHTML := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <link href="https://fonts.googleapis.com/css2?family=Lato:wght@400;700&display=swap" rel="stylesheet">
    <style>%s</style>
</head>
<body>
%s
</body>
</html>`, css, html)

	// Set up chromedp options - use new headless mode to avoid crashpad issues
	opts := []chromedp.ExecAllocatorOption{
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

	log.Printf("Starting Chrome with user data dir: /tmp/chrome-data")

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	taskCtx, taskCancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	defer taskCancel()

	// Set timeout
	taskCtx, cancel := context.WithTimeout(taskCtx, 30*time.Second)
	defer cancel()

	var buf []byte

	if err := chromedp.Run(taskCtx,
		chromedp.Navigate("about:blank"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.Evaluate(`document.write(`+jsonString(fullHTML)+`); document.close();`, nil).Do(ctx)
		}),
		chromedp.Sleep(500*time.Millisecond), // Wait for fonts to load
		chromedp.FullScreenshot(&buf, 100),
	); err != nil {
		return fmt.Errorf("taking screenshot: %w", err)
	}

	// Rotate if needed
	if config.Rotate != 0 {
		buf = rotateImage(buf)
	}

	// Write to temp file first, then rename for atomic write
	dir := filepath.Dir(config.OutputPath)

	tempFile, err := os.CreateTemp(dir, "dashboard-*.png")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}

	tempPath := tempFile.Name()

	if _, err := tempFile.Write(buf); err != nil {
		//nolint:errcheck // Best effort cleanup
		_ = tempFile.Close()
		//nolint:errcheck // Best effort cleanup
		_ = os.Remove(tempPath)

		return fmt.Errorf("writing temp file: %w", err)
	}

	//nolint:errcheck // Error not relevant after successful write
	_ = tempFile.Close()

	//nolint:gosec // 644 permissions required for external access
	if err := os.Chmod(tempPath, 0o644); err != nil {
		//nolint:errcheck // Best effort cleanup
		_ = os.Remove(tempPath)

		return fmt.Errorf("setting file permissions: %w", err)
	}

	if err := os.Rename(tempPath, config.OutputPath); err != nil {
		//nolint:errcheck // Best effort cleanup
		_ = os.Remove(tempPath)

		return fmt.Errorf("renaming temp file: %w", err)
	}

	return nil
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

// rotateImage rotates a PNG image by 270 degrees clockwise.
func rotateImage(data []byte) []byte {
	// Decode the PNG image
	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return data // Return original on error
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

	// Encode back to PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return data // Return original on error
	}

	return buf.Bytes()
}
