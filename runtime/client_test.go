package runtime

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"io"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/delivery/transport"
	flamehttp "github.com/Tangerg/flame/runtime/internal/delivery/transport/http"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/protocol"
	"github.com/Tangerg/sse"
)

func TestClientsShareRuntimeWithoutSharingOwnership(t *testing.T) {
	rt, server, discovery := remoteRuntime(t)
	first := remoteTestClient(t, server.URL)
	second := remoteTestClient(t, server.URL)
	options := CommandOptions{IdempotencyKey: "shared-create", IdempotencyNamespace: discovery.Capabilities.Limits.Idempotency.Namespace}
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
	}, SubscriptionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	for _, err := range events {
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("closed client stream error = %v", err)
		}
	}
	if _, err := first.Discover(t.Context(), CallOptions{}); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed client discovery = %v", err)
	}
	if _, err := rt.Discover(t.Context(), CallOptions{}); err != nil {
		t.Fatalf("closing client stopped Runtime: %v", err)
	}
	if got, err := second.GetSession(t.Context(), protocol.GetSessionRequest{SessionID: created.ID}, CallOptions{}); err != nil || got.ID != created.ID {
		t.Fatalf("surviving client = (%+v, %v)", got, err)
	}

	options.IdempotencyNamespace = testsupport.AlternateIdempotencyNamespace
	if _, err := second.CreateSession(t.Context(), request, options); !errors.Is(err, protocol.ErrIdempotencyStoreMismatch) || errors.Is(err, ErrAcknowledgementUnknown) {
		t.Fatalf("store fence = %v", err)
	}
	if _, err := second.Discover(t.Context(), CallOptions{RequestMeta: protocol.RequestMeta{ProtocolVersion: "1900-01-01"}}); !errors.Is(err, protocol.ErrInvalidProtocolVersion) {
		t.Fatalf("protocol negotiation = %v", err)
	}
	_, err = second.ListRuns(t.Context(), protocol.ListRunsRequest{IncludeDescendants: true}, CallOptions{
		RequestMeta: protocol.RequestMeta{ClientCapabilities: &protocol.ClientCapabilities{}},
	})
	var problem protocol.ProblemError
	if !errors.Is(err, protocol.ErrCapabilityNotNeg) || !errors.As(err, &problem) || len(problem.Problem().RequiredCapabilities) == 0 {
		t.Fatalf("capability metadata = %v", err)
	}
}

func TestClientRetainsUnknownAcknowledgementAfterCommittedMutation(t *testing.T) {
	rt, original, discovery := remoteRuntime(t)
	var attempts atomic.Int32
	endpoint, err := rt.endpoint()
	if err != nil {
		t.Fatal(err)
	}
	handler := remoteHTTPHandler(t, endpoint, discovery.ServerInfo)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		// Delivery completes and commits before the proxy loses its response.
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, r)
		connection, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = connection.Close()
	}))
	t.Cleanup(server.Close)
	client := remoteTestClient(t, server.URL)
	request := protocol.CreateSessionRequest{Title: "committed before disconnect"}
	options := CommandOptions{IdempotencyKey: "retained-command", IdempotencyNamespace: discovery.Capabilities.Limits.Idempotency.Namespace}
	_, err = client.CreateSession(t.Context(), request, options)
	if !errors.Is(err, ErrAcknowledgementUnknown) || attempts.Load() != 1 {
		t.Fatalf("lost acknowledgement = %v, dispatches = %d", err, attempts.Load())
	}
	page, err := rt.ListSessions(t.Context(), protocol.ListSessionsRequest{}, CallOptions{})
	if err != nil || len(page.Data) != 1 {
		t.Fatalf("durable mutation = (%+v, %v)", page, err)
	}
	reconnected := remoteTestClient(t, original.URL)
	replayed, err := reconnected.CreateSession(t.Context(), request, options)
	if err != nil || replayed.ID != page.Data[0].ID {
		t.Fatalf("explicit same-identity recovery = (%+v, %v)", replayed, err)
	}
}

func TestClientCarriesSnapshotAndOpaqueReplayCursor(t *testing.T) {
	target := &remoteRunTarget{}
	endpoint, err := delivery.NewEndpoint(target, delivery.EndpointConfig{Lifetime: t.Context()})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(remoteHTTPHandler(t, endpoint, protocol.ServerInfo{
		Name: "test", Version: "1", InstanceID: testsupport.RuntimeInstanceID,
	}))
	t.Cleanup(server.Close)
	client := remoteTestClient(t, server.URL)
	request := protocol.SubscribeRunRequest{RunID: "run_test", SegmentID: "seg_test", Snapshot: true}
	ack, events, err := client.SubscribeRun(t.Context(), request, RunSubscriptionOptions{})
	if err != nil || ack.Snapshot == nil || ack.HeadEventID == nil || *ack.HeadEventID != "evt_opaque_head" {
		t.Fatalf("snapshot handoff = (%+v, %v)", ack, err)
	}
	count := 0
	for event, err := range events {
		if err != nil || event.Event.Type != protocol.StreamSegmentFinished {
			t.Fatalf("tail = (%+v, %v)", event, err)
		}
		count++
	}
	if count != 1 {
		t.Fatalf("tail events = %d", count)
	}
	request.Snapshot = false
	_, replay, err := client.SubscribeRun(t.Context(), request, RunSubscriptionOptions{AfterEventID: *ack.HeadEventID})
	if err != nil {
		t.Fatal(err)
	}
	for _, err := range replay {
		if err != nil {
			t.Fatal(err)
		}
	}
	if target.cursor != "evt_opaque_head" {
		t.Fatalf("replay cursor changed: %q", target.cursor)
	}
}

