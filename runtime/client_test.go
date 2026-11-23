package runtime

import (
	json "encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	runtimeclient "github.com/Tangerg/flame/packages/go-sdk/client"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	runtimehttp "github.com/Tangerg/flame/runtime/internal/delivery/http"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestClientRejectsResumeAcknowledgementsThatChangeInputPresence(t *testing.T) {
	for _, withInput := range []bool{false, true} {
		t.Run(fmt.Sprintf("input_%t", withInput), func(t *testing.T) {
			const runID = "run_fixture"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					ID string `json:"id"`
				}
				if err := json.UnmarshalRead(r.Body, &request); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				result := protocol.ResumeRunResponse{RunID: runID, SegmentID: "seg_fixture"}
				if !withInput {
					result.UserItemID = new("item_unrequested")
				}
				body, err := json.Marshal(struct {
					Version string                     `json:"jsonrpc"`
					ID      string                     `json:"id"`
					Result  protocol.ResumeRunResponse `json:"result"`
				}{"2.0", request.ID, result})
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", protocol.RPCStreamMediaType)
				if _, err := fmt.Fprintf(w, "event: message\ndata: %s\n\n", body); err != nil {
					t.Error(err)
				}
			}))
			t.Cleanup(server.Close)
			client := remoteTestClient(t, server.URL)
			request := protocol.ResumeRunRequest{RunID: runID, Responses: []protocol.InterruptResponse{{
				ItemID:   "item_question",
				Response: protocol.InterruptResponseValue{Type: protocol.InterruptResponseAnswer, Answers: [][]string{{"yes"}}},
			}}}
			if withInput {
				request.Input = []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "additional input"}}
			}
			result, _, err := client.ResumeRun(t.Context(), request, protocol.RunCommandOptions{
				IdempotencyKey: "resume-response", IdempotencyNamespace: testsupport.IdempotencyNamespace,
			})
			if result != nil || !errors.Is(err, protocol.ErrInvalidResponse) || !errors.Is(err, runtimeclient.ErrAcknowledgementUnknown) {
				t.Fatalf("Resume acknowledgement = (%+v, %v)", result, err)
			}
		})
	}
}

