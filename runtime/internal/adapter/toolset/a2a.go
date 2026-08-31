package toolset

import (
	"context"
	"fmt"
	"slices"

	scopea2a "github.com/Tangerg/scope/a2a"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// A2AAgentConfig identifies one remote Agent-to-Agent endpoint to expose as a
// delegation tool in the assembled tool environment.
type A2AAgentConfig struct {
	Name              string
	CardURL           string
	AllowedRPCOrigins []string
}

var a2aTracer = otel.Tracer("scope/flame/adapter/toolset/a2a")

// openA2AToolSet opens the remote delegation tools owned by this assembled
// tool environment. Scope's ToolSet is the lifecycle owner: it closes partial
// construction on failure and provides nil-safe, idempotent shutdown.
func openA2AToolSet(ctx context.Context, agents []A2AAgentConfig) (_ *scopea2a.ToolSet, err error) {
	ctx, span := a2aTracer.Start(ctx, "a2a.open_toolset",
		trace.WithAttributes(attribute.Int("a2a.agent.count", len(agents))))
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()

	endpoints := make([]scopea2a.Endpoint, len(agents))
	for i, agent := range agents {
		endpoints[i] = scopea2a.Endpoint{
			Name:              agent.Name,
			CardURL:           agent.CardURL,
			AllowedRPCOrigins: slices.Clone(agent.AllowedRPCOrigins),
		}
	}
	toolSet, err := scopea2a.OpenToolSet(ctx, endpoints...)
	if err != nil {
		return nil, fmt.Errorf("toolset: open A2A agents: %w", err)
	}
	span.SetAttributes(attribute.Int("a2a.tool.count", len(toolSet.Tools())))
	return toolSet, nil
}
