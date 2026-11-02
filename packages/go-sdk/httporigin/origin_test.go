package httporigin

import (
	"encoding/json/v2"
	"errors"
	"net/url"
	"os"
	"strings"
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
		{"IPv6 default port", "http://[::1]/a", "http://[::1]:80/b", true},
		{"IPv6 different port", "http://[::1]/a", "http://[::1]:8080/b", false},
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

func TestParseFailureKeepsItsCauseWithoutTheCredentialBearingURL(t *testing.T) {
	_, err := Parse("https://user:private-password@example.test/%zz?key=private-token")
	if err == nil || strings.Contains(err.Error(), "private-password") || strings.Contains(err.Error(), "private-token") {
		t.Fatal("URL parse failure exposed credential-bearing input")
	}
	if !errors.Is(err, url.EscapeError("%zz")) {
		t.Fatalf("URL parse failure lost its syntax cause: %v", err)
	}
}

func TestParseEndpointOwnsTheSharedAdmissionRule(t *testing.T) {
	for _, raw := range []string{
		"ftp://example.com", "https://user@example.com", "https:opaque", "https://", "https://example.com:0",
		"https://example.com:65536", "https://example.com:http", "not a URL",
	} {
		if _, err := ParseEndpoint(raw); err == nil {
			t.Errorf("ParseEndpoint(%q) admitted an invalid endpoint", raw)
		}
	}
	endpoint, err := ParseEndpoint("HTTP://LocalHost:8080/v1/?#")
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.Secure() || !endpoint.Loopback() || endpoint.LoopbackIP() || !endpoint.HasQuery() || !endpoint.HasFragment() || endpoint.IsOrigin() {
		t.Fatalf("endpoint queries = secure %t loopback %t ip %t query %t fragment %t origin %t",
			endpoint.Secure(), endpoint.Loopback(), endpoint.LoopbackIP(), endpoint.HasQuery(), endpoint.HasFragment(), endpoint.IsOrigin())
	}
	origin, err := ParseEndpoint("https://[::1]:8443/")
	if err != nil || !origin.Secure() || !origin.LoopbackIP() || !origin.IsOrigin() || origin.Origin().String() != "https://[::1]:8443" {
		t.Fatalf("IPv6 loopback origin = %v, %v", origin.Origin(), err)
	}
}

func TestOriginCanonicalizesEquivalentIPAndPort(t *testing.T) {
	for _, pair := range [][2]string{{"https://example.org:00443/a", "https://example.org/b"}, {"https://[::1]", "https://[0:0:0:0:0:0:0:1]"}} {
		if !Same(pair[0], pair[1]) {
			t.Fatalf("equivalent origins %v", pair)
		}
	}
	if Same("https://[fe80::1%25ethA]:443", "https://[fe80::1%25etha]:443") {
		t.Fatal("collapsed zone identities")
	}
}

func TestRuntimeEndpointAdmission(t *testing.T) {
	data, err := os.ReadFile("testdata/runtime-endpoints.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Input    string `json:"input"`
		Accepted bool   `json:"accepted"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		endpoint, err := ParseEndpoint(tc.Input)
		accepted := err == nil && !endpoint.HasQuery() && !endpoint.HasFragment()
		if accepted != tc.Accepted {
			t.Errorf("endpoint %q accepted=%v, want %v", tc.Input, accepted, tc.Accepted)
		}
	}
}

func TestCanonicalBasePreservesZoneAndEscapedPath(t *testing.T) {
	endpoint, err := ParseEndpoint("https://[fe80::1%25en0]:443/a%2Fb/")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := endpoint.CanonicalBase(), "https://[fe80::1%25en0]:443/a%2Fb"; got != want {
		t.Fatalf("canonical base = %q, want %q", got, want)
	}
}
