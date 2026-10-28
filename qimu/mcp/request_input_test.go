package mcp

import (
	"context"
	"iter"
	"testing"

	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/portable"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestBoundMethodRetainsRawNumbersBeforeTypedConversion(t *testing.T) {
	type input struct {
		N float64 `json:"n"`
	}
	method, err := NewMethod[input, struct{}](MethodConfig{Name: "example.update", Kind: portable.Mutation, Target: "installation", Description: "Update a number."})
	if err != nil {
		t.Fatal(err)
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "owner"}, nil)
	backend := testBackend(t, server)
	var captured jsonvalue.Value
	if err := method.RegisterServer(backend, nil, func(ctx context.Context, _ *sdk.ServerSession, command Command[input]) (struct{}, error) {
		var err error
		captured, err = command.RequestInput()
		if err != nil {
			return struct{}{}, err
		}
		projected, err := RequestInput(ctx)
		if err != nil || projected != captured {
			t.Fatalf("context changed request evidence: %s, %v", projected.String(), err)
		}
		return struct{}{}, nil
	}); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "caller"}, nil)
	if err := RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	session := connectPublishedBackend(t, backend, client)
	binding, err := NewBinding(session, method.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{`{"n":1.0}`, `{"n":9007199254740993}`} {
		admitted, err := jsonvalue.Parse([]byte(value))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := binding.Invoke(t.Context(), value, admitted); err != nil {
			t.Fatal(err)
		}
		if captured.String() != value {
			t.Fatalf("typed conversion replaced request identity: %s, want %s", captured.String(), value)
		}
	}
	if _, err := RequestInput(t.Context()); err == nil {
		t.Fatal("unbound context acquired request evidence")
	}
	if _, err := (Command[input]{ID: "forged", Input: input{N: 1}}).RequestInput(); err == nil {
		t.Fatal("constructed DTO acquired received input")
	}
}

func TestDynamicNumbersRemainExactThroughMethodAndReceiptCodecs(t *testing.T) {
	type payload struct {
		Value any `json:"value"`
	}
	method := testMethod[payload, payload](t, "example.numbers/echo", portable.Mutation)
	bare := testMethod[payload, payload](t, "example.numbers/direct", portable.Mutation)
	contract, err := portable.NewCapability(portable.CapabilityConfig{Reference: "example.numbers", Composition: portable.Collection, Scope: "installation"}, method.Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "exact-number-owner"}, nil)
	backend := testBackend(t, server)
	var captured, committed jsonvalue.Value
	handler := func(_ context.Context, _ *sdk.ServerSession, command Command[payload]) (payload, error) {
		var err error
		captured, err = jsonvalue.Encode(command.Input)
		if err != nil {
			return payload{}, err
		}
		committed, err = EncodeReply(command.Input, nil, nil)
		return command.Input, err
	}
	for _, operation := range []*Method[payload, payload]{method, bare} {
		if err := operation.RegisterServer(backend, nil, handler); err != nil {
			t.Fatal(err)
		}
	}
	receipts, err := ReceiptQuery()
	if err != nil {
		t.Fatal(err)
	}
	if err := receipts.RegisterServer(backend, nil, func(context.Context, *sdk.ServerSession, Command[ReceiptRequest]) (Receipt, error) {
		return Receipt{Found: true, Reply: committed.JSON()}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := backend.BindCapability("numbers", contract); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "exact-number-consumer"}, nil)
	if err := RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	session := connectPublishedBackend(t, backend, client)
	exported, err := ResolveCapability(t.Context(), session, "numbers", contract)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := method.Bind(exported)
	if err != nil {
		t.Fatal(err)
	}
	for _, literal := range []string{"9007199254740993", "1e400", "1.00"} {
		t.Run(literal, func(t *testing.T) {
			input := payload{Value: map[string]any{"nested": []any{jsonvalue.Number(literal)}}}
			expected, err := jsonvalue.Encode(input)
			if err != nil {
				t.Fatal(err)
			}
			assertExact := func(name string, actual payload, err error) {
				t.Helper()
				if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				encoded, err := jsonvalue.Encode(actual)
				if err != nil || encoded.String() != expected.String() {
					t.Fatalf("%s changed number lexemes: %s, want %s: %v", name, encoded.String(), expected.String(), err)
				}
			}
			output, err := bound.CallCommand(t.Context(), literal, input)
			assertExact("method output", output, err)
			if captured.String() != expected.String() {
				t.Fatalf("business input changed number lexemes: %s", captured.String())
			}
			output, found, err := bound.ReadReceipt(t.Context(), literal, input)
			if !found {
				t.Fatalf("bound receipt missing: %v", err)
			}
			assertExact("bound receipt", output, err)
			output, err = bare.CallCommand(t.Context(), session, literal, input)
			assertExact("direct method", output, err)
			output, found, err = ReadReceipt[payload, payload](t.Context(), session, literal, bare.Name(), input)
			if !found {
				t.Fatalf("receipt missing: %v", err)
			}
			assertExact("receipt", output, err)
		})
	}
}

