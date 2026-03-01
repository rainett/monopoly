package auth

import (
	"strings"

	"github.com/microcosm-cc/bluemonday"
)

var policy = bluemonday.StrictPolicy()

// SanitizeString is a generic sanitizer for user input
func SanitizeString(input string) string {
	cleaned := policy.Sanitize(input)
	cleaned = strings.TrimSpace(cleaned)
	return cleaned
}
