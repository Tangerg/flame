package bootstrap

import (
	"context"
	"errors"
)

type teardownStep struct {
	action func(context.Context) error
}

func terminalStep(action func(context.Context) error) *teardownStep {
	return &teardownStep{action: action}
}

func (s *teardownStep) run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("teardown: context is required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.action(ctx)
}