func TestClientsShareRuntimeWithoutSharingOwnership(t *testing.T) {
	rt, server, discovery := remoteRuntime(t)
	first := remoteTestClient(t, server.URL)
	second := remoteTestClient(t, server.URL)
	options := protocol.CommandOptions{IdempotencyKey: "shared-create", IdempotencyNamespace: discovery.Capabilities.Limits.Idempotency.Namespace}
	request := protocol.CreateSessionRequest{Title: "shared session"}
	created, err := first.CreateSession(t.Context(), request, options)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := second.CreateSession(t.Context(), request, options)
	if err != nil || replayed.ID != created.ID {
		t.Fatalf("cross-client replay = (%+v, %v), want %s", replayed, err, created.ID)
	}
	_, events, err := first.SubscribeRuntime(t.Context(), protocol.RuntimeSubscribeRequest{
		Topics: []protocol.RuntimeTopic{protocol.TopicSessionsChanged},
	}, protocol.SubscriptionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	for _, err := range events {
		if !errors.Is(err, runtimeclient.ErrClosed) {
			t.Fatalf("closed client stream error = %v", err)
		}
	}
	if _, err := first.Discover(t.Context(), protocol.CallOptions{}); !errors.Is(err, runtimeclient.ErrClosed) {
		t.Fatalf("closed client discovery = %v", err)
	}
	if _, err := rt.Discover(t.Context(), protocol.CallOptions{}); err != nil {
		t.Fatalf("closing client stopped Runtime: %v", err)
	}
	if got, err := second.GetSession(t.Context(), protocol.GetSessionRequest{SessionID: created.ID}, protocol.CallOptions{}); err != nil || got.ID != created.ID {
		t.Fatalf("surviving client = (%+v, %v)", got, err)
	}

	options.IdempotencyNamespace = testsupport.AlternateIdempotencyNamespace
	if _, err := second.CreateSession(t.Context(), request, options); !errors.Is(err, protocol.ErrIdempotencyStoreMismatch) || errors.Is(err, runtimeclient.ErrAcknowledgementUnknown) {
		t.Fatalf("store fence = %v", err)
	}
	if _, err := second.Discover(t.Context(), protocol.CallOptions{RequestMeta: protocol.RequestMeta{ProtocolVersion: "1900-01-01"}}); !errors.Is(err, protocol.ErrInvalidProtocolVersion) {
		t.Fatalf("protocol negotiation = %v", err)
	}
	_, err = second.ListRuns(t.Context(), protocol.ListRunsRequest{IncludeDescendants: true}, protocol.CallOptions{
		RequestMeta: protocol.RequestMeta{ClientCapabilities: &protocol.ClientCapabilities{}},
	})
	var problem *protocol.Failure
	if !errors.Is(err, protocol.ErrCapabilityNotNeg) || !errors.As(err, &problem) || len(problem.Problem().RequiredCapabilities) == 0 {
		t.Fatalf("capability metadata = %v", err)
	}
}

func TestClientDoesNotFollowRedirectsOrTransmitCredentials(t *testing.T) {
	var forwarded atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { forwarded.Add(1) }))
	t.Cleanup(destination.Close)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(server.Close)
	client := remoteTestClient(t, server.URL)
	_, err := client.Discover(t.Context(), protocol.CallOptions{})
	var failure *runtimeclient.TransportError
	if !errors.As(err, &failure) || failure.StatusCode != http.StatusTemporaryRedirect || forwarded.Load() != 0 {
		t.Fatalf("redirect = %v, forwarded = %d", err, forwarded.Load())
	}
	if strings.Contains(err.Error(), "test-local-token") {
		t.Fatalf("credential in error: %v", err)
	}
}

func remoteRuntime(t *testing.T) (*Runtime, *httptest.Server, *protocol.DiscoverResponse) {
	t.Helper()
	for _, name := range []string{"FLAME_APIKEY", "FLAME_MODEL", "FLAME_BASEURL"} {
		t.Setenv(name, "")
	}
	t.Setenv("FLAME_PROVIDER", "anthropic")
	t.Setenv("FLAME_MODEL", "claude-opus-5")
	rt, err := Open(t.Context(), Config{DataDirectory: testsupport.TempDirectory(t), UserHomePath: testsupport.TempDirectory(t), DefaultWorkspacePath: testsupport.TempDirectory(t), ConfigDirectories: []string{testsupport.TempDirectory(t)}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := rt.Close(); err != nil {
			t.Error(err)
		}
	})
	discovery, err := rt.Discover(t.Context(), protocol.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	endpoint, err := rt.endpoint()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(remoteHTTPHandler(t, endpoint, discovery.ServerInfo))
	t.Cleanup(server.Close)
	return rt, server, discovery
}

func remoteHTTPHandler(t *testing.T, endpoint *delivery.Endpoint, info protocol.ServerInfo) http.Handler {
	t.Helper()
	server, err := runtimehttp.NewServer(runtimehttp.Config{
		Endpoint: endpoint, PluginControl: http.NotFoundHandler(), Addr: "127.0.0.1:0", ServerInfo: info,
		Authentication: mustLocalToken(t, "test-local-token"),
	})
	if err != nil {
		t.Fatal(err)
	}
	return server.Handler()
}

func remoteTestClient(t *testing.T, endpoint string) *runtimeclient.Client {
	t.Helper()
	client, err := runtimeclient.Connect(t.Context(), runtimeclient.RemoteConfig{Endpoint: endpoint, Token: "test-local-token"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	return client
}

func mustLocalToken(t testing.TB, token string) runtimehttp.Authentication {
	t.Helper()
	authentication, err := runtimehttp.LocalTokenAuthentication(token)
	if err != nil {
		t.Fatal(err)
	}
	return authentication
}
