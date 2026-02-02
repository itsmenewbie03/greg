package watchparty

import (
	"fmt"
	"net/url"
)

// ParseAndValidateURL parses and validates a URL string
func ParseAndValidateURL(rawURL string) (*url.URL, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL format: %w", err)
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return nil, fmt.Errorf("URL must use http or https scheme")
	}

	if parsedURL.Host == "" {
		return nil, fmt.Errorf("URL must have a host")
	}

	return parsedURL, nil
}

// NormalizeURL ensures the URL has proper formatting
func NormalizeURL(inputURL string) (string, error) {
	parsed, err := ParseAndValidateURL(inputURL)
	if err != nil {
		return "", err
	}

	// Ensure URL ends with a slash if needed by proxy services
	result := parsed.String()
	return result, nil
}

// EncodeURLParameter safely encodes a URL parameter
func EncodeURLParameter(value string) string {
	return url.QueryEscape(value)
}
