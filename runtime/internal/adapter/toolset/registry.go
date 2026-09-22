package toolset

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"

	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

var toolTracer = otel.Tracer("scope/flame/tool")

const attrGenAIToolName = "gen_ai.tool.name"

// NewDiagnosticRegistry returns the explicitly direct-invocable diagnostic
// catalog. It deliberately does not reuse the agent resolver: agent tools may
// require a process, session, approval flow, or model loop that does not exist
// for a client-driven call.
func NewDiagnosticRegistry(directory string) (DiagnosticRegistry, error) {
	if !filepath.IsAbs(directory) {
		return DiagnosticRegistry{}, errors.New("toolset: diagnostic catalog directory must be absolute")
	}
	return DiagnosticRegistry{directory: filepath.Clean(directory)}, nil
}

// DiagnosticRegistry is the direct-invocation adapter for the small diagnostic
// tool catalog exposed outside an Agent Run.
type DiagnosticRegistry struct{ directory string }

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
		schema, err := tool.ParseSchema(definition.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("toolset: decode input schema for tool %q: %w", definition.Name, err)
		}
		out = append(out, tool.Tool{
			Name:        definition.Name,
			Description: definition.Description,
			Schema:      schema,
			SafetyClass: interpreter.SafetyClass(definition.Name),
		})
	}
	return out, nil
}

func (DiagnosticRegistry) Invoke(ctx context.Context, root, name string, arguments tool.Arguments) (_ tool.Result, err error) {
	if name == "" {
		return tool.Result{}, errors.New("toolset: direct tool name must not be empty")
	}
	ctx, span := toolTracer.Start(ctx, "execute_direct_tool "+name,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(attribute.String(attrGenAIToolName, name)))
	defer span.End()

	direct, err := openDirectTools(root)
	if err != nil {
		return tool.Result{}, err
	}
	defer func() { err = errors.Join(err, direct.Close()) }()
	registry, err := toolcontract.NewRegistry(direct.Visible...)
	if err != nil {
		return tool.Result{}, fmt.Errorf("toolset: bind diagnostic catalog: %w", err)
	}
	binding, found := registry.Resolve(name)
	if !found {
		err = fmt.Errorf("toolset: direct tool %q is not registered", name)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return tool.Result{}, err
	}
	proposed, prepareErr := binding.Contract().Prepare(chat.ToolCall{ID: "direct", Name: name, Arguments: arguments.Canonical()})
	if prepareErr != nil {
		return tool.Result{}, fmt.Errorf("%w: direct tool %q: %w", tool.ErrInvalidArguments, name, prepareErr)
	}
	normalized, err := normalizeDirectArguments(root, name, proposed)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return tool.Result{}, err
	}
	invocation, prepareErr := binding.Contract().Prepare(chat.ToolCall{ID: "direct", Name: name, Arguments: normalized})
	if prepareErr != nil {
		return tool.Result{}, fmt.Errorf("toolset: prepare direct tool %q: %w", name, prepareErr)
	}
	output, callErr := binding.Call(ctx, invocation)
	if callErr != nil {
		span.RecordError(callErr)
		span.SetStatus(codes.Error, callErr.Error())
		return tool.Result{}, callErr
	}
	return directResult(output)
}
