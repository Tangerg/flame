package mcp

import (
	"context"
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
