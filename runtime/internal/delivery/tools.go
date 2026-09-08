package delivery

import (
	"context"

	"github.com/Tangerg/flame/runtime/protocol"
)

const (
	ToolsList   Name = "tools.list"
	ToolsInvoke Name = "tools.invoke"
)

func registerTools(registry *Registry) {
	registry.Query(MethodMeta{Name: ToolsList},
		func(service *Handler, ctx context.Context, _ struct{}) (*protocol.Page[protocol.ToolSpec], error) {
			return service.ListTools(ctx)
		})

	registry.Command(MethodMeta{
		Name: ToolsInvoke,
		Errors: []string{
			protocol.ErrWorkspaceUnavailable.Error(),
			protocol.ErrPathOutsideRoot.Error(),
		},
	}, (*Handler).InvokeTool)
}
