package workspace

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

type toolRegistryFixture struct {
	tools []tool.Tool
}

func (t toolRegistryFixture) List(context.Context) ([]tool.Tool, error) {
	return slices.Clone(t.tools), nil
}
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
	err  error
}

func (r *rootRecorder) ResolveRoot(cwd string) (string, error) {
	r.root = cwd
	return "/workspace", r.err
}

func TestNewDiagnosticToolsRequiresCompleteDependencies(t *testing.T) {
	for _, test := range []struct {
		name     string
		registry DiagnosticToolRegistry
		roots    DiagnosticToolRoots
	}{
		{name: "registry", roots: &rootRecorder{}},
		{name: "typed nil registry", registry: (*toolRegistryFixture)(nil), roots: &rootRecorder{}},
		{name: "roots", registry: toolRegistryFixture{}},
		{name: "typed nil roots", registry: toolRegistryFixture{}, roots: (*rootRecorder)(nil)},
	} {
		t.Run(test.name, func(t *testing.T) {
			if useCases, err := NewDiagnosticTools(test.registry, test.roots); err == nil || useCases != nil {
				t.Fatalf("NewDiagnosticTools = (%v, %v), want incomplete construction rejected", useCases, err)
			}
		})
	}
}

func newDiagnosticTools(t *testing.T, registry DiagnosticToolRegistry, roots DiagnosticToolRoots) *DiagnosticTools {
	t.Helper()
	useCases, err := NewDiagnosticTools(registry, roots)
	if err != nil {
		t.Fatal(err)
	}
	return useCases
}

func TestListOwnsSafeUniqueNameOrder(t *testing.T) {
	registry := toolRegistryFixture{tools: []tool.Tool{
		{Name: "read", SafetyClass: tool.SafetyClassSafe},
		{Name: "glob", SafetyClass: tool.SafetyClassSafe},
	}}
	c := newDiagnosticTools(t, registry, &rootRecorder{})

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
	next, err := c.List(t.Context())
	if err != nil || len(next) != 2 || next[0].Name != "glob" {
		t.Fatalf("List after caller reused result = (%+v, %v)", next, err)
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
			c := newDiagnosticTools(t, toolRegistryFixture{tools: tools}, &rootRecorder{})
			if _, err := c.List(context.Background()); err == nil {
				t.Fatal("invalid diagnostic tool catalog was accepted")
			}
		})
	}
}

func TestInvokeUsesRegistry(t *testing.T) {
	invoker := &toolRegistryRecorder{}
	roots := &rootRecorder{}
	c := newDiagnosticTools(t, invoker, roots)

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

func TestInvokePreservesRootRejectionBeforeToolExecution(t *testing.T) {
	cause := errors.New("workspace root is unavailable")
	invoker := &toolRegistryRecorder{}
	c := newDiagnosticTools(t, invoker, &rootRecorder{err: cause})
	if _, err := c.Invoke(t.Context(), DiagnosticToolInvocation{Name: "read", CWD: "/requested"}); !errors.Is(err, cause) {
		t.Fatalf("Invoke error = %v, want root rejection", err)
	}
	if invoker.name != "" {
		t.Fatal("rejected root reached tool invocation")
	}
}
