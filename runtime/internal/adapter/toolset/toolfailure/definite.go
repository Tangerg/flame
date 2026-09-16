// Package toolfailure classifies the unsuccessful outcome of one Runtime Tool
// call for the toolset adapters.
package toolfailure

import (
	"context"
	"errors"

	"github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// Definite settles cause as this call's definite outcome, so the model is told
// and can act on it. Only a call that provably performed no external mutation
// may use it: a rejected argument, a local search, a read.
//
// The alternative is what makes this matter. The Host reads an unclassified Tool
// error as an operation whose durable outcome it cannot prove, and settles the
// whole Run tree — the root and every delegated child — as lost. A mistyped path
// or a miscounted patch hunk would end the Session over work that never started.
//
// Cancellation belongs to the execution owner and is returned unchanged; it is
// never model feedback.
func Definite(cause error) error {
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
