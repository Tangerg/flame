package plugin

import (
	"context"
	"errors"

	framecall "github.com/Tangerg/frame/call"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// Only a call that provably performed no external mutation may settle as a
// definite failure: a rejected argument, a local search, a read.
//
// The alternative is what makes this matter. The Host reads an unclassified Tool
// error as an operation whose durable outcome it cannot prove, and settles the
// whole Run tree — the root and every delegated child — as lost. A mistyped path
// or a miscounted patch hunk would end the Session over work that never started.
//
// Cancellation belongs to the execution owner and is returned unchanged; it is
// never model feedback.
func DefiniteToolFailure(cause error) error {
	if cause == nil || errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) {
		return cause
	}
	if _, known := toolcontract.ClassifyFailure(cause); known {
		return cause
	}
	failure, err := toolcontract.NewFailure(toolcontract.FailureConfig{
		Kind:   toolcontract.FailureKindFailed,
		Cause:  cause,
		Output: toolcontract.NewTextOutput(cause.Error()),
	})
	if err != nil {
		return err
	}
	return failure
}

// RefusedToolFailure reports a business refusal to the model as a rejected
// call carrying the refusal message; every other error is returned unchanged.
func RefusedToolFailure(err error) error {
	refusal, known := errors.AsType[*framecall.Refusal](err)
	if !known {
		return err
	}
	failure, failureErr := toolcontract.NewFailure(toolcontract.FailureConfig{Kind: toolcontract.FailureKindRejected, Output: toolcontract.NewTextOutput(refusal.Message), Cause: err})
	return errors.Join(failure, failureErr)
}
