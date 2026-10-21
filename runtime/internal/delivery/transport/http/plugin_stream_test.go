package http_test

import (
	"context"
	"errors"
	"iter"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	runtimeclient "github.com/Tangerg/flame/packages/runtime-client/go"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/protocol"
)

type streamRuntime struct {
	fakeRuntime
	terminal          protocol.PluginStreamEvent
	starts            atomic.Int32
	identity          string
	awaitCancellation bool
	canceled          chan struct{}
	input             chan string
}

func (s *streamRuntime) PreparePluginCall(_ context.Context, request protocol.CallPluginRequest) (delivery.OperationKind, func(context.Context) (*protocol.CallPluginResponse, error), error) {
	return delivery.OperationQuery, func(context.Context) (*protocol.CallPluginResponse, error) {
		s.input <- request.Input
		return &protocol.CallPluginResponse{Output: request.Input}, nil
	}, nil
}

func TestRemotePluginCallPreservesSharedNumericCorpus(t *testing.T) {
	data, err := os.ReadFile("../../../../testdata/plugin-values.json")
	if err != nil {
		t.Fatal(err)
	}
	source := strings.TrimSpace(string(data))
	method, err := pluginsdk.NewMethod[map[string]any, map[string]any](pluginsdk.MethodConfig{Name: "records.echo", Kind: capability.Query, Target: "installation", Description: "Read original JSON evidence."})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := capability.New(capability.Config{Reference: "records", Composition: capability.Selection, Scope: "installation"}, method.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	owner := &streamRuntime{input: make(chan string, 1)}
	server := newTestServerFor(t, owner)
	defer server.Close()
	client, err := runtimeclient.Connect(t.Context(), runtimeclient.RemoteConfig{Endpoint: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	id := testsupport.InstallationID(t).String()
	reply, err := client.CallPlugin(t.Context(), protocol.CallPluginRequest{InstallationID: id, Digest: testsupport.Digest("1").String(), Contribution: "records", Contract: string(contract.Identity().JSON()), Method: method.Name(), Input: source, Target: capability.Target{Scope: "installation", Reference: id}}, protocol.PluginCallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if reply.Output != source || <-owner.input != source {
		t.Fatal("Core transport changed original feature numbers or receipt lexemes")
	}
}

func (s *streamRuntime) Discover(ctx context.Context) (*protocol.DiscoverResponse, error) {
	result, err := s.fakeRuntime.Discover(ctx)
	result.Capabilities.Features[protocol.FeaturePlugins] = protocol.FeatureCapability{Enabled: true}
	result.Capabilities.StreamingMethods = []string{"plugins.stream"}
	return result, err
}

func (s *streamRuntime) PreparePluginStream(_ context.Context, request protocol.CallPluginRequest) (delivery.OperationKind, func(context.Context) (struct{}, iter.Seq2[protocol.PluginStreamEvent, error], error), error) {
	contract, err := capability.Parse([]byte(request.Contract))
	if err != nil {
		return "", nil, err
	}
	method, found := contract.Method(request.Method)
	if !found {
		return "", nil, errors.New("missing stream method")
	}
	kind := delivery.OperationQuery
	if method.Kind() == capability.Mutation {
		kind = delivery.OperationCommand
	}
	return kind, func(ctx context.Context) (struct{}, iter.Seq2[protocol.PluginStreamEvent, error], error) {
		return s.StreamPlugin(ctx, request)
	}, nil
}

func (s *streamRuntime) StreamPlugin(ctx context.Context, _ protocol.CallPluginRequest) (struct{}, iter.Seq2[protocol.PluginStreamEvent, error], error) {
	s.identity = operation.Identity(ctx)
	return struct{}{}, func(yield func(protocol.PluginStreamEvent, error) bool) {
		s.starts.Add(1)
		if !yield(protocol.PluginStreamEvent{Kind: protocol.PluginStreamChunk, Value: `18446744073709551615`}, nil) {
			return
		}
		if s.awaitCancellation {
			<-ctx.Done()
			close(s.canceled)
			return
		}
		if s.terminal.Kind != "" {
			yield(s.terminal, nil)
		}
	}, nil
}

func TestRemotePluginCommandStreamDistinguishesFinalRefusalAndMissingTerminal(t *testing.T) {
	for _, test := range []struct {
		name     string
		terminal protocol.PluginStreamEvent
		unknown  bool
	}{
		{name: "result", terminal: protocol.PluginStreamEvent{Kind: protocol.PluginStreamResult, Value: `true`}},
		{name: "refusal", terminal: protocol.PluginStreamEvent{Kind: protocol.PluginStreamFailure, Failure: &protocol.PluginCallFailure{Code: "refused", Message: "owner refused"}}},
		{name: "missing terminal", unknown: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			owner := &streamRuntime{terminal: test.terminal}
			server := newTestServerFor(t, owner)
			defer server.Close()
			client, err := runtimeclient.Connect(t.Context(), runtimeclient.RemoteConfig{Endpoint: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			request := protocol.CallPluginRequest{InstallationID: testsupport.InstallationID(t).String(), Digest: testsupport.Digest("1").String(), Contribution: "records", Contract: string(testsupport.CommandContract(t, "records.stream", "installation").JSON()), Method: "records.stream", Input: `{}`, Target: capability.Target{Scope: "installation", Reference: testsupport.InstallationID(t).String()}}
			_, events, err := client.StreamPlugin(t.Context(), request, protocol.PluginCallOptions{IdempotencyKey: "original"})
			if err != nil {
				t.Fatal(err)
			}
			var received []protocol.PluginStreamEvent
			var streamErr error
			for event, err := range events {
				if err != nil {
					streamErr = err
					break
				}
				received = append(received, event)
			}
			if test.unknown != errors.Is(streamErr, runtimeclient.ErrAcknowledgementUnknown) || !test.unknown && streamErr != nil || len(received) == 0 || string(received[0].Value) != "18446744073709551615" || owner.starts.Load() != 1 || owner.identity == "" {
				t.Fatalf("stream=%+v %v starts=%d identity=%s", received, streamErr, owner.starts.Load(), owner.identity)
			}
			if !test.unknown && (len(received) != 2 || received[1].Kind != test.terminal.Kind) {
				t.Fatalf("terminal evidence lost: %+v", received)
			}
		})
	}
}

func TestRemotePluginQueryStreamUsesReadAdmissionAndCancellation(t *testing.T) {
	method, err := pluginsdk.NewStreamingMethod[struct{}, uint64, bool](pluginsdk.MethodConfig{Name: "records.query", Kind: capability.Query, Target: "installation", Description: "Read streamed records."})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := capability.New(capability.Config{Reference: "records", Composition: capability.Selection, Scope: "installation"}, method.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	owner := &streamRuntime{terminal: protocol.PluginStreamEvent{Kind: protocol.PluginStreamResult, Value: `true`}}
	server := newTestServerFor(t, owner)
	defer server.Close()
	client, err := runtimeclient.Connect(t.Context(), runtimeclient.RemoteConfig{Endpoint: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	request := protocol.CallPluginRequest{InstallationID: testsupport.InstallationID(t).String(), Digest: testsupport.Digest("1").String(), Contribution: "records", Contract: string(contract.Identity().JSON()), Method: "records.query", Input: `{}`, Target: capability.Target{Scope: "installation", Reference: testsupport.InstallationID(t).String()}}
	if _, _, err := client.StreamPlugin(t.Context(), request, protocol.PluginCallOptions{IdempotencyKey: "invalid-query-key"}); !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("query admitted a command identity: %v", err)
	}
	_, events, err := client.StreamPlugin(t.Context(), request, protocol.PluginCallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var received []protocol.PluginStreamEvent
	for event, err := range events {
		if err != nil {
			t.Fatal(err)
		}
		received = append(received, event)
	}
	if len(received) != 2 || string(received[0].Value) != "18446744073709551615" || received[1].Kind != protocol.PluginStreamResult || owner.starts.Load() != 1 {
		t.Fatalf("query stream=%+v calls=%d", received, owner.starts.Load())
	}
	owner.terminal = protocol.PluginStreamEvent{}
	_, missing, err := client.StreamPlugin(t.Context(), request, protocol.PluginCallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var missingErr error
	for _, err := range missing {
		if err != nil {
			missingErr = err
		}
	}
	if missingErr == nil || errors.Is(missingErr, runtimeclient.ErrAcknowledgementUnknown) {
		t.Fatalf("missing query evidence acquired command uncertainty: %v", missingErr)
	}

	owner.awaitCancellation = true
	owner.canceled = make(chan struct{})
	_, events, err = client.StreamPlugin(t.Context(), request, protocol.PluginCallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for event, err := range events {
		if err != nil || event.Kind != protocol.PluginStreamChunk {
			t.Fatalf("query cancellation opening=%+v error=%v", event, err)
		}
		break
	}
	select {
	case <-owner.canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("early query consumption did not cancel its source")
	}

}
