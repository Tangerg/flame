package mcp

import (
	"bytes"
	"context"
	"io"
	"iter"
	"net"
	"sync"
	"testing"

	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/go-sdk/jsonrpc"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type frameRecorder struct {
	mu     sync.Mutex
	data   bytes.Buffer
	target io.WriteCloser
}

func (r *frameRecorder) Write(data []byte) (int, error) {
	r.mu.Lock()
	r.data.Write(data)
	r.mu.Unlock()
	return r.target.Write(data)
}

func TestRequestFrameBudgetIncludesActualSDKEnvelopeAndStreamGrant(t *testing.T) {
	for _, streaming := range []bool{false, true} {
		t.Run(map[bool]string{false: "call", true: "stream"}[streaming], func(t *testing.T) {
			method := testMethod[string, string](t, "frame.write", portable.Mutation)
			stream, err := NewStreamingMethod[string, string, string](MethodConfig{Name: "frame.stream", Kind: portable.Mutation, Target: "installation", Description: "Stream."})
			if err != nil {
				t.Fatal(err)
			}
			backend := testBackend(t, sdk.NewServer(&sdk.Implementation{Name: "frame-owner"}, nil))
			if err := method.RegisterServer(backend, nil, func(_ context.Context, _ *sdk.ServerSession, command Command[string]) (string, error) {
				return command.Input, nil
			}); err != nil {
				t.Fatal(err)
			}
			if err := stream.RegisterServer(backend, func(_ context.Context, _ *sdk.ServerSession, command Command[string]) iter.Seq2[StreamValue[string, string], error] {
				return func(yield func(StreamValue[string, string], error) bool) {
					yield(StreamValue[string, string]{Result: new(command.Input)}, nil)
				}
			}); err != nil {
				t.Fatal(err)
			}
			contract, err := portable.NewCapability(portable.CapabilityConfig{Reference: "frame", Composition: portable.Selection, Scope: "installation"}, method.Descriptor(), stream.Descriptor())
			if err != nil {
				t.Fatal(err)
			}
			if err := backend.BindCapability("frame", contract); err != nil {
				t.Fatal(err)
			}
			if err := backend.Publish(); err != nil {
				t.Fatal(err)
			}
			a, b := net.Pipe()
			t.Cleanup(func() { _ = a.Close(); _ = b.Close() })
			server, err := backend.server.Connect(t.Context(), &sdk.IOTransport{Reader: a, Writer: a}, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = server.Close() })
			recorded := &frameRecorder{target: b}
			client, err := NewProcessClient()
			if err != nil {
				t.Fatal(err)
			}
			session, err := client.Connect(t.Context(), &sdk.IOTransport{Reader: b, Writer: recorded}, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = session.Close() })
			capability, err := NewCapabilityBinding(session, "frame", contract)
			if err != nil {
				t.Fatal(err)
			}
			input := `escaped <>& " quotation and Unicode 世界`
			admitted, err := jsonvalue.Encode(input)
			if err != nil {
				t.Fatal(err)
			}
			invocation := portable.Invocation{ID: "original-frame", Origin: "human", Parent: "original-owner", Target: portable.Target{Scope: "installation", Reference: "original-installation"}}
			ctx, err := WithInvocation(t.Context(), invocation)
			if err != nil {
				t.Fatal(err)
			}
			name := method.Name()
			if streaming {
				name = stream.Descriptor().Name
			}
			bound, err := capability.Method(name)
			if err != nil {
				t.Fatal(err)
			}
			budget, err := ProcessRequestFrameBytes(ctx, contract, "frame", name, invocation.ID, admitted, sdk.ParamsBase{})
			if err != nil {
				t.Fatal(err)
			}
			if streaming {
				callbacks := newTestCallbacks(t)
				if err := callbacks.AdmitSession(session); err != nil {
					t.Fatal(err)
				}
				for value, err := range stream.StreamWithMetadata(ctx, bound, callbacks, callbackTestLimits, invocation.ID, input, sdk.ParamsBase{}) {
					if err != nil || value.Result == nil || *value.Result != input {
						t.Fatalf("stream=%+v %v", value, err)
					}
				}
			} else if value, err := bound.Invoke(ctx, invocation.ID, admitted); err != nil || value != admitted {
				t.Fatalf("call=%v %v", value, err)
			}
			recorded.mu.Lock()
			data := bytes.Clone(recorded.data.Bytes())
			recorded.mu.Unlock()
			found := false
			for _, line := range bytes.Split(data, []byte("\n")) {
				if len(line) == 0 {
					continue
				}
				message, err := jsonrpc.DecodeMessage(line)
				if err != nil {
					t.Fatal(err)
				}
				request, ok := message.(*jsonrpc.Request)
				if !ok || request.Method != InvokeMethod {
					continue
				}
				found = true
				actual := len(line) + 1
				if actual > budget {
					t.Fatalf("actual frame=%d budget=%d", actual, budget)
				}
			}
			if !found {
				t.Fatal("no operation crossed the SDK transport")
			}
		})
	}
}

func (r *frameRecorder) Close() error { return r.target.Close() }