func TestClientRejectsMalformedAcknowledgementsWithoutLosingUncertainty(t *testing.T) {
	for _, body := range []string{
		`{"jsonrpc":"2.0","id":"wrong","result":{}}`,
		`{"jsonrpc":"2.0","id":$ID,"result":null}`,
		`{"jsonrpc":"2.0","id":$ID,"result":{"unknown":true}}`,
		`{"jsonrpc":"2.0","id":$ID,"result":{},"result":{}}`,
	} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					ID string `json:"id"`
				}
				if err := json.UnmarshalRead(r.Body, &request); err != nil {
					t.Error(err)
					return
				}
				id, _ := json.Marshal(request.ID)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, strings.ReplaceAll(body, "$ID", string(id)))
			}))
			t.Cleanup(server.Close)
			client := remoteTestClient(t, server.URL)
			if err := client.DeleteSession(t.Context(), protocol.DeleteSessionRequest{SessionID: "ses_test"}, CommandOptions{}); !errors.Is(err, ErrInvalidResponse) || !errors.Is(err, ErrAcknowledgementUnknown) {
				t.Fatalf("malformed command ack = %v", err)
			}
		})
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
	_, err := client.Discover(t.Context(), CallOptions{})
	var failure *TransportError
	if !errors.As(err, &failure) || failure.StatusCode != http.StatusTemporaryRedirect || forwarded.Load() != 0 {
		t.Fatalf("redirect = %v, forwarded = %d", err, forwarded.Load())
	}
	if strings.Contains(err.Error(), "test-local-token") {
		t.Fatalf("credential in error: %v", err)
	}
}

func TestClientDistinguishesPostAcknowledgementLossFromMalformedStream(t *testing.T) {
	for _, test := range []struct {
		name  string
		frame string
		want  error
	}{
		{name: "truncated observation", want: ErrDisconnected},
		{name: "malformed notification", frame: "data: {not-json}\n\n", want: ErrInvalidResponse},
		{name: "invalid UTF-8", frame: "data: {\"text\":\"\xff\"}\n\n", want: ErrInvalidResponse},
		{name: "unexpected SSE event", frame: "event: other\ndata: {}\n\n", want: ErrInvalidResponse},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Error(err)
					return
				}
				message, err := transport.DecodeMessage(body)
				if err != nil {
					t.Error(err)
					return
				}
				request := message.(*transport.Request)
				ack, err := transport.NewResponseResult(request.ID, &protocol.StartRunResponse{RunID: "run_test", SegmentID: "seg_test", UserItemID: "item_test"})
				if err != nil {
					t.Error(err)
					return
				}
				encoded, err := transport.EncodeMessage(ack)
				if err != nil {
					t.Error(err)
					return
				}
				writer := sse.NewHTTPWriter(w)
				if err := writer.Write(sse.Message{Data: encoded}); err != nil {
					t.Error(err)
					return
				}
				_, _ = io.WriteString(w, test.frame)
			}))
			t.Cleanup(server.Close)
			client := remoteTestClient(t, server.URL)
			ack, events, err := client.StartRun(t.Context(), protocol.StartRunRequest{
				SessionID: "ses_test", Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "hello"}},
			}, RunCommandOptions{})
			if err != nil || ack.RunID != "run_test" {
				t.Fatalf("acknowledgement = (%+v, %v)", ack, err)
			}
			var failure error
			for _, err := range events {
				failure = err
			}
			if !errors.Is(failure, test.want) || errors.Is(failure, ErrAcknowledgementUnknown) {
				t.Fatalf("post-ack failure = %v, want %v without unknown acknowledgement", failure, test.want)
			}
			if test.want == ErrInvalidResponse && errors.Is(failure, ErrDisconnected) {
				t.Fatalf("malformed stream was classified reconnectable: %v", failure)
			}
		})
	}
}

func TestClientCancellationClosesAnUnconsumedStream(t *testing.T) {
	detached := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID string `json:"id"`
		}
		if err := json.UnmarshalRead(r.Body, &request); err != nil {
			t.Error(err)
			return
		}
		id, _ := json.Marshal(request.ID)
		writer := sse.NewHTTPWriter(w)
		if err := writer.Write(sse.Message{Data: []byte(`{"jsonrpc":"2.0","id":` + string(id) + `,"result":{}}`)}); err != nil {
			t.Error(err)
			return
		}
		<-r.Context().Done()
		close(detached)
	}))
	t.Cleanup(server.Close)
	client := remoteTestClient(t, server.URL)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	_, _, err := client.SubscribeRuntime(ctx, protocol.RuntimeSubscribeRequest{Topics: []protocol.RuntimeTopic{protocol.TopicSessionsChanged}}, SubscriptionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-detached:
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation retained the unconsumed HTTP body")
	}
}

