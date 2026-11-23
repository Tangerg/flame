package delivery

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const RuntimeDiscover Name = "runtime.discover"

func registerLifecycle(registry *Registry) {
	registry.query(MethodMeta{Name: RuntimeDiscover, GoName: "Discover"},
		func(service interface {
			Discover(context.Context) (*protocol.DiscoverResponse, error)
		}, ctx context.Context, _ struct{}) (*protocol.DiscoverResponse, error) {
			return service.Discover(ctx)
		})
}
