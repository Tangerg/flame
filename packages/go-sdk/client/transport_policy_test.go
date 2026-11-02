package client

import (
	"strings"
	"testing"
)

func TestRemoteCredentialsAndResponseBudget(t *testing.T) {
	for _, test := range []struct {
		target, token string
		allowed       bool
	}{
		{"http://example.test", "", true},
		{"http://example.test", "example", false},
		{"https://example.test", "example", true},
		{"http://127.0.0.1:8080", "example", true},
	} {
		c, err := Connect(t.Context(), RemoteConfig{Endpoint: test.target, Token: test.token})
		if (err == nil) != test.allowed {
			t.Fatalf("%s allowed = %t, error %v", test.target, test.allowed, err)
		}
		if c != nil {
			if c.maxMessageBytes <= 0 {
				t.Fatal("discovery is unbounded")
			}
			_ = c.Close()
		}
	}
	if _, err := readRemoteMessage(strings.NewReader("1234"), 3); err == nil {
		t.Fatal("oversized document admitted")
	}
}
