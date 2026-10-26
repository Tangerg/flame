package mcp

import (
	"errors"
	"net/http"
	"testing"
)

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
			err = checkRedirect(target, []*http.Request{initial})
			if errors.Is(err, errCrossOriginRedirect) != test.blocked {
				t.Fatalf("checkRedirect error = %v, blocked = %t", err, test.blocked)
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
	if err := checkRedirect(request, via); err == nil {
		t.Fatal("checkRedirect accepted an overlong redirect chain")
	}
}
