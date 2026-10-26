package bootstrap

import (
	"context"
	"errors"
)

// teardownStep is one terminal resource action. shutdownSequence owns execution, ordering and
// joining; the step deliberately carries no second lifecycle state.
type teardownStep struct {
	action func(context.Context) error
}

// terminalStep returns a teardown step whose action reaching a return statement is
// the resource's final state. Its error is a shutdown diagnostic, not evidence
// that replaying the same one-shot Close can make further progress.
func terminalStep(action func(context.Context) error) *teardownStep {
	return &teardownStep{action: action}
}

func (s *teardownStep) run(ctx context.Context) error {
	if s == nil || s.action == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("teardown: context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.action(ctx)
}
