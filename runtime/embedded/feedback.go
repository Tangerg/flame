package embedded

import (
	"context"

	"github.com/Tangerg/flame/runtime/internal/delivery"
	"github.com/Tangerg/flame/runtime/protocol"
)

// CreateFeedback records one quality signal.
func (r *Runtime) CreateFeedback(ctx context.Context, request protocol.FeedbackRequest, options CommandOptions) error {
	return r.invokeAck(ctx, delivery.FeedbackCreate, request, commandOptions(options))
}
