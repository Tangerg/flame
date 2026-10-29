package runtime

import (
	"context"
	"errors"
	"iter"
	"testing"

	runtimeclient "github.com/Tangerg/flame/packages/go-sdk/client"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

type typedStreamCaller struct {
	request protocol.CallPluginRequest
	options protocol.PluginCallOptions
	calls   int
	events  []protocol.PluginStreamEvent
	err     error
}

func (c *typedStreamCaller) StreamPlugin(_ context.Context, request protocol.CallPluginRequest, options protocol.PluginCallOptions) (struct{}, iter.Seq2[protocol.PluginStreamEvent, error], error) {
	c.calls++
	c.request, c.options = request, options
	return struct{}{}, func(yield func(protocol.PluginStreamEvent, error) bool) {
		for _, event := range c.events {
			if !yield(event, nil) {
				return
			}
		}
		if c.err != nil {
			yield(protocol.PluginStreamEvent{}, c.err)
		}
	}, nil
}

func bindTypedStream(t *testing.T, caller runtimeclient.PluginStreamer) *runtimeclient.PluginStreamingMethod[map[string]string, uint64, bool] {
	t.Helper()
	method, err := qimumcp.NewStreamingMethod[map[string]string, uint64, bool](qimumcp.MethodConfig{Name: "records.stream", Kind: portable.Mutation, Target: extension.ScopeInstallation, Description: "Stream original records."})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := portable.NewCapability(portable.CapabilityConfig{Reference: "records", Composition: portable.Selection, Scope: extension.ScopeInstallation}, method.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	id := testsupport.InstallationID(t).String()
	bound, err := runtimeclient.BindPluginStreamingMethod(caller, runtimeclient.PluginBinding{InstallationID: id, Digest: testsupport.Digest("1").String(), Contribution: "records", Contract: contract, Target: portable.Target{Scope: extension.ScopeInstallation, Reference: id}}, method)
	if err != nil {
		t.Fatal(err)
	}
	return bound
}

func TestTypedInstalledStreamFreezesBeforeLazySingleConsumption(t *testing.T) {
	caller := &typedStreamCaller{events: []protocol.PluginStreamEvent{{Kind: protocol.PluginStreamChunk, Value: `18446744073709551615`}, {Kind: protocol.PluginStreamResult, Value: `true`}}}
	bound := bindTypedStream(t, caller)
	input := map[string]string{"value": "original"}
	info := &protocol.ClientInfo{Name: "original", Version: "1"}
	stream := bound.Stream(t.Context(), input, protocol.PluginCallOptions{IdempotencyKey: "original", RequestMeta: protocol.RequestMeta{ClientInfo: info}})
	input["value"], info.Name = "replacement", "replacement"
	if caller.calls != 0 {
		t.Fatal("lazy stream performed I/O before consumption")
	}
	var values []qimumcp.StreamValue[uint64, bool]
	for value, err := range stream {
		if err != nil {
			t.Fatal(err)
		}
		values = append(values, value)
	}
	if len(values) != 2 || values[0].Chunk == nil || *values[0].Chunk != ^uint64(0) || values[1].Result == nil || !*values[1].Result || string(caller.request.Input) != `{"value":"original"}` || caller.options.RequestMeta.ClientInfo.Name != "original" {
		t.Fatalf("captured stream=%+v request=%s metadata=%+v", values, caller.request.Input, caller.options.RequestMeta)
	}
	for _, err := range stream {
		if !errors.Is(err, runtimeclient.ErrClosed) {
			t.Fatalf("second consumption=%v", err)
		}
	}
	if caller.calls != 1 {
		t.Fatalf("original stream invoked %d times", caller.calls)
	}
}

func TestTypedInstalledStreamDistinguishesRefusalFromInvalidOrMissingFinal(t *testing.T) {
	for _, test := range []struct {
		name   string
		events []protocol.PluginStreamEvent
		err    error
		known  bool
	}{
		{name: "known refusal", events: []protocol.PluginStreamEvent{{Kind: protocol.PluginStreamFailure, Failure: &protocol.PluginCallFailure{Code: "refused", Message: "owner refused"}}}, known: true},
		{name: "invalid chunk", events: []protocol.PluginStreamEvent{{Kind: protocol.PluginStreamChunk, Value: `"invalid"`}}},
		{name: "missing final", events: []protocol.PluginStreamEvent{{Kind: protocol.PluginStreamChunk, Value: `1`}}},
		{name: "late failure", events: []protocol.PluginStreamEvent{{Kind: protocol.PluginStreamResult, Value: `true`}}, err: errors.Join(runtimeclient.ErrAcknowledgementUnknown, errors.New("late source failure"))},
	} {
		t.Run(test.name, func(t *testing.T) {
			caller := &typedStreamCaller{events: test.events, err: test.err}
			stream := bindTypedStream(t, caller).Stream(t.Context(), map[string]string{}, protocol.PluginCallOptions{IdempotencyKey: "original"})
			var failure error
			for value, err := range stream {
				if value.Result != nil {
					t.Fatal("invalid source published a final result")
				}
				if err != nil {
					failure = err
				}
			}
			_, known := errors.AsType[*portable.Refusal](failure)
			if failure == nil || known != test.known || errors.Is(failure, runtimeclient.ErrAcknowledgementUnknown) == test.known || caller.calls != 1 {
				t.Fatalf("outcome=%v known=%v calls=%d", failure, known, caller.calls)
			}
		})
	}
}

func TestTypedInstalledQueryStreamRetainsWideValuesWithoutCommandUncertainty(t *testing.T) {
	method, err := qimumcp.NewStreamingMethod[struct{}, uint64, bool](qimumcp.MethodConfig{Name: "records.query", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Read streamed records."})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := portable.NewCapability(portable.CapabilityConfig{Reference: "records", Composition: portable.Selection, Scope: extension.ScopeInstallation}, method.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	for _, complete := range []bool{true, false} {
		caller := &typedStreamCaller{events: []protocol.PluginStreamEvent{{Kind: protocol.PluginStreamChunk, Value: `18446744073709551615`}}}
		if complete {
			caller.events = append(caller.events, protocol.PluginStreamEvent{Kind: protocol.PluginStreamResult, Value: `true`})
		}
		id := testsupport.InstallationID(t).String()
		bound, err := runtimeclient.BindPluginStreamingMethod(caller, runtimeclient.PluginBinding{InstallationID: id, Digest: testsupport.Digest("1").String(), Contribution: "records", Contract: contract, Target: portable.Target{Scope: extension.ScopeInstallation, Reference: id}}, method)
		if err != nil {
			t.Fatal(err)
		}
		chunks, finals := 0, 0
		var streamErr error
		for value, err := range bound.Stream(t.Context(), struct{}{}, protocol.PluginCallOptions{}) {
			if err != nil {
				streamErr = err
				break
			}
			if value.Chunk != nil {
				chunks++
				if *value.Chunk != ^uint64(0) {
					t.Fatal("query stream narrowed the original uint64")
				}
			}
			if value.Result != nil {
				finals++
			}
		}
		if chunks != 1 || (finals == 1) != complete || (streamErr == nil) != complete || errors.Is(streamErr, runtimeclient.ErrAcknowledgementUnknown) || caller.options.IdempotencyKey != "" {
			t.Fatalf("query stream acquired command semantics: chunks=%d finals=%d error=%v options=%+v", chunks, finals, streamErr, caller.options)
		}
	}
}
