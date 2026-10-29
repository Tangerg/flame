package mcp

import (
	"context"
	"errors"
	"iter"
	"sync/atomic"
	"testing"

	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/portable"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestInputValidationUsesTypedUnaryAndStreamContractsWithoutDispatch(t *testing.T) {
	type input struct {
		Revision int64 `json:"revision"`
	}
	unary, err := NewMethod[input, bool](MethodConfig{Name: "example.records/update", Kind: portable.Mutation, Target: "installation", Description: "Update a record."})
	if err != nil {
		t.Fatal(err)
	}
	query, err := NewMethod[input, bool](MethodConfig{Name: "example.records/read", Kind: portable.Query, Target: "installation", Description: "Read a record."})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := NewStreamingMethod[input, string, bool](MethodConfig{Name: "example.records/stream", Kind: portable.Mutation, Target: "installation", Description: "Stream a record update."})
	if err != nil {
		t.Fatal(err)
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "typed-input-owner"}, nil)
	backend := testBackend(t, server)
	var calls atomic.Int32
	if err := unary.RegisterServer(backend, nil, func(context.Context, *sdk.ServerSession, Command[input]) (bool, error) {
		calls.Add(1)
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := query.RegisterServer(backend, nil, func(context.Context, *sdk.ServerSession, Command[input]) (bool, error) {
		calls.Add(1)
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := stream.RegisterServer(backend, func(context.Context, *sdk.ServerSession, Command[input]) iter.Seq2[StreamValue[string, bool], error] {
		calls.Add(1)
		return func(yield func(StreamValue[string, bool], error) bool) {
			yield(StreamValue[string, bool]{Result: new(true)}, nil)
		}
	}); err != nil {
		t.Fatal(err)
	}
	contract, err := portable.NewCapability(portable.CapabilityConfig{Reference: "example.records", Composition: portable.Collection, Scope: "installation"}, unary.Descriptor(), query.Descriptor(), stream.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	if err := backend.BindCapability("records", contract); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "typed-admission-caller"}, nil)
	if err := RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	session := connectPublishedBackend(t, backend, client)
	exported, err := ResolveCapability(t.Context(), session, "records", contract)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{unary.Name(), query.Name(), stream.Descriptor().Name} {
		binding, err := exported.Method(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, scenario := range []struct {
			number string
			shape  bool
		}{{"0.0", true}, {"1e0", true}, {"9223372036854775808", false}} {
			raw, err := jsonvalue.Parse([]byte(`{"revision":` + scenario.number + `}`))
			if err != nil {
				t.Fatal(err)
			}
			if err := binding.method.ValidateInput(raw); (err == nil) != scenario.shape {
				t.Fatalf("unexpected structural boundary for %s: %v", raw.String(), err)
			}
			if err := binding.ValidateInput(t.Context(), raw); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("%s accepted typed-invalid %s: %v", name, raw.String(), err)
			}
		}
		valid, err := jsonvalue.Parse([]byte(`{"revision":9007199254740993}`))
		if err != nil {
			t.Fatal(err)
		}
		if err := binding.ValidateInput(t.Context(), valid); err != nil {
			t.Fatalf("%s refused a valid wide integer: %v", name, err)
		}
		canceled, cancel := context.WithCancel(t.Context())
		cancel()
		if err := binding.ValidateInput(canceled, valid); err == nil || errors.Is(err, ErrInvalidInput) {
			t.Fatalf("transport cancellation became a definite input refusal: %v", err)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("input validation entered business dispatch %d times", calls.Load())
	}
}

func TestPublishedMethodImplementationsRefuseAmbiguousCodecs(t *testing.T) {
	type embedded struct {
		Value string `json:"value"`
	}
	type ambiguous struct {
		embedded
		Other string `json:"value"`
	}
	method := testMethod[struct{}, struct{}](t, "example.codec/read", portable.Query)
	stream, err := NewStreamingMethod[struct{}, struct{}, struct{}](MethodConfig{Name: "example.codec/stream", Kind: portable.Query, Target: "installation", Description: "Read codec values."})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := portable.NewCapability(portable.CapabilityConfig{Reference: "example.codec", Composition: portable.Collection, Scope: "installation"}, method.Descriptor(), stream.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ImplementMethod[ambiguous, struct{}](owner, method.Name()); err == nil {
		t.Fatal("ambiguous input codec acquired a published method")
	}
	if _, err := ImplementMethod[struct{}, ambiguous](owner, method.Name()); err == nil {
		t.Fatal("ambiguous output codec acquired a published method")
	}
	if _, err := ImplementStreamingMethod[struct{}, ambiguous, struct{}](owner, stream.Descriptor().Name); err == nil {
		t.Fatal("ambiguous chunk codec acquired a published stream")
	}
}
