package plugin

import (
	"context"
	"errors"

	framecall "github.com/Tangerg/frame/call"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// Only a call that provably performed no external mutation may settle as a
// definite failure. The Host reads an unclassified Tool error as an unprovable
// outcome and settles the whole Run tree as lost. Cancellation is returned
// unchanged; it is never model feedback.
func DefiniteToolFailure(cause error) error {
	if cause == nil || errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		return cause
	}
	if _, known := toolcontract.ClassifyFailure(cause); known {
		return cause
	}
	return toolcontract.FailureFromCause(toolcontract.FailureKindFailed, cause)
}

func RefusedToolFailure(err error) error {
	refusal, known := errors.AsType[*framecall.Refusal](err)
	if !known {
		return err
	}
	failure, failureErr := toolcontract.NewFailure(toolcontract.FailureConfig{Kind: toolcontract.FailureKindRejected, Output: toolcontract.NewTextOutput(refusal.Message), Cause: err})
	return errors.Join(failure, failureErr)
}
