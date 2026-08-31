package workspace

import (
	"context"

	toolsvc "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

// DiagnosticToolRegistry is the directly invocable diagnostic-tool catalog. It is deliberately
// distinct from the agent's full tool set: every entry must be safe to run
// outside a Run and must honor the supplied workspace root.
type DiagnosticToolRegistry interface {
	List(ctx context.Context) ([]toolsvc.Tool, error)
	Invoke(ctx context.Context, root, name, arguments string) (toolsvc.Result, error)
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
	Arguments string
	CWD       string
}

// DiagnosticTools drives direct diagnostic-tool use cases.
type DiagnosticTools struct {
	registry DiagnosticToolRegistry
	roots    DiagnosticToolRoots
}

// NewDiagnosticTools returns diagnostic tool use cases over the direct registry and workspace-root
// admission boundary.
func NewDiagnosticTools(registry DiagnosticToolRegistry, roots DiagnosticToolRoots) *DiagnosticTools {
	return &DiagnosticTools{registry: registry, roots: roots}
}

// List returns every tool that can be invoked directly outside a Run.
func (c *DiagnosticTools) List(ctx context.Context) ([]toolsvc.Tool, error) {
	return c.registry.List(ctx)
}

// Invoke runs one direct diagnostic tool within its admitted workspace root.
func (c *DiagnosticTools) Invoke(ctx context.Context, in DiagnosticToolInvocation) (toolsvc.Result, error) {
	root, err := c.roots.ResolveRoot(in.CWD)
	if err != nil {
		return toolsvc.Result{}, err
	}
	return c.registry.Invoke(ctx, root, in.Name, in.Arguments)
}