func TestDynamicNumbersRemainExactThroughBothStreamingDirections(t *testing.T) {
	type payload struct {
		Value any `json:"value"`
	}
	method, err := NewStreamingMethod[payload, payload, payload](MethodConfig{Name: "example.numbers/stream", Kind: portable.Mutation, Target: "call", Description: "Stream exact numbers."})
	if err != nil {
		t.Fatal(err)
	}
	handler := func(_ context.Context, command Command[payload]) iter.Seq2[StreamValue[payload, payload], error] {
		return func(yield func(StreamValue[payload, payload], error) bool) {
			if yield(StreamValue[payload, payload]{Chunk: &command.Input}, nil) {
				yield(StreamValue[payload, payload]{Result: &command.Input}, nil)
			}
		}
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "exact-stream-owner"}, nil)
	backend := testBackend(t, server)
	if err := method.RegisterServer(backend, func(ctx context.Context, _ *sdk.ServerSession, command Command[payload]) iter.Seq2[StreamValue[payload, payload], error] {
		return handler(ctx, command)
	}); err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "exact-stream-consumer"}, nil)
	if err := RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	session := connectPublishedBackend(t, backend, client)
	callbacks := newTestCallbacks(t)
	if err := callbacks.AdmitSession(session); err != nil {
		t.Fatal(err)
	}
	binding, err := Resolve(t.Context(), session, method.Descriptor().Name)
	if err != nil {
		t.Fatal(err)
	}
	callback, err := NewStreamingCallback(method, handler)
	if err != nil {
		t.Fatal(err)
	}
	ref, release, err := callbacks.Open(t.Context(), session, callbackTestLimits, callback)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	for _, literal := range []string{"9007199254740993", "1e400", "1.00"} {
		input := payload{Value: []any{jsonvalue.Number(literal)}}
		expected, err := jsonvalue.Encode(input)
		if err != nil {
			t.Fatal(err)
		}
		for _, direction := range []struct {
			name   string
			stream iter.Seq2[StreamValue[payload, payload], error]
		}{
			{"backend", method.StreamWithMetadata(t.Context(), binding, callbacks, callbackTestLimits, literal, input, sdk.ParamsBase{})},
			{"host", StreamHost(t.Context(), ref, method, literal, input)},
		} {
			t.Run(direction.name+"/"+literal, func(t *testing.T) {
				chunks, results := 0, 0
				for item, err := range direction.stream {
					if err != nil {
						t.Fatal(err)
					}
					value := item.Chunk
					if value != nil {
						chunks++
					} else {
						value = item.Result
						results++
					}
					encoded, err := jsonvalue.Encode(value)
					if err != nil || encoded.String() != expected.String() {
						t.Fatalf("stream changed number lexemes: %s, want %s: %v", encoded.String(), expected.String(), err)
					}
				}
				if chunks != 1 || results != 1 {
					t.Fatalf("stream values: chunks=%d results=%d", chunks, results)
				}
			})
		}
	}
}
