package bootstrap

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	runtimeclient "github.com/Tangerg/flame/packages/go-sdk/client"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/packages/go-sdk/runevent"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/delivery/dispatch"
	runtimehttp "github.com/Tangerg/flame/runtime/internal/delivery/http"
	"github.com/Tangerg/go-sdk/jsonrpc"
	"github.com/Tangerg/scope/core/chat"
)

func TestRemoteRunStreamRejectsAChildUsingTheRootSegment(t *testing.T) {
	home := t.TempDir()
	t.Setenv("FLAME_HOME", home)
	var calls atomic.Int32
	model := delegateRestartModel{chat.ModelFunc(completeAllDelegatesScript{calls: &calls}.Call)}
	host, api, _ := openProtocolRuntimeWithStores(t, model)
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})

	const token = "run-stream-test-token"
	discovery, err := api.Discover(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	server, err := runtimehttp.NewServer(runtimehttp.Config{
		Endpoint: host.Endpoint(), PluginControl: host.PluginControl(),
		Addr: "127.0.0.1:0", ServerInfo: discovery.ServerInfo, Authentication: mustLocalToken(t, token),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := server.Close(); err != nil {
			t.Error(err)
		}
	})
	handler := server.Handler()
	methodHeader := runtimehttp.Contract().Headers.Method
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(methodHeader) != delivery.RunsStart.String() {
			handler.ServeHTTP(w, r)
			return
		}
		recorded := httptest.NewRecorder()
		handler.ServeHTTP(recorded, r)
		body, err := misrouteRootTerminal(recorded.Body.String())
		if err != nil {
			t.Error(err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		for name, values := range recorded.Header() {
			w.Header()[name] = values
		}
		w.Header().Del("Content-Length")
		w.WriteHeader(recorded.Code)
		if _, err := w.Write([]byte(body)); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(remote.Close)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	client, err := runtimeclient.Connect(ctx, runtimeclient.RemoteConfig{Endpoint: remote.URL, Token: token})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	metadata := protocol.RequestMeta{ClientCapabilities: &protocol.ClientCapabilities{
		Features: map[string]protocol.FeaturePreference{protocol.FeatureChildRuns: {Enabled: true}},
	}}
	options := protocol.CallOptions{RequestMeta: metadata}
	namespace := discovery.Capabilities.Limits.Idempotency.Namespace
	session, err := client.CreateSession(ctx, protocol.CreateSessionRequest{Title: "Remote Run stream root"},
		protocol.CommandOptions{RequestMeta: metadata, IdempotencyKey: "root-stream-session", IdempotencyNamespace: namespace})
	if err != nil {
		t.Fatal(err)
	}
	started, events, err := client.StartRun(ctx, protocol.StartRunRequest{
		SessionID: session.ID, Input: []protocol.ContentBlock{{Type: protocol.ContentBlockText, Text: "delegate both tasks"}},
	}, protocol.RunCommandOptions{RequestMeta: metadata, IdempotencyKey: "root-stream-start", IdempotencyNamespace: namespace})
	if err != nil {
		t.Fatal(err)
	}
	var childFinished bool
	var observationErr error
	for event, err := range events {
		if err != nil {
			observationErr = err
			break
		}
		if event.RunID == started.RunID || event.Event.Type != runevent.SegmentFinished {
			continue
		}
		if event.SegmentID == started.SegmentID {
			t.Errorf("delivered a terminal event from child %s using root Segment %s", event.RunID, event.SegmentID)
			continue
		}
		childFinished = true
	}
	if !childFinished || !errors.Is(observationErr, protocol.ErrInvalidResponse) {
		t.Fatalf("child completion = %t, observation error = %v", childFinished, observationErr)
	}
	run, err := client.GetRun(ctx, protocol.GetRunRequest{RunID: started.RunID}, options)
	if err != nil || run.Outcome == nil || run.Outcome.Type != protocol.OutcomeCompleted {
		t.Fatalf("durable root Run = %+v, error = %v", run, err)
	}
}

func misrouteRootTerminal(body string) (string, error) {
	frames := strings.Split(body, "\n\n")
	var root protocol.StartRunResponse
	var childID string
	for index, frame := range frames {
		prefix, document, hasData := strings.Cut(frame, "data: ")
		if !hasData {
			continue
		}
		message, err := protocol.DecodeRPCMessage([]byte(document))
		if err != nil {
			return "", err
		}
		switch message := message.(type) {
		case *jsonrpc.Response:
			if err := protocol.DecodeResponse(message.Result, &root); err != nil {
				return "", err
			}
		case *jsonrpc.Request:
			var event protocol.RunEvent
			if err := protocol.DecodeResponse(message.Params, &event); err != nil {
				return "", err
			}
			if event.Event.Type != runevent.SegmentFinished {
				continue
			}
			if event.RunID != root.RunID {
				childID = event.RunID
				continue
			}
			if childID == "" || event.SegmentID != root.SegmentID {
				return "", errors.New("expected a child terminal before the acknowledged root terminal")
			}
			event.RunID = childID
			notification, err := protocol.NewRPCNotification(dispatch.NotificationRunEvent, event)
			if err != nil {
				return "", err
			}
			encoded, err := jsonrpc.EncodeMessage(notification)
			if err != nil {
				return "", err
			}
			frames[index] = prefix + "data: " + string(encoded)
			return strings.Join(frames, "\n\n"), nil
		}
	}
	return "", errors.New("remote Run stream had no root terminal")
}
