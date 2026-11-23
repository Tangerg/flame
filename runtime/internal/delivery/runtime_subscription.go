package delivery

import (
	"context"
	"iter"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const RuntimeSubscribe Name = "runtime.subscribe"

func registerRuntimeSubscription(registry *Registry) {
	registry.subscription(MethodMeta{
		Name: RuntimeSubscribe, GoName: "SubscribeRuntime",
		CapabilityRules: []CapabilityRule{{
			When:     []FieldCondition{{Field: "watches", Operator: OperatorPresent}},
			Requires: []string{protocol.FeatureFileWatch},
		}},
	}, func(service interface {
		SubscribeRuntime(context.Context, protocol.RuntimeSubscribeRequest) (*protocol.RuntimeSubscribeResponse, iter.Seq2[protocol.RuntimeEvent, error], error)
	}, ctx context.Context, request protocol.RuntimeSubscribeRequest,
	) (*protocol.RuntimeSubscribeResponse, iter.Seq2[protocol.RuntimeEvent, error], error) {
		return service.SubscribeRuntime(ctx, request)
	})
}
