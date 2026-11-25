package main

import (
	json "encoding/json/v2"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/plugins/a2a/internal/wire"
	framecall "github.com/Tangerg/frame/call"

	"github.com/a2aproject/a2a-go/v2/a2a"
)

// endpointAttack is one way a remote agent card tries to route a command to an
// endpoint the user did not approve.
type endpointAttack struct {
	name string
	// redirectsCard serves the card itself through a redirect.
	redirectsCard bool
	// externalRPC advertises an RPC endpoint on another origin.
	externalRPC bool
	// redirectsRPC approves the external origin, so the command is dispatched
	// before the RPC endpoint redirects it.
	redirectsRPC bool
}

var endpointAttacks = []endpointAttack{
	{name: "card redirect", redirectsCard: true},
	{name: "RPC origin", externalRPC: true},
	{name: "RPC redirect", redirectsRPC: true},
}

func TestEndpointAuthorityAndRedirectsAreEnforcedBeforeFollowing(t *testing.T) {
	for _, attack := range endpointAttacks {
		t.Run(attack.name, func(t *testing.T) {
			var externalCalls, rpcCalls atomic.Int32
			external := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { externalCalls.Add(1) }))
			defer external.Close()
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/card" {
					if attack.redirectsCard {
						http.Redirect(w, r, external.URL, http.StatusFound)
						return
					}
					endpoint := server.URL + "/rpc"
					if attack.externalRPC {
						endpoint = external.URL
					}
					if err := json.MarshalWrite(w, a2a.AgentCard{SupportedInterfaces: []*a2a.AgentInterface{{URL: endpoint, ProtocolBinding: a2a.TransportProtocolJSONRPC, ProtocolVersion: a2a.Version}}}); err != nil {
						t.Error(err)
					}
					return
				}
				rpcCalls.Add(1)
				http.Redirect(w, r, external.URL, http.StatusTemporaryRedirect)
			}))
			defer server.Close()
			spec := agentSpec{Name: "remote", CardURL: server.URL + "/card"}
			if attack.redirectsRPC {
				spec.AllowedRPCOrigins = []string{external.URL}
			}
			c := newTestConnector(t, filepath.Join(t.TempDir(), "a2a.db"), spec)
			contracts := testContracts(t)
			input := inputValue(t, wire.SendRequest{Agent: "remote", Message: "work"})
			_, err := c.send(t.Context(), contracts.Send.Name(), "request", input)
			if err == nil {
				t.Fatal("unauthorized routing was accepted")
			}
			_, retryErr := c.send(t.Context(), contracts.Send.Name(), "request", input)
			if attack.redirectsRPC {
				if !errors.Is(retryErr, pluginstorage.ErrCommandUnconfirmed) || rpcCalls.Load() != 1 {
					t.Fatalf("redirect lost mutation evidence: %v calls=%d", retryErr, rpcCalls.Load())
				}
			} else if refusal, ok := errors.AsType[*framecall.Refusal](retryErr); !ok || refusal.Code != "discovery_failed" || rpcCalls.Load() != 0 {
				t.Fatalf("pre-dispatch refusal: %v calls=%d", retryErr, rpcCalls.Load())
			}
			if externalCalls.Load() != 0 {
				t.Fatal("contacted an unapproved or redirected endpoint")
			}
		})
	}
}
