package workspace

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	toolsvc "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

// DiagnosticToolRegistry is the directly invocable diagnostic-tool catalog. It is deliberately
// distinct from the agent's full tool set: every entry must be safe to run
// outside a Run and must honor the supplied workspace root. List transfers
// ownership of the returned tool definitions to the caller.
type DiagnosticToolRegistry interface {
	List(ctx context.Context) ([]toolsvc.Tool, error)
	Invoke(ctx context.Context, root, name string, arguments toolsvc.Arguments) (toolsvc.Result, error)
}

// DiagnosticToolRoots resolves the workspace root an external diagnostic invocation is
// allowed to inspect. The workspace use case owns cwd admission; tools never
// accept an unchecked client path as their filesystem root.
type DiagnosticToolRoots interface {
	ResolveRoot(cwd string) (string, error)
}

// DiagnosticToolInvocation is one direct, read-only diagnostic tool call.
type DiagnosticToolInvocation struct {
	Name      string
	Arguments toolsvc.Arguments
	CWD       string
}

// DiagnosticTools drives direct diagnostic-tool use cases.
type DiagnosticTools struct {
	registry DiagnosticToolRegistry
	roots    DiagnosticToolRoots
}

// NewDiagnosticTools returns diagnostic tool use cases over the direct registry and workspace-root
// admission boundary.
func NewDiagnosticTools(registry DiagnosticToolRegistry, roots DiagnosticToolRoots) (*DiagnosticTools, error) {
	for _, dependency := range []struct {
		name  string
		value any
	}{
		{name: "registry", value: registry},
		{name: "roots", value: roots},
	} {
		if missingDependency(dependency.value) {
			return nil, fmt.Errorf("workspace: diagnostic tool %s is required", dependency.name)
		}
	}
	return &DiagnosticTools{registry: registry, roots: roots}, nil
}

// List returns the safe direct-invocation catalog, ordered by unique tool name.
func (c *DiagnosticTools) List(ctx context.Context) ([]toolsvc.Tool, error) {
	tools, err := c.registry.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, candidate := range tools {
		if err := candidate.Validate(); err != nil {
			return nil, fmt.Errorf("workspace: diagnostic tool catalog contains an invalid definition: %w", err)
		}
		if candidate.SafetyClass != toolsvc.SafetyClassSafe {
			return nil, fmt.Errorf("workspace: diagnostic tool %q is not safe for direct invocation", candidate.Name)
		}
	}
	slices.SortFunc(tools, func(first, second toolsvc.Tool) int {
		return cmp.Compare(first.Name, second.Name)
	})
	for index := 1; index < len(tools); index++ {
		if tools[index].Name == tools[index-1].Name {
			return nil, fmt.Errorf("workspace: diagnostic tool catalog repeats name %q", tools[index].Name)
		}
	}
	return tools, nil
}

// Invoke runs one direct diagnostic tool within its admitted workspace root.
func (c *DiagnosticTools) Invoke(ctx context.Context, in DiagnosticToolInvocation) (toolsvc.Result, error) {
	root, err := c.roots.ResolveRoot(in.CWD)
	if err != nil {
		return toolsvc.Result{}, err
	}
	return c.registry.Invoke(ctx, root, in.Name, in.Arguments)
}
