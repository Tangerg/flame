package main

import (
	json "encoding/json/v2"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/plugins/a2a/internal/wire"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/a2aproject/a2a-go/v2/a2a"
)

func TestEndpointAuthorityAndRedirectsAreEnforcedBeforeFollowing(t *testing.T) {
	for _, mode := range []string{"card redirect", "RPC origin", "RPC redirect"} {
		t.Run(mode, func(t *testing.T) {
			var externalCalls, rpcCalls atomic.Int32
			external := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { externalCalls.Add(1) }))
			defer external.Close()
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/card" {
					if mode == "card redirect" {
						http.Redirect(w, r, external.URL, http.StatusFound)
						return
					}
					endpoint := server.URL + "/rpc"
					if mode == "RPC origin" {
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
			if mode == "RPC redirect" {
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
			if mode == "RPC redirect" {
				if !errors.Is(retryErr, pluginstorage.ErrCommandUnconfirmed) || rpcCalls.Load() != 1 {
					t.Fatalf("redirect lost mutation evidence: %v calls=%d", retryErr, rpcCalls.Load())
				}
			} else if refusal, ok := errors.AsType[*portable.Refusal](retryErr); !ok || refusal.Code != "discovery_failed" || rpcCalls.Load() != 0 {
				t.Fatalf("pre-dispatch refusal: %v calls=%d", retryErr, rpcCalls.Load())
			}
			if externalCalls.Load() != 0 {
				t.Fatal("contacted an unapproved or redirected endpoint")
			}
		})
	}
}

func TestExplicitRPCOriginAllowsSeparateAgentEndpoint(t *testing.T) {
	remote := newRemoteFixture(t)
	card := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.MarshalWrite(w, a2a.AgentCard{SupportedInterfaces: []*a2a.AgentInterface{{URL: remote.server.URL + "/rpc", ProtocolBinding: a2a.TransportProtocolJSONRPC, ProtocolVersion: a2a.Version}}}); err != nil {
			t.Error(err)
		}
	}))
	defer card.Close()
	c := newTestConnector(t, filepath.Join(t.TempDir(), "a2a.db"), agentSpec{Name: "remote", CardURL: card.URL, AllowedRPCOrigins: []string{remote.server.URL}})
	contracts := testContracts(t)
	if _, err := c.send(t.Context(), contracts.Send.Name(), "request", inputValue(t, wire.SendRequest{Agent: "remote", Message: "work"})); err != nil {
		t.Fatal(err)
	}
}

func TestResponseLimitKeepsSendUnconfirmed(t *testing.T) {
	remote := newRemoteFixture(t)
	remote.message = a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart(strings.Repeat("x", maxResponseBytes)))
	c := newTestConnector(t, filepath.Join(t.TempDir(), "a2a.db"), remote.spec())
	contracts := testContracts(t)
	input := inputValue(t, wire.SendRequest{Agent: "research", Message: "work"})
	if _, err := c.send(t.Context(), contracts.Send.Name(), "request", input); !errors.Is(err, errResponseLimit) {
		t.Fatalf("response bound: %v", err)
	}
	if _, err := c.send(t.Context(), contracts.Send.Name(), "request", input); !errors.Is(err, pluginstorage.ErrCommandUnconfirmed) {
		t.Fatalf("oversized send replay: %v", err)
	}
}

func TestConfigurationRejectsAmbiguousAuthoritiesWithoutExposingSecrets(t *testing.T) {
	for _, spec := range []agentSpec{
		{Name: "bad name", CardURL: "https://example.test/card"},
		{Name: "agent", CardURL: "file:///private/agent.json"},
		{Name: "agent", CardURL: "https://user:private-password@example.test/card?token=private-token"},
		{Name: "agent", CardURL: "https://example.test/%zz?token=private-token"},
		{Name: "agent", CardURL: "https://example.test/card", AllowedRPCOrigins: []string{"https://rpc.test/path?key=private-token"}},
		{Name: "agent", CardURL: "https://example.test/card#ignored"},
	} {
		_, err := configureAgents([]agentSpec{spec})
		if err == nil || strings.Contains(err.Error(), "private-") {
			t.Fatalf("unsafe configuration failure: %v", err)
		}
	}
	left, err := configureAgents([]agentSpec{{Name: "agent", CardURL: "https://example.test/card", AllowedRPCOrigins: []string{"https://RPC.test", "https://rpc.test:443/"}}})
	if err != nil {
		t.Fatal(err)
	}
	right, err := configureAgents([]agentSpec{{Name: "agent", CardURL: "https://example.test/card", AllowedRPCOrigins: []string{"https://rpc.test:443"}, Description: "renamed description"}})
	if err != nil || left["agent"].authority != right["agent"].authority {
		t.Fatalf("equivalent grants differ: %v", err)
	}
}
