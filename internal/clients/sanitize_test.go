package clients

import (
	"strings"
	"testing"
)

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
