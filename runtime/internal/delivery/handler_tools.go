package delivery

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	domaintool "github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

func (s *Handler) ListTools(ctx context.Context) (*protocol.Page[protocol.ToolSpec], error) {
	internal, err := s.tools.List(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]protocol.ToolSpec, 0, len(internal))
	for _, t := range internal {
		schema, err := presentToolSchema(t.Definition)
		if err != nil {
			return nil, err
		}
		out = append(out, protocol.ToolSpec{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  schema,
			SafetyClass: presentSafetyClass(t.SafetyClass),
		})
	}
	return protocol.NewPage(out), nil
}

func (s *Handler) InvokeTool(ctx context.Context, in protocol.InvokeToolRequest) (any, error) {
	args, err := domaintool.ArgumentsFromMap(in.Arguments)
	if err != nil {
		return nil, protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err)
	}
	result, err := s.tools.Invoke(ctx, workspaceapp.DiagnosticToolInvocation{
		Name: in.Name, Arguments: args, CWD: workspaceRefPath(in.Workspace),
	})
	if err != nil {
		return nil, wireWorkspaceError(err)
	}
	return result.Any(), nil
}
