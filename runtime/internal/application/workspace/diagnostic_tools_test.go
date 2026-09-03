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
func (toolRegistryFixture) Invoke(context.Context, string, string, string) (tool.Result, error) {
	return tool.Result{}, nil
}

type toolRegistryRecorder struct {
	root      string
	name      string
	arguments string
}

func (t *toolRegistryRecorder) Invoke(_ context.Context, root, name string, arguments string) (tool.Result, error) {
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
		"empty name":  {{SafetyClass: tool.SafetyClassSafe}},
		"padded name": {{Name: " read ", SafetyClass: tool.SafetyClassSafe}},
		"unsafe":      {{Name: "write", SafetyClass: tool.SafetyClassWrite}},
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

	got, err := c.Invoke(context.Background(), DiagnosticToolInvocation{Name: "read", Arguments: `{"path":"main.go"}`, CWD: "/requested"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if text, ok := got.String(); !ok || text != "ok" || roots.root != "/requested" || invoker.root != "/workspace" || invoker.name != "read" || invoker.arguments != `{"path":"main.go"}` {
		t.Fatalf("result=%#v cwd=%q root=%q name=%q arguments=%q", got, roots.root, invoker.root, invoker.name, invoker.arguments)
	}
}
