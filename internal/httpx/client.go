package httpx

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func NewClient(timeout time.Duration, insecureSkipVerify bool) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if insecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}

func NormalizeBaseURL(raw string) (string, error) {
	parsed, err := parseHTTPURL(raw)
	if err != nil {
		return "", err
	}
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func NormalizeRequestURL(raw string) (string, error) {
	parsed, err := parseHTTPURL(raw)
	if err != nil {
		return "", err
	}
	return parsed.String(), nil
}

func parseHTTPURL(raw string) (*url.URL, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("url is required")
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return nil, fmt.Errorf("parse url %q: %w", raw, err)
	}

	switch parsed.Scheme {
	case "http", "https":
	default:
		if parsed.Scheme == "" {
			return nil, fmt.Errorf("url must include scheme http:// or https://: %s", raw)
		}
		return nil, fmt.Errorf("unsupported url scheme %q in %s", parsed.Scheme, raw)
	}

	if parsed.Host == "" {
		return nil, fmt.Errorf("url must include host: %s", raw)
	}
	return parsed, nil
}
