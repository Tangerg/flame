package plugin

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"iter"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type implementationRecord struct {
	Status   string `json:"status"`
	Revision int    `json:"revision"`
}

func TestImplementationUsesPublishedContractInsteadOfLocalTypeIdentity(t *testing.T) {
	config := MethodConfig{Name: "reviews.update", Kind: extension.Mutation, Target: "installation", Description: "Update a review."}
	defined, err := NewMethod[contractInput, contractInput](config)
	if err != nil {
		t.Fatal(err)
	}
	owner := testCapability(t, defined.Descriptor())
	local, err := NewMethod[implementationRecord, implementationRecord](config)
	if err != nil {
		t.Fatal(err)
	}
	if local.contract.Identity() == defined.contract.Identity() {
		t.Fatal("fixture needs independently defined types with different schemas")
	}
	method, err := ImplementMethod[implementationRecord, implementationRecord](owner, defined.Name())
	if err != nil {
		t.Fatal(err)
	}
	backend := testBackend(t, sdk.NewServer(&sdk.Implementation{Name: "independent-author"}, nil))
	calls := 0
	if err := method.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command Command[implementationRecord]) (implementationRecord, error) {
		calls++
		input, err := command.RequestInput()
		if err != nil {
			t.Fatal(err)
		}
		original, err := RequestInput(ctx)
		if err != nil || original != input || command.ID != "original" {
			t.Fatalf("original command evidence changed: %v", err)
		}
		if command.Input.Revision == 2 {
			return implementationRecord{Status: "not-published", Revision: 2}, nil
		}
		return command.Input, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := backend.BindCapability("reviews", owner); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "consumer"}, nil)
	if err := RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	session := connectPublishedBackend(t, backend, client)
	binding, err := ResolveCapability(t.Context(), session, "reviews", owner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := local.Bind(binding); err == nil {
		t.Fatal("structural resemblance replaced the consumer's exact contract")
	}
	bound, err := defined.Bind(binding)
	if err != nil {
		t.Fatal(err)
	}
	input := contractInput{Status: "open", Revision: 1}
	if result, err := bound.CallCommand(t.Context(), "original", input); err != nil || result != input {
		t.Fatalf("independent implementation = %+v, %v", result, err)
	}
	for _, raw := range []string{`{"status":"not-published","revision":1}`, `{"status":"open"}`, `{"status":"open","revision":1,"extra":true}`} {
		admitted, err := jsonvalue.Parse([]byte(raw))
		if err != nil {
			t.Fatal(err)
		}
		// Exercise the implementation's server admission, independently of the
		// consumer's local schema check.
		member := backend.methods[defined.Name()]
		if _, err := member.invoke(t.Context(), nil, &invokeParams{Input: admitted.JSON(), CommandID: "original"}); err == nil {
			t.Fatalf("implementation admitted invalid published input: %s", raw)
		}
	}
	if calls != 1 {
		t.Fatalf("invalid input reached the business owner: %d calls", calls)
	}
	input.Revision = 2
	if _, err := bound.CallCommand(t.Context(), "original", input); err == nil {
		t.Fatal("invalid implementation output became success")
	} else if _, known := errors.AsType[*extension.Refusal](err); known {
		t.Fatal("invalid output became a known refusal")
	}
}

func TestStreamingImplementationRetainsPublishedInputChunkAndOutput(t *testing.T) {
	defined, err := NewStreamingMethod[contractInput, contractInput, contractInput](MethodConfig{Name: "reviews.stream", Kind: extension.Query, Target: "installation", Description: "Read reviews."})
	if err != nil {
		t.Fatal(err)
	}
	owner := testCapability(t, defined.Descriptor())
	method, err := ImplementStreamingMethod[implementationRecord, implementationRecord, implementationRecord](owner, defined.Descriptor().Name)
	if err != nil {
		t.Fatal(err)
	}
	if method.contract.Identity() != defined.contract.Identity() {
		t.Fatal("local codecs replaced the streaming contract")
	}
	for _, invalid := range []string{"", "input", "chunk", "output"} {
		t.Run(invalid, func(t *testing.T) {
			calls := 0
			bound := bindStreaming(method, func(_ context.Context, _ *sdk.ServerSession, command Command[implementationRecord]) iter.Seq2[StreamValue[implementationRecord, implementationRecord], error] {
				calls++
				return func(yield func(StreamValue[implementationRecord, implementationRecord], error) bool) {
					chunk, output := command.Input, command.Input
					if invalid == "chunk" {
						chunk.Status = "not-published"
					}
					if invalid == "output" {
						output.Status = "not-published"
					}
					if yield(StreamValue[implementationRecord, implementationRecord]{Chunk: &chunk}, nil) {
						yield(StreamValue[implementationRecord, implementationRecord]{Result: &output}, nil)
					}
				}
			})
			input := jsontext.Value(`{"status":"open","revision":1}`)
			if invalid == "input" {
				input = jsontext.Value(`{"status":"not-published","revision":1}`)
			}
			stream, err := bound.stream(t.Context(), nil, &invokeParams{Input: input})
			if invalid == "input" {
				if err == nil || calls != 0 {
					t.Fatal("invalid input reached streaming handler")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			chunks, results := 0, 0
			var failure error
			for frame, err := range stream {
				if err != nil {
					failure = err
					break
				}
				if frame.Kind == "chunk" {
					chunks++
				}
				if frame.Kind == "result" {
					results++
				}
			}
			if invalid == "" {
				if failure != nil || chunks != 1 || results != 1 {
					t.Fatalf("stream = %d chunks, %d results, %v", chunks, results, failure)
				}
			} else if failure == nil || results != 0 {
				t.Fatal("invalid stream evidence became success")
			}
		})
	}
}

func TestImplementationRequiresAnExistingMethodAndMatchingDelivery(t *testing.T) {
	unary, err := NewMethod[struct{}, bool](MethodConfig{Name: "read", Kind: extension.Query, Target: "installation", Description: "Read."})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := NewStreamingMethod[struct{}, bool, bool](MethodConfig{Name: "stream", Kind: extension.Query, Target: "installation", Description: "Stream."})
	if err != nil {
		t.Fatal(err)
	}
	owner := testCapability(t, unary.Descriptor(), stream.Descriptor())
	for _, candidate := range []struct {
		owner *extension.Capability
		name  string
	}{{nil, "read"}, {&extension.Capability{}, "read"}, {owner, "missing"}} {
		if _, err := ImplementMethod[struct{}, bool](candidate.owner, candidate.name); err == nil {
			t.Fatal("missing unary contract admitted")
		}
		if _, err := ImplementStreamingMethod[struct{}, bool, bool](candidate.owner, candidate.name); err == nil {
			t.Fatal("missing streaming contract admitted")
		}
	}
	if _, err := ImplementMethod[struct{}, bool](owner, "stream"); err == nil {
		t.Fatal("streaming method admitted as unary")
	}
	if _, err := ImplementStreamingMethod[struct{}, bool, bool](owner, "read"); err == nil {
		t.Fatal("unary method admitted as streaming")
	}
}
