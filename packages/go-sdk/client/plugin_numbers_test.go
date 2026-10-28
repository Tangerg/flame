package client_test

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"iter"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/client"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/scope/core/jsonschema"
)

const numericInstallation = "11111111-1111-4111-8111-111111111111"
const originalNumbers = `{"wide":9007199254740993,"fraction":1.2300,"exponent":1e+400}`

type numericPlugin struct{ streamCalls *int }

func (numericPlugin) CallPlugin(context.Context, protocol.CallPluginRequest, protocol.PluginCallOptions) (*protocol.CallPluginResponse, error) {
	return &protocol.CallPluginResponse{Output: originalNumbers}, nil
}

func (p numericPlugin) StreamPlugin(context.Context, protocol.CallPluginRequest, protocol.PluginCallOptions) (struct{}, iter.Seq2[protocol.PluginStreamEvent, error], error) {
	if p.streamCalls != nil {
		*p.streamCalls++
	}
	return struct{}{}, func(yield func(protocol.PluginStreamEvent, error) bool) {
		if !yield(protocol.PluginStreamEvent{Kind: protocol.PluginStreamChunk, Value: originalNumbers}, nil) {
			return
		}
		yield(protocol.PluginStreamEvent{Kind: protocol.PluginStreamResult, Value: originalNumbers}, nil)
	}, nil
}

func (numericPlugin) ResolvePluginPoint(context.Context, protocol.ResolvePluginPointRequest, protocol.CallOptions) (*protocol.Page[protocol.PluginPointBinding], error) {
	return protocol.NewPage([]protocol.PluginPointBinding{{
		InstallationID: numericInstallation,
		Digest:         identity.DigestStrings("numbers").String(),
		Contribution:   "numbers",
		Value:          originalNumbers,
	}}), nil
}

func numericBinding(t *testing.T, descriptor portable.MethodDescriptor) client.PluginBinding {
	t.Helper()
	contract, err := portable.NewCapability(portable.CapabilityConfig{
		Reference: "numbers", Composition: portable.Selection, Scope: extension.ScopeInstallation,
	}, descriptor)
	if err != nil {
		t.Fatal(err)
	}
	return client.PluginBinding{
		InstallationID: numericInstallation,
		Digest:         identity.DigestStrings("numbers").String(),
		Contribution:   "numbers", Contract: contract,
		Target: portable.Target{Scope: extension.ScopeInstallation, Reference: numericInstallation},
	}
}

func assertOriginalNumbers(t *testing.T, value map[string]any) {
	t.Helper()
	for key, literal := range map[string]string{"wide": "9007199254740993", "fraction": "1.2300", "exponent": "1e+400"} {
		encoded, err := json.Marshal(value[key])
		if err != nil || string(encoded) != literal {
			t.Fatalf("%s lost original numeric evidence: %s, %v", key, encoded, err)
		}
	}
}

func TestBoundPluginCallPreservesUnstructuredNumbers(t *testing.T) {
	method, err := qimumcp.NewMethod[struct{}, map[string]any](qimumcp.MethodConfig{
		Name: "numbers.read", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Read original numbers.",
	})
	if err != nil {
		t.Fatal(err)
	}
	bound, err := client.BindPluginMethod(numericPlugin{}, numericBinding(t, method.Descriptor()), method)
	if err != nil {
		t.Fatal(err)
	}
	value, err := bound.Call(t.Context(), struct{}{}, protocol.PluginCallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	assertOriginalNumbers(t, value)
}

func TestBoundPluginStreamPreservesUnstructuredNumbers(t *testing.T) {
	method, err := qimumcp.NewStreamingMethod[struct{}, map[string]any, map[string]any](qimumcp.MethodConfig{
		Name: "numbers.stream", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Stream original numbers.",
	})
	if err != nil {
		t.Fatal(err)
	}
	bound, err := client.BindPluginStreamingMethod(numericPlugin{}, numericBinding(t, method.Descriptor()), method)
	if err != nil {
		t.Fatal(err)
	}
	chunks, results := 0, 0
	for value, err := range bound.Stream(t.Context(), struct{}{}, protocol.PluginCallOptions{}) {
		if err != nil {
			t.Fatal(err)
		}
		if value.Chunk != nil {
			chunks++
			assertOriginalNumbers(t, *value.Chunk)
		}
		if value.Result != nil {
			results++
			assertOriginalNumbers(t, *value.Result)
		}
	}
	if chunks != 1 || results != 1 {
		t.Fatalf("stream evidence: %d chunks, %d results", chunks, results)
	}
}

func TestPointValuesPreserveUnstructuredNumbers(t *testing.T) {
	schema, err := jsonschema.For[map[string]any]()
	if err != nil {
		t.Fatal(err)
	}
	contract, err := portable.AdmitPoint(portable.PointDescriptor{
		Protocol: portable.Protocol, Reference: "numbers", Composition: portable.Collection,
		Scope: extension.ScopeInstallation, Ordering: portable.CanonicalKeys,
		Value: schema.JSON(), Applicability: jsontext.Value(`{}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	values, err := client.ResolvePointValues[map[string]any](t.Context(), numericPlugin{}, contract,
		portable.Target{Scope: extension.ScopeInstallation, Reference: numericInstallation}, struct{}{}, nil, protocol.CallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 {
		t.Fatalf("point values: %d", len(values))
	}
	assertOriginalNumbers(t, values[0].Value)
}

func TestBoundPluginQueryStreamRefusesCommandIdentityBeforeDispatch(t *testing.T) {
	method, err := qimumcp.NewStreamingMethod[struct{}, map[string]any, map[string]any](qimumcp.MethodConfig{
		Name: "numbers.stream", Kind: portable.Query, Target: extension.ScopeInstallation, Description: "Stream original numbers.",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, options := range []protocol.PluginCallOptions{{IdempotencyKey: "command"}, {IdempotencyNamespace: "store"}} {
		calls := 0
		bound, err := client.BindPluginStreamingMethod(numericPlugin{streamCalls: &calls}, numericBinding(t, method.Descriptor()), method)
		if err != nil {
			t.Fatal(err)
		}
		failures := 0
		for value, err := range bound.Stream(t.Context(), struct{}{}, options) {
			if err == nil || value.Chunk != nil || value.Result != nil {
				t.Fatalf("query with a command identity yielded evidence: %+v, %v", value, err)
			}
			failures++
		}
		if failures != 1 || calls != 0 {
			t.Fatalf("query identity admission: %d failures, %d dispatches", failures, calls)
		}
	}
}
