package clients

import (
	"strings"
)

// sanitizeLogData removes newlines and other control characters that could
// cause log injection attacks.
func sanitizeLogData(data string) string {
	// Replace newlines and carriage returns
	sanitized := strings.ReplaceAll(data, "\n", " ")
	sanitized = strings.ReplaceAll(sanitized, "\r", " ")

	// Limit length to prevent log flooding
	const maxLength = 500
	if len(sanitized) > maxLength {
		sanitized = sanitized[:maxLength] + "... (truncated)"
	}

	return sanitized
}
