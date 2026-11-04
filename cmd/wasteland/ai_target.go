package main

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"strings"
)

const defaultAIAPIURL = "https://api.openai.com/v1/chat/completions"

// aiAPIURL validates the configured endpoint against built-in provider hosts
// and exact additional hostnames in AI_ALLOWED_HOSTS (comma-separated).
func aiAPIURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = defaultAIAPIURL
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Hostname() == "" || (u.Port() != "" && u.Port() != "443") {
		return "", errors.New("AI endpoint must be an allowlisted HTTPS URL")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	allowed := map[string]bool{"api.openai.com": true, "api.deepseek.com": true}
	for _, entry := range strings.Split(os.Getenv("AI_ALLOWED_HOSTS"), ",") {
		entry = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(entry, ".")))
		if entry != "" {
			allowed[entry] = true
		}
	}
	if !allowed[host] {
		return "", errors.New("AI endpoint host is not allowlisted")
	}
	return u.String(), nil
}

func rejectAIRedirect(_ *http.Request, _ []*http.Request) error {
	return http.ErrUseLastResponse
}
