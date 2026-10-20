//go:build darwin

package main

import (
	"context"
	json "encoding/json/v2"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/plugins/sandbox/rpc"
	"github.com/Tangerg/flame/runtime/jsoninput"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestTypedAndModelExecutionShareAdmissionAndOriginalToolReceipt(t *testing.T) {
	data, home := t.TempDir(), t.TempDir()
	db, err := openStore(t.Context(), filepath.Join(data, "sandbox.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	calls := 0
	runner, _ := newRunner(db, filepath.Join(data, "scratch"), func(context.Context) (string, error) { calls++; return home, nil })
	server, err := newServer(runner)
	if err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "sandbox-test", Version: "1"}, nil)
	if err := pluginsdk.RegisterClient(client); err != nil {
		t.Fatal(err)
	}
	a, b := sdk.NewInMemoryTransports()
	connection, err := server.Connect(t.Context(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	session, err := client.Connect(t.Context(), b, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	contracts, _ := rpc.NewContracts()
	exported, err := pluginsdk.ResolveCapability(t.Context(), session, "sandbox", contracts.Capability)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := exported.Method(contracts.Execute.Name())
	if err != nil {
		t.Fatal(err)
	}
	input := rpc.ExecuteInput{Script: "printf sandbox-result; exit 3", TimeoutMillis: 3000}
	encoded, _ := jsoninput.Encode(input)
	ctx, err := pluginsdk.WithInvocation(t.Context(), capability.Invocation{ID: "typed", Origin: capability.Human, Target: capability.Target{Scope: "installation", Reference: "sandbox"}})
	if err != nil {
		t.Fatal(err)
	}
	typed, err := bound.Invoke(ctx, "tool:model", encoded)
	if err != nil {
		t.Fatal(err)
	}
	meta := make(sdk.Meta)
	if err := meta.Set(pluginsdk.InvocationMetadataKey, capability.Invocation{ID: "model", Origin: capability.Model, Target: capability.Target{Scope: "installation", Reference: "sandbox"}}); err != nil {
		t.Fatal(err)
	}
	model, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: executeTool, Arguments: input, Meta: meta})
	if err != nil || model.IsError {
		t.Fatalf("model entry lost original result: %+v %v", model, err)
	}
	raw, err := json.Marshal(model.StructuredContent)
	projected, parseErr := jsoninput.Parse(raw)
	if err != nil || parseErr != nil || projected != typed || calls != 1 {
		t.Fatalf("model entry did not project the typed owner result: %s %s %v %v calls=%d", typed.String(), projected.String(), err, parseErr, calls)
	}
	receipt, err := pluginsdk.ReadToolReceipt(t.Context(), session, "model", executeTool, encoded)
	if err != nil || receipt == nil || calls != 1 {
		t.Fatalf("Tool receipt replayed execution or lost evidence: %+v %v calls=%d", receipt, err, calls)
	}
	stored, err := jsoninput.Parse(receipt.Details)
	if err != nil || stored != typed {
		t.Fatalf("Tool recovery changed completed output: %s %v", stored.String(), err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE commands SET reply=json_set(reply,'$.value.status','invalid') WHERE id='tool:model'`); err != nil {
		t.Fatal(err)
	}
	if _, err := pluginsdk.ReadToolReceipt(t.Context(), session, "model", executeTool, encoded); err == nil || calls != 1 {
		t.Fatalf("corrupt result became an outcome or authorized replay: %v calls=%d", err, calls)
	}
}
