package render

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
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
	t.Run("rotates 2x3 image to 3x2", func(t *testing.T) {
		// Create a 2x3 test image (width=2, height=3)
		// Pixels arranged as:
		// [R, G]
		// [B, Y]
		// [W, K]
		src := image.NewRGBA(image.Rect(0, 0, 2, 3))
		red := color.RGBA{255, 0, 0, 255}
		green := color.RGBA{0, 255, 0, 255}
		blue := color.RGBA{0, 0, 255, 255}
		yellow := color.RGBA{255, 255, 0, 255}
		white := color.RGBA{255, 255, 255, 255}
		black := color.RGBA{0, 0, 0, 255}

		src.Set(0, 0, red)
		src.Set(1, 0, green)
		src.Set(0, 1, blue)
		src.Set(1, 1, yellow)
		src.Set(0, 2, white)
		src.Set(1, 2, black)

		// Encode to PNG
		var buf bytes.Buffer
		if err := png.Encode(&buf, src); err != nil {
			t.Fatalf("failed to encode test image: %v", err)
		}

		// Rotate
		result := rotateImage(buf.Bytes())

		// Decode result
		dst, err := png.Decode(bytes.NewReader(result))
		if err != nil {
			t.Fatalf("failed to decode rotated image: %v", err)
		}

		// After 270 degree clockwise rotation, dimensions should be swapped
		bounds := dst.Bounds()
		if bounds.Dx() != 3 || bounds.Dy() != 2 {
			t.Errorf("rotated image dimensions = %dx%d, want 3x2", bounds.Dx(), bounds.Dy())
		}

		// After 270 degree clockwise rotation:
		// Original:     Rotated 270 CW:
		// [R, G]        [G, Y, K]
		// [B, Y]   =>   [R, B, W]
		// [W, K]
		checkPixel := func(x, y int, expected color.RGBA, name string) {
			got := dst.At(x, y)
			r, g, b, a := got.RGBA()
			er, eg, eb, ea := expected.RGBA()
			if r != er || g != eg || b != eb || a != ea {
				t.Errorf("pixel (%d,%d) %s: got RGBA(%d,%d,%d,%d), want RGBA(%d,%d,%d,%d)",
					x, y, name, r>>8, g>>8, b>>8, a>>8, er>>8, eg>>8, eb>>8, ea>>8)
			}
		}

		// Top row: G, Y, K
		checkPixel(0, 0, green, "green")
		checkPixel(1, 0, yellow, "yellow")
		checkPixel(2, 0, black, "black")

		// Bottom row: R, B, W
		checkPixel(0, 1, red, "red")
		checkPixel(1, 1, blue, "blue")
		checkPixel(2, 1, white, "white")
	})

	t.Run("returns original on invalid PNG", func(t *testing.T) {
		invalidData := []byte("not a valid png")
		result := rotateImage(invalidData)

		if !bytes.Equal(result, invalidData) {
			t.Errorf("should return original data for invalid PNG")
		}
	})

	t.Run("handles 1x1 image", func(t *testing.T) {
		src := image.NewRGBA(image.Rect(0, 0, 1, 1))
		src.Set(0, 0, color.RGBA{128, 128, 128, 255})

		var buf bytes.Buffer
		if err := png.Encode(&buf, src); err != nil {
			t.Fatalf("failed to encode test image: %v", err)
		}

		result := rotateImage(buf.Bytes())

		dst, err := png.Decode(bytes.NewReader(result))
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
		Width:      800,
		Height:     600,
		Rotate:     90,
		OutputPath: "/tmp/test.png",
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

	if config.OutputPath != "/tmp/test.png" {
		t.Errorf("OutputPath = %q, want %q", config.OutputPath, "/tmp/test.png")
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
			path:    "/output/dashboard.png",
			wantErr: false,
		},
		{
			name:    "valid relative path",
			path:    "output/dashboard.png",
			wantErr: false,
		},
		{
			name:    "rejects path traversal with ..",
			path:    "/output/../../../etc/passwd",
			wantErr: true,
		},
		{
			name:    "rejects sneaky path traversal",
			path:    "/output/dashboard.png/../../../etc/passwd",
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
