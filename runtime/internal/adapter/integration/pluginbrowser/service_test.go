package pluginbrowser

import "testing"

func TestControlEndpointRequiresAnHTTPAuthorityOrSameOriginPath(t *testing.T) {
	for _, test := range []struct {
		endpoint string
		valid    bool
	}{
		{endpoint: "/control", valid: true},
		{endpoint: "https://localhost/control", valid: true},
		{endpoint: "http://127.0.0.1:3210/control", valid: true},
		{endpoint: "http:/control"},
		{endpoint: "https:///control"},
		{endpoint: "//foreign.example/control"},
		{endpoint: "control"},
		{endpoint: "https://user:secret@localhost/control"},
	} {
		t.Run(test.endpoint, func(t *testing.T) {
			if got := validControlEndpoint(test.endpoint); got != test.valid {
				t.Fatalf("validControlEndpoint(%q)=%v", test.endpoint, got)
			}
		})
	}
}
