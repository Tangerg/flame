package plugin

import (
	"context"
	"encoding/json/jsontext"
	"testing"

	contractsdk "github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/Tangerg/flame/packages/go-sdk/jsoninput"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/tool"
)

func TestBoundMethodRetainsRawNumbersBeforeTypedConversion(t *testing.T) {
	type input struct {
		N float64 `json:"n"`
	}
	method, err := NewMethod[input, struct{}](MethodConfig{Name: "example.update", Kind: contractsdk.Mutation, Target: "installation", Description: "Update a number."})
	if err != nil {
		t.Fatal(err)
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "owner"}, nil)
	backend := testBackend(t, server)
	var captured jsoninput.Input
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
	for _, value := range []string{`{"n":1.0}`, `{"n":9007199254740993}`} {
		if _, err := CallCommand[jsontext.Value, struct{}](t.Context(), session, method.Name(), value, jsontext.Value(value)); err != nil {
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

func TestToolBoundaryRetainsOriginalArgumentsAndOptionalEmptyObject(t *testing.T) {
	type input struct {
		N float64 `json:"n"`
	}
	var captured jsoninput.Input
	read := func(ctx context.Context) (string, error) {
		var err error
		captured, err = RequestInput(ctx)
		return captured.String(), err
	}
	number, err := tool.NewFunc(tool.FuncConfig{Name: "number", Description: "Read a number."}, func(ctx context.Context, _ input) (string, error) { return read(ctx) })
	if err != nil {
		t.Fatal(err)
	}
	empty, err := tool.NewFunc(tool.FuncConfig{Name: "empty", Description: "Read no arguments."}, func(ctx context.Context, _ struct{}) (string, error) { return read(ctx) })
	if err != nil {
		t.Fatal(err)
	}
	server := sdk.NewServer(&sdk.Implementation{Name: "owner"}, nil)
	if err := RegisterTools(server, number, empty); err != nil {
		t.Fatal(err)
	}
	session := connectMethodSessions(t, server, sdk.NewClient(&sdk.Implementation{Name: "caller"}, nil))
	if result, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: "number", Arguments: jsontext.Value(`{"n":1.0}`)}); err != nil || result.IsError || captured.String() != `{"n":1.0}` {
		t.Fatalf("Tool lost original numeric input: %s, %v", captured.String(), err)
	}
	if result, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: "empty"}); err != nil || result.IsError || captured.String() != `{}` {
		t.Fatalf("optional Tool arguments = %s, %v", captured.String(), err)
	}
}
