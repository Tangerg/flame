package delivery

import (
	"context"
	jsonv1 "encoding/json"
	"errors"
	"math"
	"testing"

	"github.com/Tangerg/scope/core/chat"

	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/protocol"
)

// toolRegistryFake is the diagnostic tool registry the tools coordinator drives.
type toolRegistryFake struct {
	tools          []tool.Tool
	invokedCWD     string
	invokedName    string
	invokedPayload tool.Arguments
}

func (t *toolRegistryFake) List(context.Context) ([]tool.Tool, error) { return t.tools, nil }

func (t *toolRegistryFake) Invoke(_ context.Context, in workspaceapp.DiagnosticToolInvocation) (tool.Result, error) {
	t.invokedCWD = in.CWD
	t.invokedName = in.Name
	t.invokedPayload = in.Arguments
	return tool.StringResult("ok"), nil
}

func TestListToolsMapsRegisteredToolsToWire(t *testing.T) {
	s := handlerWithTools(&toolRegistryFake{tools: []tool.Tool{
		{
			ToolDefinition: chat.ToolDefinition{
				Name: "shell", Description: "run a command",
				InputSchema: []byte(`{"type":"object","properties":{"cmd":{"type":"string"},"limit":{"maximum":9007199254740993}}}`),
			},
			SafetyClass: tool.SafetyClassExec,
		},
	}})

	page, err := s.ListTools(context.Background())
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(page.Data) != 1 {
		t.Fatalf("tools = %+v, want 1", page.Data)
	}
	if page.Data[0].Name != "shell" || page.Data[0].SafetyClass != protocol.SafetyClassExec {
		t.Fatalf("shell wire = %+v", page.Data[0])
	}
	if page.Data[0].Parameters["type"] != "object" {
		t.Fatalf("schema = %+v, want decoded object schema", page.Data[0].Parameters)
	}
	limit := page.Data[0].Parameters["properties"].(map[string]any)["limit"].(map[string]any)
	if limit["maximum"] != jsonv1.Number("9007199254740993") {
		t.Fatalf("schema lost exact numeric bound: %v", limit)
	}
	page.Data[0].Parameters["type"] = "array"
	next, err := s.ListTools(t.Context())
	if err != nil || next.Data[0].Parameters["type"] != "object" {
		t.Fatalf("catalog projection exposed producer storage: %+v, %v", next, err)
	}
}

func TestInvokeToolPassesJSONArgumentsToRuntime(t *testing.T) {
	rt := &toolRegistryFake{}
	s := handlerWithTools(rt)

	got, err := s.InvokeTool(context.Background(), protocol.InvokeToolRequest{
		Name:      "read",
		Arguments: map[string]any{"path": "main.go"},
		Workspace: &protocol.WorkspaceRef{Path: "/workspace"},
	})
	if err != nil {
		t.Fatalf("invoke tool: %v", err)
	}
	if got != "ok" {
		t.Fatalf("result = %v, want ok", got)
	}
	if rt.invokedName != "read" || rt.invokedCWD != "/workspace" {
		t.Fatalf("invocation = %q in %q, want read in /workspace", rt.invokedName, rt.invokedCWD)
	}
	payload := rt.invokedPayload.Map()
	if payload["path"] != "main.go" {
		t.Fatalf("payload = %+v, want path=main.go", payload)
	}
}

func TestInvokeToolRejectsUnrepresentableArgumentsBeforeApplication(t *testing.T) {
	rt := &toolRegistryFake{}
	s := handlerWithTools(rt)

	_, err := s.InvokeTool(context.Background(), protocol.InvokeToolRequest{
		Name: "read", Arguments: map[string]any{"depth": math.Inf(1)},
	})
	if !errors.Is(err, protocol.ErrInvalidParams) {
		t.Fatalf("InvokeTool error = %v, want invalid params", err)
	}
	if rt.invokedName != "" {
		t.Fatal("application was invoked with unrepresentable arguments")
	}
}
