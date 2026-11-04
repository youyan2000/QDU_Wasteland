package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHSTSDefaultsToCurrentHostAndCanOptIntoSubdomains(t *testing.T) {
	t.Setenv("HSTS_INCLUDE_SUBDOMAINS", "")
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "https://example.invalid/", nil))
	if got := rr.Header().Get("Strict-Transport-Security"); got != "max-age=31536000" {
		t.Fatalf("unexpected default HSTS: %q", got)
	}
	t.Setenv("HSTS_INCLUDE_SUBDOMAINS", "true")
	rr = httptest.NewRecorder()
	handler = securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "https://example.invalid/", nil))
	if got := rr.Header().Get("Strict-Transport-Security"); got != "max-age=31536000; includeSubDomains" {
		t.Fatalf("unexpected opted-in HSTS: %q", got)
	}
}
