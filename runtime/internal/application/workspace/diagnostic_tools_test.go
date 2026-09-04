package workspace

import (
	"context"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

type toolRegistryFixture struct {
	tools []tool.Tool
}

func (t toolRegistryFixture) List(context.Context) ([]tool.Tool, error) { return t.tools, nil }
func (toolRegistryFixture) Invoke(context.Context, string, string, tool.Arguments) (tool.Result, error) {
	return tool.Result{}, nil
}

type toolRegistryRecorder struct {
	root      string
	name      string
	arguments tool.Arguments
}

func (t *toolRegistryRecorder) Invoke(_ context.Context, root, name string, arguments tool.Arguments) (tool.Result, error) {
	t.root = root
	t.name = name
	t.arguments = arguments
	return tool.StringResult("ok"), nil
}

func (*toolRegistryRecorder) List(context.Context) ([]tool.Tool, error) { return nil, nil }

type rootRecorder struct {
	root string
}

func (r *rootRecorder) ResolveRoot(cwd string) (string, error) {
	r.root = cwd
	return "/workspace", nil
}

func TestListOwnsSafeUniqueNameOrder(t *testing.T) {
	registry := toolRegistryFixture{tools: []tool.Tool{
		{Name: "read", SafetyClass: tool.SafetyClassSafe},
		{Name: "glob", SafetyClass: tool.SafetyClassSafe},
	}}
	c := NewDiagnosticTools(registry, &rootRecorder{})

	got, err := c.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 || got[0].Name != "glob" || got[1].Name != "read" {
		t.Fatalf("tools = %+v, want glob then read", got)
	}
	if registry.tools[0].Name != "read" {
		t.Fatal("List reordered registry-owned storage")
	}
	got[0].Name = "mutated"
	if registry.tools[1].Name != "glob" {
		t.Fatal("List aliases registry-owned storage")
	}
}

func TestListRejectsInvalidCatalogs(t *testing.T) {
	for name, tools := range map[string][]tool.Tool{
		"empty name":          {{SafetyClass: tool.SafetyClassSafe}},
		"padded name":         {{Name: " read ", SafetyClass: tool.SafetyClassSafe}},
		"invalid description": {{Name: "read", Description: string([]byte{0xff}), SafetyClass: tool.SafetyClassSafe}},
		"unknown safety":      {{Name: "read", SafetyClass: tool.SafetyClass("future")}},
		"unsafe":              {{Name: "write", SafetyClass: tool.SafetyClassWrite}},
		"duplicate name": {
			{Name: "read", SafetyClass: tool.SafetyClassSafe},
			{Name: "read", SafetyClass: tool.SafetyClassSafe},
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := NewDiagnosticTools(toolRegistryFixture{tools: tools}, &rootRecorder{})
			if _, err := c.List(context.Background()); err == nil {
				t.Fatal("invalid diagnostic tool catalog was accepted")
			}
		})
	}
}

func TestInvokeUsesRegistry(t *testing.T) {
	invoker := &toolRegistryRecorder{}
	roots := &rootRecorder{}
	c := NewDiagnosticTools(invoker, roots)

	arguments, err := tool.ParseArguments(`{"path":"main.go"}`)
	if err != nil {
		t.Fatalf("ParseArguments: %v", err)
	}
	got, err := c.Invoke(context.Background(), DiagnosticToolInvocation{Name: "read", Arguments: arguments, CWD: "/requested"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if text, ok := got.String(); !ok || text != "ok" || roots.root != "/requested" || invoker.root != "/workspace" || invoker.name != "read" || invoker.arguments.Canonical() != `{"path":"main.go"}` {
		t.Fatalf("result=%#v cwd=%q root=%q name=%q arguments=%q", got, roots.root, invoker.root, invoker.name, invoker.arguments.Canonical())
	}
}
