package render

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

func TestJsonString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple string",
			input: "hello",
			want:  "`hello`",
		},
		{
			name:  "with backtick",
			input: "hello`world",
			want:  "`hello\\`world`",
		},
		{
			name:  "with backslash",
			input: "hello\\world",
			want:  "`hello\\\\world`",
		},
		{
			name:  "with dollar sign",
			input: "hello$world",
			want:  "`hello\\$world`",
		},
		{
			name:  "with all special chars",
			input: "`$\\",
			want:  "`\\`\\$\\\\`",
		},
		{
			name:  "empty string",
			input: "",
			want:  "``",
		},
		{
			name:  "html content",
			input: "<html><body>Test</body></html>",
			want:  "`<html><body>Test</body></html>`",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := jsonString(tt.input)
			if got != tt.want {
				t.Errorf("jsonString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestRotateImage(t *testing.T) {
	t.Run("rotates image dimensions correctly", func(t *testing.T) {
		// Create a 2x3 test image (width=2, height=3) with grayscale patterns
		// JPEG handles grayscale better than saturated colors in tiny images
		src := image.NewRGBA(image.Rect(0, 0, 2, 3))

		// Fill with different gray levels to verify rotation
		for y := range 3 {
			for x := range 2 {
				// #nosec G115 -- small test values (0-5 * 50 = 0-250), no overflow possible
				level := uint8((x + y*2) * 50)
				src.Set(x, y, color.RGBA{level, level, level, 255})
			}
		}

		// Encode to JPEG
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: 100}); err != nil {
			t.Fatalf("failed to encode test image: %v", err)
		}

		// Rotate
		result, err := rotateImage(buf.Bytes(), 100)
		if err != nil {
			t.Fatalf("failed to rotate image: %v", err)
		}

		// Decode result
		dst, err := jpeg.Decode(bytes.NewReader(result))
		if err != nil {
			t.Fatalf("failed to decode rotated image: %v", err)
		}

		// After 270 degree clockwise rotation, dimensions should be swapped
		bounds := dst.Bounds()
		if bounds.Dx() != 3 || bounds.Dy() != 2 {
			t.Errorf("rotated image dimensions = %dx%d, want 3x2", bounds.Dx(), bounds.Dy())
		}
	})

	t.Run("returns error on invalid JPEG", func(t *testing.T) {
		invalidData := []byte("not a valid jpeg")
		_, err := rotateImage(invalidData, 85)

		if err == nil {
			t.Errorf("should return error for invalid JPEG")
		}
	})

	t.Run("handles 1x1 image", func(t *testing.T) {
		src := image.NewRGBA(image.Rect(0, 0, 1, 1))
		src.Set(0, 0, color.RGBA{128, 128, 128, 255})

		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, src, &jpeg.Options{Quality: 95}); err != nil {
			t.Fatalf("failed to encode test image: %v", err)
		}

		result, err := rotateImage(buf.Bytes(), 95)
		if err != nil {
			t.Fatalf("failed to rotate image: %v", err)
		}

		dst, err := jpeg.Decode(bytes.NewReader(result))
		if err != nil {
			t.Fatalf("failed to decode rotated image: %v", err)
		}

		bounds := dst.Bounds()
		if bounds.Dx() != 1 || bounds.Dy() != 1 {
			t.Errorf("1x1 rotated image dimensions = %dx%d, want 1x1", bounds.Dx(), bounds.Dy())
		}
	})
}

func TestImageConfig(t *testing.T) {
	config := ImageConfig{
		Width:       800,
		Height:      600,
		Rotate:      90,
		OutputPath:  "/tmp/test.jpg",
		JPEGQuality: 90,
	}

	if config.Width != 800 {
		t.Errorf("Width = %d, want 800", config.Width)
	}

	if config.Height != 600 {
		t.Errorf("Height = %d, want 600", config.Height)
	}

	if config.Rotate != 90 {
		t.Errorf("Rotate = %d, want 90", config.Rotate)
	}

	if config.OutputPath != "/tmp/test.jpg" {
		t.Errorf("OutputPath = %q, want %q", config.OutputPath, "/tmp/test.jpg")
	}

	if config.JPEGQuality != 90 {
		t.Errorf("JPEGQuality = %d, want 90", config.JPEGQuality)
	}
}

func TestValidateOutputPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{
			name:    "valid absolute path",
			path:    "/output/dashboard.jpg",
			wantErr: false,
		},
		{
			name:    "valid relative path",
			path:    "output/dashboard.jpg",
			wantErr: false,
		},
		{
			name:    "rejects path traversal with ..",
			path:    "/output/../../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "rejects sneaky path traversal",
			path:    "/output/dashboard.jpg/../../../etc/passwd",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOutputPath(tt.path)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateOutputPath(%q) error = %v, wantErr %v", tt.path, err, tt.wantErr)
			}
		})
	}
}
