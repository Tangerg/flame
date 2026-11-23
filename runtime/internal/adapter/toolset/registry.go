package toolset

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	builtincontract "github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"

	toolcontract "github.com/Tangerg/scope/core/tool"
	oteltool "github.com/Tangerg/scope/otel/tool"
)

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

type DiagnosticRegistry struct {
	directory string
	telemetry oteltool.Middleware
}

func (r DiagnosticRegistry) List(context.Context) (_ []tool.Tool, err error) {
	manifest, err := openDirectTools(r.directory)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, manifest.Close()) }()
	registry, err := diagnosticRegistry(manifest.Visible, r.telemetry)
	if err != nil {
		return nil, fmt.Errorf("toolset: bind diagnostic catalog: %w", err)
	}
	interpreter := Interpreter{}
	out := make([]tool.Tool, 0, len(manifest.Visible))
	for _, definition := range registry.Definitions() {
		ref, err := tool.BuiltIn(builtincontract.Name(definition.Name))
		if err != nil {
			return nil, err
		}
		out = append(out, tool.Tool{
			Definition:  definition,
			SafetyClass: interpreter.SafetyClass(ref),
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
	registry, err := diagnosticRegistry(direct.Visible, r.telemetry)
	if err != nil {
		return tool.Result{}, fmt.Errorf("toolset: bind diagnostic catalog: %w", err)
	}
	binding, found := registry.Resolve(name)
	if !found {
		return tool.Result{}, fmt.Errorf("toolset: direct tool %q is not registered", name)
	}
	proposed, prepareErr := binding.Contract().Prepare(toolcontract.Request{Name: name, Arguments: arguments.Canonical()}, toolcontract.Admission{})
	if prepareErr != nil {
		return tool.Result{}, fmt.Errorf("%w: direct tool %q: %w", tool.ErrInvalidArguments, name, prepareErr)
	}
	normalized, err := normalizeDirectArguments(root, name, proposed)
	if err != nil {
		return tool.Result{}, err
	}
	invocation, prepareErr := binding.Contract().Prepare(toolcontract.Request{Name: name, Arguments: normalized}, toolcontract.Admission{})
	if prepareErr != nil {
		return tool.Result{}, fmt.Errorf("toolset: prepare direct tool %q: %w", name, prepareErr)
	}
	output, callErr := binding.Call(ctx, invocation)
	if callErr != nil {
		return tool.Result{}, callErr
	}
	return directResult(output.Output)
}

func diagnosticRegistry(tools []toolcontract.Tool, telemetry oteltool.Middleware) (toolcontract.Registry, error) {
	bindings := make([]toolcontract.Binding, len(tools))
	for index, executable := range tools {
		binding, err := toolcontract.Bind(toolcontract.Wrap(executable, telemetry.Call))
		if err != nil {
			return toolcontract.Registry{}, err
		}
		bindings[index] = binding
	}
	return toolcontract.NewRegistry(bindings...)
}
