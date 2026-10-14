package http_test

import (
	"context"
	"errors"
	"iter"
	"sync/atomic"
	"testing"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/protocol"
)

type streamRuntime struct {
	fakeRuntime
	terminal protocol.PluginStreamEvent
	starts   atomic.Int32
	identity string
}

func (s *streamRuntime) Discover(ctx context.Context) (*protocol.DiscoverResponse, error) {
	result, err := s.fakeRuntime.Discover(ctx)
	result.Capabilities.Features[protocol.FeaturePlugins] = protocol.FeatureCapability{Enabled: true}
	result.Capabilities.StreamingMethods = []string{"plugins.stream"}
	return result, err
}

func (s *streamRuntime) StreamPlugin(ctx context.Context, _ protocol.CallPluginRequest) (struct{}, iter.Seq2[protocol.PluginStreamEvent, error], error) {
	s.identity = operation.Identity(ctx)
	return struct{}{}, func(yield func(protocol.PluginStreamEvent, error) bool) {
		s.starts.Add(1)
		if !yield(protocol.PluginStreamEvent{Kind: protocol.PluginStreamChunk, Value: []byte(`18446744073709551615`)}, nil) {
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
		{name: "result", terminal: protocol.PluginStreamEvent{Kind: protocol.PluginStreamResult, Value: []byte(`true`)}},
		{name: "refusal", terminal: protocol.PluginStreamEvent{Kind: protocol.PluginStreamFailure, Failure: &protocol.PluginCallFailure{Code: "refused", Message: "owner refused"}}},
		{name: "missing terminal", unknown: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			owner := &streamRuntime{terminal: test.terminal}
			server := newTestServerFor(t, owner)
			defer server.Close()
			client, err := flameruntime.Connect(t.Context(), flameruntime.RemoteConfig{Endpoint: server.URL})
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			request := protocol.CallPluginRequest{InstallationID: testsupport.InstallationID(t).String(), Digest: testsupport.Digest("1").String(), Contribution: "records", Contract: string(testsupport.CommandContract(t, "records.stream", "installation").JSON()), Method: "records.stream", Input: []byte(`{}`), Target: capability.Target{Scope: "installation", Reference: testsupport.InstallationID(t).String()}}
			_, events, err := client.StreamPlugin(t.Context(), request, flameruntime.CommandOptions{IdempotencyKey: "original"})
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
			if test.unknown != errors.Is(streamErr, flameruntime.ErrAcknowledgementUnknown) || !test.unknown && streamErr != nil || len(received) == 0 || string(received[0].Value) != "18446744073709551615" || owner.starts.Load() != 1 || owner.identity == "" {
				t.Fatalf("stream=%+v %v starts=%d identity=%s", received, streamErr, owner.starts.Load(), owner.identity)
			}
			if !test.unknown && (len(received) != 2 || received[1].Kind != test.terminal.Kind) {
				t.Fatalf("terminal evidence lost: %+v", received)
			}
		})
	}
}
