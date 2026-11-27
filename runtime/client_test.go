package runtime

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"iter"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
	rt, server, discovery := remoteRuntime(t, testsupport.TempDirectory(t))
	first := remoteTestClient(t, server.URL)
	second := remoteTestClient(t, server.URL)
	options := protocol.CommandOptions{IdempotencyKey: "shared-create", IdempotencyNamespace: discovery.Capabilities.Limits.Idempotency.Namespace}
	request := protocol.CreateSessionRequest{Title: "shared session"}
	invalid := options
	invalid.IdempotencyKey = "shared create"
	if _, err := first.CreateSession(t.Context(), request, invalid); !errors.Is(err, protocol.ErrInvalidParams) || errors.Is(err, runtimeclient.ErrAcknowledgementUnknown) {
		t.Fatalf("remote command key = %v, want definitive refusal", err)
	}
	if _, err := rt.CreateSession(t.Context(), request, invalid); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("local command key = %v, want refusal", err)
	}
	sessions, err := rt.ListSessions(t.Context(), protocol.ListSessionsRequest{}, protocol.CallOptions{})
	if err != nil || len(sessions.Data) != 0 {
		t.Fatalf("invalid command key changed Sessions = (%+v, %v)", sessions, err)
	}
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
	for _, version := range []string{"", "1900-01-01"} {
		options := protocol.CallOptions{RequestMeta: protocol.RequestMeta{ProtocolVersion: new(version)}}
		if _, err := second.Discover(t.Context(), options); !errors.Is(err, protocol.ErrInvalidProtocolVersion) {
			t.Fatalf("remote protocol version %q = %v", version, err)
		}
		if _, err := rt.Discover(t.Context(), options); !errors.Is(err, protocol.ErrInvalidProtocolVersion) {
			t.Fatalf("local protocol version %q = %v", version, err)
		}
	}
	_, err = second.ListRuns(t.Context(), protocol.ListRunsRequest{IncludeDescendants: true}, protocol.CallOptions{
		RequestMeta: protocol.RequestMeta{ClientCapabilities: &protocol.ClientCapabilities{}},
	})
	var problem *protocol.Failure
	if !errors.Is(err, protocol.ErrCapabilityNotNeg) || !errors.As(err, &problem) || len(problem.Problem().RequiredCapabilities) == 0 {
		t.Fatalf("capability metadata = %v", err)
	}
}

func TestRuntimePreservesLocalCauseAndProjectsPublicProblem(t *testing.T) {
	root := testsupport.TempDirectory(t)
	data := filepath.Join(root, "runtime-data")
	rt, server, _ := remoteRuntime(t, data)
	remote := remoteTestClient(t, server.URL)
	options := protocol.CallOptions{}
	waitForRuntimeCompanions(t, rt)

	// Moving the root while companions are starting can fail their process
	// admission independently of the storage error exercised by Discover.
	moved := filepath.Join(root, "moved-data")
	if err := os.Rename(data, moved); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Rename(moved, data); err != nil {
			t.Error(err)
		}
	}()

	_, err := rt.Discover(t.Context(), options)
	if !errors.Is(err, protocol.ErrInternalError) || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("local discovery lost the filesystem cause: %v", err)
	}
	localFailure, ok := errors.AsType[*protocol.Failure](err)
	if !ok {
		t.Fatalf("local discovery did not return a protocol failure: %v", err)
	}
	public := localFailure.Problem()
	if public.Type != protocol.ProblemInternalError || strings.Contains(public.Detail, data) {
		t.Fatalf("local discovery projected an invalid public problem: %+v", public)
	}

	_, err = remote.Discover(t.Context(), options)
	wireFailure, ok := errors.AsType[*protocol.Failure](err)
	if !ok || !errors.Is(err, protocol.ErrInternalError) || errors.Is(err, os.ErrNotExist) {
		t.Fatalf("remote discovery did not return the public failure: %v", err)
	}
	if wire := wireFailure.Problem(); wire.Type != public.Type || wire.Detail != public.Detail {
		t.Fatalf("remote discovery changed the public problem: %+v", wire)
	}
}

func waitForRuntimeCompanions(t *testing.T, rt *Runtime) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	_, changes, err := rt.SubscribeRuntime(ctx, protocol.RuntimeSubscribeRequest{
		Topics: []protocol.RuntimeTopic{protocol.TopicMCPChanged},
	}, protocol.SubscriptionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	next, stop := iter.Pull2(changes)
	defer stop()
	for {
		servers, err := rt.ListMCPServers(ctx, protocol.CallOptions{})
		if err != nil {
			t.Fatal(err)
		}
		var pending []string
		for _, server := range servers.Data {
			switch server.Status.Type {
			case protocol.MCPServerConnecting, protocol.MCPServerDisconnected:
				pending = append(pending, server.ID.Name)
			}
		}
		if len(pending) == 0 {
			return
		}
		if _, err, ok := next(); err != nil || !ok {
			t.Fatalf("runtime companions did not settle: %v: %v", pending, errors.Join(err, ctx.Err()))
		}
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

func TestHTTPRefusalRetainsItsRequestIDAndDefinitiveOutcome(t *testing.T) {
	_, server, discovery := remoteRuntime(t, testsupport.TempDirectory(t))
	client, err := runtimeclient.Connect(t.Context(), runtimeclient.RemoteConfig{
		Endpoint: server.URL,
		Token:    "incorrect-test-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_, err = client.CreateSession(t.Context(), protocol.CreateSessionRequest{Title: "refused before admission"}, protocol.CommandOptions{
		IdempotencyKey:       "refused-command",
		IdempotencyNamespace: discovery.Capabilities.Limits.Idempotency.Namespace,
	})
	var refusal *runtimeclient.TransportError
	if !errors.As(err, &refusal) || refusal.StatusCode != http.StatusUnauthorized || refusal.RequestID == "" ||
		errors.Is(err, runtimeclient.ErrAcknowledgementUnknown) {
		t.Fatalf("HTTP refusal = %v, want a definitive unauthorized response with its request ID", err)
	}
}

func remoteRuntime(t *testing.T, dataDirectory string) (*Runtime, *httptest.Server, *protocol.DiscoverResponse) {
	t.Helper()
	for _, name := range []string{"FLAME_APIKEY", "FLAME_BASEURL", "ANTHROPIC_API_KEY"} {
		t.Setenv(name, "")
	}
	t.Setenv("FLAME_PROVIDER", "anthropic")
	t.Setenv("FLAME_MODEL", "claude-opus-5")
	rt, err := Open(t.Context(), Config{DataDirectory: dataDirectory, UserHomePath: testsupport.TempDirectory(t), DefaultWorkspacePath: testsupport.TempDirectory(t), ConfigDirectories: []string{testsupport.TempDirectory(t)}})
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
