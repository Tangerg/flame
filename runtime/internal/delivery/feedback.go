package delivery

import (
	"context"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

const FeedbackCreate Name = "feedback.create"

func registerFeedback(registry *Registry) {
	registry.commandAck(MethodMeta{Name: FeedbackCreate, GoName: "CreateFeedback"},
		func(service interface {
			CreateFeedback(context.Context, protocol.FeedbackRequest) error
		}, ctx context.Context, request protocol.FeedbackRequest) error {
			return service.CreateFeedback(ctx, request)
		})
}
