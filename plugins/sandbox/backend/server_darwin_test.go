//go:build darwin

package main

import (
	"context"
	json "encoding/json/v2"
	"path/filepath"
	"testing"

	"github.com/Tangerg/frame/portable"
	framemcp "github.com/Tangerg/frame/process/mcp"
	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/flame/packages/go-sdk/extension"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	rpc "github.com/Tangerg/flame/plugins/sandbox/internal/contract"
	"github.com/Tangerg/flame/plugins/sandbox/internal/wire"
	sdk "github.com/Tangerg/go-sdk/mcp"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
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
	if err := pluginmcp.RegisterClient(client); err != nil {
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
	exported, err := pluginmcp.ResolveCapability(t.Context(), session, "sandbox", contracts.Capability)
	if err != nil {
		t.Fatal(err)
	}
	bound, err := exported.Method(contracts.Execute.Name())
	if err != nil {
		t.Fatal(err)
	}
	input := wire.ExecuteInput{Script: "printf sandbox-result; exit 3", TimeoutMillis: 3000}
	encoded, _ := framevalue.EncodeJSON(input)
	ctx, err := pluginmcp.WithInvocation(t.Context(), framemcp.Invocation{ID: "typed", Origin: extension.Human, Target: portable.Target{Scope: "installation", Reference: "sandbox"}})
	if err != nil {
		t.Fatal(err)
	}
	typed, err := bound.Invoke(ctx, "tool:model", encoded)
	if err != nil {
		t.Fatal(err)
	}
	meta := make(sdk.Meta)
	if err := meta.Set(pluginmcp.InvocationMetadataKey, framemcp.Invocation{ID: "model", Origin: extension.Model, Target: portable.Target{Scope: "installation", Reference: "sandbox"}}); err != nil {
		t.Fatal(err)
	}
	model, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: executeTool, Arguments: input, Meta: meta})
	if err != nil || model.IsError {
		t.Fatalf("model entry lost original result: %+v %v", model, err)
	}
	raw, err := json.Marshal(model.StructuredContent)
	projected, parseErr := framevalue.ParseJSON(raw)
	if err != nil || parseErr != nil || projected != typed || calls != 1 {
		t.Fatalf("model entry did not project the typed owner result: %s %s %v %v calls=%d", typed.String(), projected.String(), err, parseErr, calls)
	}
	receipt, err := pluginsdk.ReadToolReceipt(t.Context(), session, "model", executeTool, encoded)
	if err != nil || receipt == nil || calls != 1 {
		t.Fatalf("Tool receipt replayed execution or lost evidence: %+v %v calls=%d", receipt, err, calls)
	}
	stored, err := framevalue.ParseJSON(receipt.Details)
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