func TestClientRecognizesDefinitiveTransportRefusal(t *testing.T) {
	_, server, _ := remoteRuntime(t)
	client, err := Connect(t.Context(), RemoteConfig{Endpoint: server.URL, Token: "wrong-token"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	err = client.DeleteSession(t.Context(), protocol.DeleteSessionRequest{SessionID: "ses_test"}, CommandOptions{IdempotencyKey: "not-dispatched"})
	var refusal *TransportError
	if !errors.As(err, &refusal) || refusal.StatusCode != http.StatusUnauthorized || errors.Is(err, ErrAcknowledgementUnknown) || errors.Is(err, ErrDisconnected) {
		t.Fatalf("authentication refusal = %v", err)
	}
}

func TestClientMessageBudgetDoesNotAuthorizeReconnect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = io.WriteString(w, strings.Repeat(" ", 65))
	}))
	t.Cleanup(server.Close)
	client, err := Connect(t.Context(), RemoteConfig{Endpoint: server.URL, MaxMessageBytes: 64})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	err = client.DeleteSession(t.Context(), protocol.DeleteSessionRequest{SessionID: "ses_test"}, CommandOptions{})
	if !errors.Is(err, ErrInvalidResponse) || !errors.Is(err, ErrAcknowledgementUnknown) || errors.Is(err, ErrDisconnected) {
		t.Fatalf("bounded response failure = %v", err)
	}
}

func TestConnectValidatesWithoutContactingOrOwningRuntime(t *testing.T) {
	for _, endpoint := range []string{"", "file:///tmp/flame", "http://user:secret@localhost", "http://localhost?token=secret", "http://localhost/#fragment"} {
		if _, err := Connect(t.Context(), RemoteConfig{Endpoint: endpoint}); err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("invalid endpoint %q = %v", endpoint, err)
		}
	}
	if _, err := Connect(t.Context(), RemoteConfig{Endpoint: "http://localhost", MaxMessageBytes: -1}); err == nil {
		t.Fatal("negative message budget was accepted")
	}
	var contacts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { contacts.Add(1) }))
	t.Cleanup(server.Close)
	client := remoteTestClient(t, server.URL)
	if err := client.Close(); err != nil || contacts.Load() != 0 {
		t.Fatalf("connect/close contacted server %d times: %v", contacts.Load(), err)
	}
}

func remoteRuntime(t *testing.T) (*Runtime, *httptest.Server, *protocol.DiscoverResponse) {
	t.Helper()
	for _, name := range []string{"FLAME_APIKEY", "FLAME_MODEL", "FLAME_BASEURL", "FLAME_MCP_SERVERS", "FLAME_A2A_AGENTS", "FLAME_A2A_RPC_ORIGINS"} {
		t.Setenv(name, "")
	}
	t.Setenv("FLAME_PROVIDER", "anthropic")
	rt, err := Open(t.Context(), Config{DataDirectory: t.TempDir(), UserHomePath: t.TempDir(), DefaultWorkspacePath: t.TempDir(), ConfigDirectories: []string{t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := rt.Close(); err != nil {
			t.Error(err)
		}
	})
	discovery, err := rt.Discover(t.Context(), CallOptions{})
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
	server, err := flamehttp.NewServer(flamehttp.Config{
		Endpoint: endpoint, Addr: "127.0.0.1:0", ServerInfo: info,
		ProtocolVersion: protocol.ProtocolVersion, LocalToken: "test-local-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	return server.Handler()
}

func remoteTestClient(t *testing.T, endpoint string) *Client {
	t.Helper()
	client, err := Connect(t.Context(), RemoteConfig{Endpoint: endpoint, Token: "test-local-token"})
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

type remoteRunTarget struct{ cursor string }

func (r *remoteRunTarget) SubscribeRun(ctx context.Context, request protocol.SubscribeRunRequest) (*protocol.SubscribeRunResponse, iter.Seq2[protocol.RunEvent, error], error) {
	r.cursor = delivery.AfterEventIDFrom(ctx)
	head := "evt_opaque_head"
	ack := &protocol.SubscribeRunResponse{RunID: request.RunID, SegmentID: request.SegmentID, HeadEventID: &head}
	if request.Snapshot {
		ack.Snapshot = &protocol.SessionSnapshot{Items: []protocol.Item{}, Runs: []protocol.RunRef{}, Interrupts: []protocol.PendingInterruptSet{}}
	}
	return ack, func(yield func(protocol.RunEvent, error) bool) {
		yield(protocol.RunEvent{
			RunID: request.RunID, SegmentID: request.SegmentID, EventID: "evt_opaque_tail", Timestamp: time.Unix(1, 0).UTC(),
			Event: protocol.StreamEvent{Type: protocol.StreamSegmentFinished, Outcome: &protocol.SegmentOutcome{Type: protocol.SegmentCompleted}, Metrics: &protocol.RunMetrics{}, ContextTokens: new(int64)},
		}, nil)
	}, nil
}
