package main

import "testing"

func TestAIAPIURLAcceptsDefaultProvidersAndExplicitHost(t *testing.T) {
	t.Setenv("AI_ALLOWED_HOSTS", "gateway.example.net,other.example.net")
	for _, raw := range []string{"", "https://api.openai.com/v1/chat/completions", "https://api.deepseek.com/v1/chat/completions", "https://gateway.example.net/v1/chat/completions"} {
		if _, err := aiAPIURL(raw); err != nil {
			t.Errorf("aiAPIURL(%q): %v", raw, err)
		}
	}
}
