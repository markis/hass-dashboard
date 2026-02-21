package clients

import (
	"net/url"
	"strings"
)

// sanitizeURL removes sensitive information from URLs for safe logging.
// Redacts passwords, tokens from query params, and ensures no newlines.
func sanitizeURL(rawURL string) string {
	// Remove any newlines that could cause log injection
	sanitized := strings.ReplaceAll(rawURL, "\n", "")
	sanitized = strings.ReplaceAll(sanitized, "\r", "")

	// Parse URL to redact sensitive parts
	parsedURL, err := url.Parse(sanitized)
	if err != nil {
		// If we can't parse it, just return the cleaned string
		return sanitized
	}

	// Redact password if present
	if parsedURL.User != nil {
		if _, hasPassword := parsedURL.User.Password(); hasPassword {
			parsedURL.User = url.UserPassword(parsedURL.User.Username(), "[REDACTED]")
		}
	}

	// Redact sensitive query parameters
	if parsedURL.RawQuery != "" {
		query := parsedURL.Query()
		for key := range query {
			lowerKey := strings.ToLower(key)
			if strings.Contains(lowerKey, "token") ||
				strings.Contains(lowerKey, "key") ||
				strings.Contains(lowerKey, "secret") ||
				strings.Contains(lowerKey, "password") {
				query.Set(key, "[REDACTED]")
			}
		}

		parsedURL.RawQuery = query.Encode()
	}

	return parsedURL.String()
}

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
