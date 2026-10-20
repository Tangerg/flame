package runtime

import (
	"context"
	"errors"
	"iter"
	"sync/atomic"
	"testing"

	runtimeclient "github.com/Tangerg/flame/packages/runtime-client/go"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk/go"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	flamehttp "github.com/Tangerg/flame/runtime/internal/delivery/transport/http"
	"github.com/Tangerg/flame/runtime/protocol"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type streamCallbackOwnerKey struct{}

type streamCallbackTarget struct{ calls atomic.Int32 }

func (*streamCallbackTarget) Discover(context.Context) (*protocol.DiscoverResponse, error) {
	return &protocol.DiscoverResponse{Capabilities: protocol.ServerCapabilities{Features: map[string]protocol.FeatureCapability{protocol.FeaturePlugins: {Enabled: true}}}}, nil
}

func (s *streamCallbackTarget) PreparePluginStream(_ context.Context, request protocol.CallPluginRequest) (delivery.OperationKind, func(context.Context) (struct{}, iter.Seq2[protocol.PluginStreamEvent, error], error), error) {
	return delivery.OperationCommand, func(ctx context.Context) (struct{}, iter.Seq2[protocol.PluginStreamEvent, error], error) {
		return s.StreamPlugin(ctx, request)
	}, nil
}

func (s *streamCallbackTarget) StreamPlugin(ctx context.Context, _ protocol.CallPluginRequest) (struct{}, iter.Seq2[protocol.PluginStreamEvent, error], error) {
	if ctx.Value(streamCallbackOwnerKey{}) != "original owner" {
		return struct{}{}, nil, errors.New("captured owner changed")
	}
	return struct{}{}, func(yield func(protocol.PluginStreamEvent, error) bool) {
		s.calls.Add(1)
		if yield(protocol.PluginStreamEvent{Kind: protocol.PluginStreamChunk, Value: `18446744073709551615`}, nil) {
			yield(protocol.PluginStreamEvent{Kind: protocol.PluginStreamResult, Value: `true`}, nil)
		}
	}, nil
}

func TestCallbackInstalledStreamKeepsCapturedOwnerAndRefusesRetiredOrMissingContext(t *testing.T) {
	target := &streamCallbackTarget{}
	endpoint, err := delivery.NewEndpoint(target, delivery.EndpointConfig{Lifetime: t.Context()})
	if err != nil {
		t.Fatal(err)
	}
	rpc, err := flamehttp.NewRPCHandler(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	callbacks := pluginsdk.NewCallbacks()
	defer callbacks.Close()
	if err := callbacks.PublishRPC(rpc); err != nil {
		t.Fatal(err)
	}
	session := new(sdk.ClientSession)
	callbacks.AdmitSession(session)
	owner := context.WithValue(t.Context(), streamCallbackOwnerKey{}, "original owner")
	ref, release, err := callbacks.OpenRPC(owner, session, pluginsdk.CallbackLimits{MaxRequestBytes: 4096, MaxResponseBytes: 4096})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	caller, err := runtimeclient.NewPluginCallbackCaller(ref)
	if err != nil {
		t.Fatal(err)
	}
	bound := bindTypedStream(t, caller)
	stream := bound.Stream(t.Context(), map[string]string{}, protocol.PluginCallOptions{IdempotencyKey: "original"})
	if target.calls.Load() != 0 {
		t.Fatal("lazy callback performed I/O before consumption")
	}
	var chunks, results int
	for value, err := range stream {
		if err != nil {
			t.Fatal(err)
		}
		if value.Chunk != nil {
			chunks++
		}
		if value.Result != nil {
			results++
		}
	}
	if chunks != 1 || results != 1 || target.calls.Load() != 1 {
		t.Fatalf("callback chunks=%d results=%d calls=%d", chunks, results, target.calls.Load())
	}
	invoked, err := pluginsdk.WithInvocation(t.Context(), capability.Invocation{ID: "original", Origin: capability.Model, Parent: "parent", Target: capability.Target{Scope: "call", Reference: "original"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, err := range bound.Stream(invoked, map[string]string{}, protocol.PluginCallOptions{IdempotencyKey: "original"}) {
		if err == nil || errors.Is(err, runtimeclient.ErrAcknowledgementUnknown) {
			t.Fatalf("missing calling-owner context was treated as dispatched: %v", err)
		}
	}
	retained := bound.Stream(t.Context(), map[string]string{}, protocol.PluginCallOptions{IdempotencyKey: "original"})
	release()
	for _, err := range retained {
		if !errors.Is(err, pluginsdk.ErrStaleCall) || errors.Is(err, runtimeclient.ErrAcknowledgementUnknown) {
			t.Fatalf("retired original reference=%v", err)
		}
	}
	if target.calls.Load() != 1 {
		t.Fatal("refused stream reached another owner")
	}
}
