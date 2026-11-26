package bootstrap

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

func TestMCPHTTPChallengeReportsTheRequiredAction(t *testing.T) {
	cases := []struct {
		name          string
		authorization string
		probe         protocol.MCPTestOutcome
		state         protocol.MCPServerStateType
		problem       protocol.MCPStatusProblemType
	}{
		{
			name: "static", authorization: "Bearer expired-fixture",
			probe: protocol.MCPTestFailed, state: protocol.MCPServerFailed, problem: protocol.MCPStatusConfigurationFailed,
		},
		{
			name:  "interactive",
			probe: protocol.MCPTestAuthorizationRequired, state: protocol.MCPServerNeedsAuth, problem: protocol.MCPStatusAuthorizationRequired,
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if supplied := r.Header.Get("Authorization"); supplied != test.authorization {
					t.Errorf("upstream received Authorization %q, want %q", supplied, test.authorization)
				}
				w.Header().Set("WWW-Authenticate", `Bearer realm="fixture"`)
				w.WriteHeader(http.StatusUnauthorized)
			}))
			t.Cleanup(upstream.Close)
			r := poMCPOpen(t, poMCPConfig(t))
			installExternalConnector(r)
			candidate := protocol.MCPServerCandidate{
				Name: "credential-fixture", Enabled: true,
				Connection:       protocol.MCPConnectionInput{Type: protocol.MCPTransportStreamableHTTP, URL: upstream.URL},
				HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
			}
			if test.authorization != "" {
				candidate.Connection.Authorization = &protocol.MCPAuthorizationChange{Type: protocol.MCPSecretSet, Value: test.authorization}
			}
			probe, err := r.api.TestMCPServer(t.Context(), candidate)
			if err != nil || probe.Outcome != test.probe {
				t.Fatalf("probe outcome = %+v, %v; want %s", probe, err, test.probe)
			}
			server := r.createUserServer(candidate, "save-credential-fixture")
			for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
				page, err := r.api.ListMCPServers(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				for _, entry := range page.Data {
					if entry.ID != server || (entry.Status.Type != protocol.MCPServerFailed && entry.Status.Type != protocol.MCPServerNeedsAuth) {
						continue
					}
					if entry.Status.Type != test.state || entry.Status.Error == nil || entry.Status.Error.Type != test.problem {
						t.Fatalf("connection status = %+v, want %s with %s", entry.Status, test.state, test.problem)
					}
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Fatal("connection did not settle")
		})
	}
}
