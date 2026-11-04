package hotdigest

import "testing"

func TestNewEnricherUsesDefaultAndExplicitAllowedHosts(t *testing.T) {
	t.Setenv("AI_API_KEY", "unused-test-key")
	t.Setenv("AI_API_URL", "")
	t.Setenv("AI_ALLOWED_HOSTS", "")
	if e := NewEnricher("key"); e == nil || e.APIURL != "https://api.openai.com/v1/chat/completions" {
		t.Fatalf("default provider not preserved: %+v", e)
	}
	t.Setenv("AI_API_URL", "https://gateway.example.net/v1/chat/completions")
	t.Setenv("AI_ALLOWED_HOSTS", "gateway.example.net")
	if e := NewEnricher("key"); e == nil || e.APIURL != "https://gateway.example.net/v1/chat/completions" {
		t.Fatalf("explicit allowed provider not enabled: %+v", e)
	}
}
