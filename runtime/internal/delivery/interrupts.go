package delivery

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const InterruptsList Name = "interrupts.list"

func registerInterrupts(registry *Registry) {
	registry.query(MethodMeta{
		Name: InterruptsList, GoName: "ListInterrupts",
		Errors: []string{protocol.ErrRunNotRoot.
			Error(), protocol.ErrCapabilityNotNeg.
			Error(),
		},
	}, func(service interface {
		ListInterrupts(context.Context, protocol.ListInterruptsRequest) (*protocol.Page[protocol.PendingInterruptSet], error)
	}, ctx context.Context, request protocol.ListInterruptsRequest) (*protocol.Page[protocol.PendingInterruptSet], error) {
		return service.ListInterrupts(ctx, request)
	})
}
