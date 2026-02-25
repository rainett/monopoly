package auth

import (
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

var policy = bluemonday.StrictPolicy()

// SanitizeUsername removes any HTML and trims whitespace from username
func SanitizeUsername(username string) string {
	// Remove any HTML tags
	cleaned := policy.Sanitize(username)
	// Trim whitespace
	cleaned = strings.TrimSpace(cleaned)
	return cleaned
}

// SanitizeString is a generic sanitizer for user input
func SanitizeString(input string) string {
	cleaned := policy.Sanitize(input)
	cleaned = strings.TrimSpace(cleaned)
	return cleaned
}
