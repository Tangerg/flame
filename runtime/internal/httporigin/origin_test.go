package httporigin

import (
	"errors"
	"net/http"
	"testing"
)

func TestSame(t *testing.T) {
	tests := []struct {
		name        string
		left, right string
		want        bool
	}{
		{"same origin different path", "https://EXAMPLE.com/a", "https://example.com/b", true},
		{"default HTTPS port", "https://example.com/a", "https://example.com:443/b", true},
		{"different host", "https://a.example/mcp", "https://b.example/mcp", false},
		{"different port", "https://example.com:8443/mcp", "https://example.com/mcp", false},
		{"different scheme", "http://example.com/mcp", "https://example.com/mcp", false},
		{"invalid endpoint", "not a URL", "not a URL", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Same(test.left, test.right); got != test.want {
				t.Fatalf("Same(%q, %q) = %v, want %v", test.left, test.right, got, test.want)
			}
		})
	}
}

func TestParseRejectsURLCredentials(t *testing.T) {
	if _, err := Parse("https://user:secret@example.com/mcp"); err == nil {
		t.Fatal("Parse err = nil, want URL credentials rejected")
	}
}

func TestCheckRedirectEnforcesInitialOrigin(t *testing.T) {
	initial, err := http.NewRequest(http.MethodGet, "https://EXAMPLE.com/start", nil)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		target  string
		blocked bool
	}{
		{name: "same origin", target: "https://example.com/next"},
		{name: "default port", target: "https://example.com:443/next"},
		{name: "different scheme", target: "http://example.com/next", blocked: true},
		{name: "different host", target: "https://other.example/next", blocked: true},
		{name: "different port", target: "https://example.com:8443/next", blocked: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target, err := http.NewRequest(http.MethodGet, test.target, nil)
			if err != nil {
				t.Fatal(err)
			}
			err = CheckRedirect(target, []*http.Request{initial})
			if errors.Is(err, ErrCrossOriginRedirect) != test.blocked {
				t.Fatalf("CheckRedirect error = %v, blocked = %t", err, test.blocked)
			}
		})
	}
}

func TestCheckRedirectBoundsTheChain(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "https://example.com/next", nil)
	if err != nil {
		t.Fatal(err)
	}
	via := make([]*http.Request, maximumRedirects)
	for index := range via {
		via[index] = request
	}
	if err := CheckRedirect(request, via); err == nil {
		t.Fatal("CheckRedirect accepted an overlong redirect chain")
	}
}
