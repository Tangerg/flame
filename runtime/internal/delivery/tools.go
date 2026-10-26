package delivery

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const (
	ToolsList   Name = "tools.list"
	ToolsInvoke Name = "tools.invoke"
)

func registerTools(registry *Registry) {
	registry.query(MethodMeta{Name: ToolsList, GoName: "ListTools"},
		func(service interface {
			ListTools(context.Context) (*protocol.Page[protocol.ToolSpec], error)
		}, ctx context.Context, _ struct{}) (*protocol.Page[protocol.ToolSpec], error) {
			return service.ListTools(ctx)
		})

	registry.command(MethodMeta{
		Name: ToolsInvoke, GoName: "InvokeTool",
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
	}, func(service interface {
		InvokeTool(context.Context, protocol.InvokeToolRequest) (any, error)
	}, ctx context.Context, request protocol.InvokeToolRequest) (any, error) {
		return service.InvokeTool(ctx, request)
	})
}
