package toolarg

import (
	"context"
	"errors"

	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// Unusable turns the refusal of a Tool call's own arguments into that call's
// definite outcome, so the model that wrote them is told and can write them
// again. It is the argument-boundary twin of the rule local searches already
// follow: a refusal that performed no mutation is a definite failed call, not an
// unacknowledged external operation.
//
// Returning the cause unclassified is what makes this matter. The Host reads an
// unclassified Tool error as an operation whose durable outcome it cannot prove
// and settles the whole Run tree as lost — so one miscounted patch hunk or one
// contradictory flag ends the Run and every delegated child with it. Argument
// validation runs before the Tool does anything, so there is nothing unknown to
// settle.
//
// Cancellation is the execution owner's and is returned unchanged.
func Unusable(cause error) error {
	if cause == nil || errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		return cause
	}
	failure, err := toolcontract.NewFailure(toolcontract.FailureConfig{
		Kind:   toolcontract.FailureKindFailed,
		Cause:  cause,
		Output: chat.NewTextToolOutput(cause.Error()),
	})
	if err != nil {
		return err
	}
	return failure
}
