package teardown

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"slices"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/completion"
)

// Sequence owns terminal resource steps in creation order. Shutdown starts one
// reverse-order generation that outlives any individual caller deadline; this
// lets a failed constructor return promptly without abandoning the dependency
// graph once an in-flight closer eventually finishes.
type Sequence struct {
	mu      sync.Mutex
	steps   []*Step
	attempt *sequenceAttempt
}

type sequenceAttempt struct {
	done chan struct{}
	err  error
}

func NewSequence(steps []*Step) *Sequence {
	return &Sequence{steps: slices.Clone(steps)}
}

// Shutdown starts or joins the one terminal teardown generation. settled=false
// means only that this caller stopped waiting; the Sequence still owns and runs
// the graph. settled=true returns the immutable aggregate diagnostic.
func (s *Sequence) Shutdown(ctx context.Context) (settled bool, err error) {
	if s == nil {
		return true, nil
	}
	if ctx == nil {
		return false, errors.New("teardown: sequence context is required")
	}
	s.mu.Lock()
	attempt := s.attempt
	if attempt == nil {
		if err := ctx.Err(); err != nil {
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

// runStep keeps one step's defect from abandoning the steps still owed. The
// sequence exists to run every step and join what each reported; a panic is what
// that step reported.
func runStep(ctx context.Context, step *Step) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("teardown: step panicked: %v\n%s", recovered, debug.Stack())
		}
	}()
	return step.run(ctx)
}

func (s *Sequence) run(ctx context.Context, attempt *sequenceAttempt) {
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
