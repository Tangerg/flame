package workspace

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"

	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/samber/lo"
)

type diagnosticToolRegistry interface {
	List(ctx context.Context) ([]domaintool.Tool, error)
	Invoke(ctx context.Context, root, name string, arguments domaintool.Arguments) (domaintool.Result, error)
}

type DiagnosticToolInvocation struct {
	Name      string
	Arguments domaintool.Arguments
	CWD       string
}

type DiagnosticTools struct {
	registry diagnosticToolRegistry
	roots    *Scope
}

func NewDiagnosticTools(registry diagnosticToolRegistry, roots *Scope) (*DiagnosticTools, error) {
	if lo.IsNil(registry) || roots == nil {
		return nil, errors.New("workspace: diagnostic tool registry and roots are required")
	}
	return &DiagnosticTools{registry: registry, roots: roots}, nil
}

func (c *DiagnosticTools) List(ctx context.Context) ([]domaintool.Tool, error) {
	tools, err := c.registry.List(ctx)
	if err != nil {
		return nil, err
	}
	for _, candidate := range tools {
		if err := candidate.Validate(); err != nil {
			return nil, fmt.Errorf("workspace: diagnostic tool catalog contains an invalid definition: %w", err)
		}
		if candidate.SafetyClass != domaintool.SafetyClassSafe {
			return nil, fmt.Errorf("workspace: diagnostic tool %q is not safe for direct invocation", candidate.Name)
		}
	}
	slices.SortFunc(tools, func(first, second domaintool.Tool) int {
		return cmp.Compare(first.Name, second.Name)
	})
	for index := 1; index < len(tools); index++ {
		if tools[index].Name == tools[index-1].Name {
			return nil, fmt.Errorf("workspace: diagnostic tool catalog repeats name %q", tools[index].Name)
		}
	}
	return tools, nil
}

func (c *DiagnosticTools) Invoke(ctx context.Context, in DiagnosticToolInvocation) (domaintool.Result, error) {
	root, err := c.roots.ResolveRoot(in.CWD)
	if err != nil {
		return domaintool.Result{}, err
	}
	return c.registry.Invoke(ctx, root, in.Name, in.Arguments)
}
