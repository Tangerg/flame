package delivery

import (
	"context"

	"github.com/Tangerg/flame/runtime/protocol"
)

const RuntimeDiscover Name = "runtime.discover"

func registerLifecycle(registry *Registry) {
	// runtime.discover takes no params; struct{} makes an unexpected field a
	// decode failure rather than something silently ignored.
	registry.Query(MethodMeta{Name: RuntimeDiscover},
		func(service *Handler, ctx context.Context, _ struct{}) (*protocol.DiscoverResponse, error) {
			return service.Discover(ctx)
		})
}
