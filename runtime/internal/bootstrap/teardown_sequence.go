package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"slices"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/completion"
)

type shutdownSequence struct {
	mu      sync.Mutex
	steps   []*teardownStep
	attempt *sequenceAttempt
}

type sequenceAttempt struct {
	done chan struct{}
	err  error
}

func newShutdownSequence(steps []*teardownStep) *shutdownSequence {
	return &shutdownSequence{steps: slices.Clone(steps)}
}

func (s *shutdownSequence) shutdown(ctx context.Context) (settled bool, err error) {
	if s == nil {
		return true, nil
	}
	if ctx == nil {
		return false, errors.New("teardown: sequence context is required")
	}
	s.mu.Lock()
	attempt := s.attempt
	if attempt == nil {
		if err := errors.Join(ctx.Err(), context.Cause(ctx)); err != nil {
			s.mu.Unlock()
			return false, err
		}
		attempt = &sequenceAttempt{done: make(chan struct{})}
		s.attempt = attempt
		go s.run(context.WithoutCancel(ctx), attempt)
	}
	s.mu.Unlock()

	if err := completion.Wait(ctx, attempt.done); err != nil {
		return false, err
	}
	return true, attempt.err
}

func runStep(ctx context.Context, step *teardownStep) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("teardown: step panicked: %v\n%s", recovered, debug.Stack())
		}
	}()
	return step.run(ctx)
}

func (s *shutdownSequence) run(ctx context.Context, attempt *sequenceAttempt) {
	var diagnostics []error
	for _, step := range slices.Backward(s.steps) {
		if step == nil {
			continue
		}
		diagnostics = append(diagnostics, runStep(ctx, step))
	}

	s.mu.Lock()
	s.steps = nil
	attempt.err = errors.Join(diagnostics...)
	close(attempt.done)
	s.mu.Unlock()
}
