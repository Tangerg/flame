package main

import (
	json "encoding/json/v2"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/plugins/a2a/internal/wire"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

func TestTypedAndModelEntriesShareTaskBindingsAndOriginalReceipts(t *testing.T) {
	remote := newRemoteFixture(t)
	c := newTestConnector(t, filepath.Join(t.TempDir(), "a2a.db"), remote.spec())
	server, err := newServer(c)
	if err != nil {
		t.Fatal(err)
	}
	client := sdk.NewClient(&sdk.Implementation{Name: "a2a-test", Version: "1"}, nil)
	if err := qimumcp.RegisterClient(client); err != nil {
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
	contracts := testContracts(t)
	exported, err := qimumcp.ResolveCapability(t.Context(), session, "remote-agents", contracts.Capability)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := qimumcp.WithInvocation(t.Context(), portable.Invocation{ID: "human-send", Origin: extension.Human, Target: portable.Target{Scope: extension.ScopeInstallation, Reference: "a2a"}})
	if err != nil {
		t.Fatal(err)
	}
	send, err := exported.Method(contracts.Send.Name())
	if err != nil {
		t.Fatal(err)
	}
	request := wire.SendRequest{Agent: "research", Message: "work"}
	input := inputValue(t, request)
	typed, err := send.Invoke(ctx, "send-command", input)
	if err != nil {
		t.Fatal(err)
	}
	var typedResult wire.Result
	if err := json.Unmarshal(typed.JSON(), &typedResult); err != nil {
		t.Fatal(err)
	}
	meta := make(sdk.Meta)
	if err := meta.Set(qimumcp.InvocationMetadataKey, portable.Invocation{ID: "model-send", Origin: extension.Model, Target: portable.Target{Scope: extension.ScopeInstallation, Reference: "a2a"}}); err != nil {
		t.Fatal(err)
	}
	var modelResult wire.Result
	for range 2 {
		model, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: "a2a_send", Arguments: request, Meta: meta})
		if err != nil || model.IsError {
			t.Fatalf("model send: %+v %v", model, err)
		}
		encoded, err := json.Marshal(model.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(encoded, &modelResult); err != nil {
			t.Fatal(err)
		}
	}
	if typedResult.RequestID != "send-command" || modelResult.RequestID != "tool:model-send" || typedResult.TaskID != modelResult.TaskID || typedResult.ContextID != modelResult.ContextID || typedResult.State != modelResult.State {
		t.Fatalf("entry results differ: %+v %+v", typedResult, modelResult)
	}
	receipt, err := pluginsdk.ReadToolReceipt(ctx, session, "model-send", "a2a_send", input)
	if err != nil || receipt == nil {
		t.Fatalf("model original receipt: %+v %v", receipt, err)
	}
	statusMethod, err := exported.Method(contracts.Status.Name())
	if err != nil {
		t.Fatal(err)
	}
	statusValue, err := statusMethod.Invoke(ctx, "", inputValue(t, wire.TaskRequest{RequestID: typedResult.RequestID}))
	if err != nil {
		t.Fatal(err)
	}
	var status wire.Result
	if err := json.Unmarshal(statusValue.JSON(), &status); err != nil || status.TaskID != typedResult.TaskID {
		t.Fatalf("shared task binding: %+v %v", status, err)
	}
	agentsMethod, err := exported.Method(contracts.Agents.Name())
	if err != nil {
		t.Fatal(err)
	}
	agentsValue, err := agentsMethod.Invoke(ctx, "", inputValue(t, struct{}{}))
	if err != nil {
		t.Fatal(err)
	}
	var agents wire.Agents
	if err := json.Unmarshal(agentsValue.JSON(), &agents); err != nil || len(agents.Agents) != 1 || agents.Agents[0].Name != "research" {
		t.Fatalf("configured agents: %+v %v", agents, err)
	}
	remote.mu.Lock()
	sends := remote.calls["SendMessage"]
	remote.mu.Unlock()
	if sends != 2 {
		t.Fatalf("typed/model receipt recovery resent: %d", sends)
	}
	failed, err := session.CallTool(t.Context(), &sdk.CallToolParams{Name: "a2a_status", Arguments: wire.TaskRequest{RequestID: "missing"}, Meta: meta})
	if err != nil || !failed.IsError {
		t.Fatalf("missing task should be a definite query failure: %+v %v", failed, err)
	}
}
