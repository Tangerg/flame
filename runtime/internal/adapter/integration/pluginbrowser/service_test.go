package pluginbrowser

import (
	"testing"
	"time"

	controlhost "github.com/Tangerg/flame/qimu/control/host"
	"github.com/Tangerg/flame/runtime/internal/application/integration/plugins"
)

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
			service, err := New(Config{
				Bridge: &controlhost.Bridge{}, Registry: &plugins.Registry{},
				ControlEndpoint: test.endpoint, MaximumMessageBytes: 1, ProofCapacity: 1, PollInterval: time.Millisecond,
			})
			if test.valid && (err != nil || service == nil) {
				t.Fatalf("valid control endpoint = (%v, %v)", service, err)
			}
			if !test.valid && (err == nil || service != nil) {
				t.Fatalf("invalid control endpoint = (%v, %v)", service, err)
			}
		})
	}
}
