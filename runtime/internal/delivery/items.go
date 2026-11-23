package delivery

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const ItemsList Name = "items.list"

func registerItems(registry *Registry) {
	registry.query(MethodMeta{
		Name: ItemsList, GoName: "ListItems",
		Errors: []string{protocol.ErrSessionNotFound.
			Error(), protocol.ErrRunNotFound.
			Error(),
		},
		CapabilityRules: descendantsRequireChildRuns("scope." + includeDescendantsParameter),
	}, func(service interface {
		ListItems(context.Context, protocol.ListItemsRequest) (*protocol.ListItemsResponse, error)
	}, ctx context.Context, request protocol.ListItemsRequest) (*protocol.ListItemsResponse, error) {
		return service.ListItems(ctx, request)
	})
}
