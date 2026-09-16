package delivery

import (
	"testing"

	"github.com/Tangerg/flame/runtime/protocol"
)

// TestIsolatedSessionUpdateIsGatedOnTheIsolationCapability pins the composition
// fact behind session isolation. An isolated Session jails its shell commands,
// so a host with no isolation backend cannot honor the policy; the endpoint
// refuses the request rather than accepting it and failing one shell call at a
// time. Releasing isolation stays available either way — a Session carried here
// from a host that could isolate must remain editable.
func TestIsolatedSessionUpdateIsGatedOnTheIsolationCapability(t *testing.T) {
	meta, found := Contract().Lookup(SessionsUpdate)
	if !found {
		t.Fatalf("%s is not published", SessionsUpdate)
	}
	isolate, release := true, false
	refusal := func(t *testing.T, available bool, isolated *bool) *Failure {
		t.Helper()
		handler := &Handler{}
		handler.features.isolation = available
		endpoint := mustNewEndpoint(t, handler, EndpointConfig{})
		return endpoint.enforceCapabilities(t.Context(), meta, protocol.UpdateSessionRequest{
			SessionID: "ses_1", ExpectedRevision: 1, Isolated: isolated,
		})
	}

	if refusal(t, false, &isolate) == nil {
		t.Fatal("a build with no isolation backend admitted an isolated session")
	}
	if failure := refusal(t, false, &release); failure != nil {
		t.Fatalf("releasing isolation was refused: %v", failure)
	}
	if failure := refusal(t, true, &isolate); failure != nil {
		t.Fatalf("a build that can isolate still refused: %v", failure)
	}
	if failure := refusal(t, false, nil); failure != nil {
		t.Fatalf("an update that does not mention isolation was refused: %v", failure)
	}
}

// TestIsolationCapabilityIsAdvertisedFromTheComposition pins that discovery
// answers the same question the gate asks, so a client can hide the surface
// instead of discovering it by being refused.
func TestIsolationCapabilityIsAdvertisedFromTheComposition(t *testing.T) {
	for _, available := range []bool{false, true} {
		handler := &Handler{}
		handler.features.isolation = available
		discovered, err := handler.Discover(t.Context())
		if err != nil {
			t.Fatalf("runtime.discover: %v", err)
		}
		if got := discovered.Capabilities.Features[protocol.FeatureIsolation]; got.Enabled != available {
			t.Fatalf("features.isolation enabled = %v, want %v", got.Enabled, available)
		}
	}
}
