package clients

import (
	"strings"
	"testing"
)

func TestSanitizeURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "removes newlines",
			input:    "http://example.com\nmalicious",
			expected: "http://example.commalicious",
		},
		{
			name:     "removes carriage returns",
			input:    "http://example.com\r\nmalicious",
			expected: "http://example.commalicious",
		},
		{ // #nosec G101 -- Test case for password redaction, not actual credentials
			name:     "redacts password",
			input:    "http://user:password@example.com",
			expected: "http://user:%5BREDACTED%5D@example.com",
		},
		{
			name:     "redacts token query param",
			input:    "http://example.com?token=secret123",
			expected: "http://example.com?token=%5BREDACTED%5D",
		},
		{
			name:     "redacts api_key query param",
			input:    "http://example.com?api_key=secret123",
			expected: "http://example.com?api_key=%5BREDACTED%5D",
		},
		{
			name:     "leaves safe URL unchanged",
			input:    "http://example.com/api/calendar?start=2023-01-01",
			expected: "http://example.com/api/calendar?start=2023-01-01",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeURL(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeURL(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSanitizeLogData(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "removes newlines",
			input:    "line1\nline2",
			expected: "line1 line2",
		},
		{
			name:     "removes carriage returns",
			input:    "line1\r\nline2",
			expected: "line1  line2",
		},
		{
			name:     "truncates long strings",
			input:    strings.Repeat("a", 600),
			expected: strings.Repeat("a", 500) + "... (truncated)",
		},
		{
			name:     "leaves short strings unchanged",
			input:    "Short message",
			expected: "Short message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sanitizeLogData(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeLogData() = %q, want %q", result, tt.expected)
			}
		})
	}
}
