package toolset

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
	oteltool "github.com/Tangerg/scope/otel/tool"

	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

// NewDiagnosticRegistry returns the explicitly direct-invocable diagnostic
// catalog. It deliberately does not reuse the agent resolver: agent tools may
// require a process, session, approval flow, or model loop that does not exist
// for a client-driven call.
func NewDiagnosticRegistry(directory string) (DiagnosticRegistry, error) {
	if !filepath.IsAbs(directory) {
		return DiagnosticRegistry{}, errors.New("toolset: diagnostic catalog directory must be absolute")
	}
	telemetry, err := oteltool.NewMiddleware(oteltool.MiddlewareConfig{})
	if err != nil {
		return DiagnosticRegistry{}, fmt.Errorf("toolset: instrument diagnostic tool calls: %w", err)
	}
	return DiagnosticRegistry{directory: filepath.Clean(directory), telemetry: telemetry}, nil
}

// DiagnosticRegistry is the direct-invocation adapter for the small diagnostic
// tool catalog exposed outside an Agent Run. A client-driven call is still a
// Tool call, so it is instrumented by the same Scope middleware the Agent
// resolver uses rather than by a second span of this adapter's own.
type DiagnosticRegistry struct {
	directory string
	telemetry oteltool.Middleware
}

// List projects Scope's admitted, frozen definitions into the product catalog.
func (r DiagnosticRegistry) List(context.Context) (_ []tool.Tool, err error) {
	manifest, err := openDirectTools(r.directory)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, manifest.Close()) }()
	registry, err := toolcontract.NewRegistry(manifest.Visible...)
	if err != nil {
		return nil, fmt.Errorf("toolset: bind diagnostic catalog: %w", err)
	}
	interpreter := Interpreter{}
	out := make([]tool.Tool, 0, len(manifest.Visible))
	for _, definition := range registry.Definitions() {
		out = append(out, tool.Tool{
			ToolDefinition: definition,
			SafetyClass:    interpreter.SafetyClass(definition.Name),
		})
	}
	return out, nil
}

func (r DiagnosticRegistry) Invoke(ctx context.Context, root, name string, arguments tool.Arguments) (_ tool.Result, err error) {
	if name == "" {
		return tool.Result{}, errors.New("toolset: direct tool name must not be empty")
	}
	direct, err := openDirectTools(root)
	if err != nil {
		return tool.Result{}, err
	}
	defer func() { err = errors.Join(err, direct.Close()) }()
	instrumented, err := instrumentTools(r.telemetry, direct.Visible)
	if err != nil {
		return tool.Result{}, err
	}
	registry, err := toolcontract.NewRegistry(instrumented...)
	if err != nil {
		return tool.Result{}, fmt.Errorf("toolset: bind diagnostic catalog: %w", err)
	}
	binding, found := registry.Resolve(name)
	if !found {
		return tool.Result{}, fmt.Errorf("toolset: direct tool %q is not registered", name)
	}
	proposed, prepareErr := binding.Contract().Prepare(chat.ToolCall{ID: "direct", Name: name, Arguments: arguments.Canonical()})
	if prepareErr != nil {
		return tool.Result{}, fmt.Errorf("%w: direct tool %q: %w", tool.ErrInvalidArguments, name, prepareErr)
	}
	normalized, err := normalizeDirectArguments(root, name, proposed)
	if err != nil {
		return tool.Result{}, err
	}
	invocation, prepareErr := binding.Contract().Prepare(chat.ToolCall{ID: "direct", Name: name, Arguments: normalized})
	if prepareErr != nil {
		return tool.Result{}, fmt.Errorf("toolset: prepare direct tool %q: %w", name, prepareErr)
	}
	output, callErr := binding.Call(ctx, invocation)
	if callErr != nil {
		return tool.Result{}, callErr
	}
	return directResult(output)
}
