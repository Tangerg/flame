package runs

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/completion"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/samber/lo"
)

type ExecutionFactCommit struct {
	executorPayloadBase
	fact  ExecutionFact
	state *executionFactCommitState
}

type ExecutionFactReceipt struct {
	state *executionFactCommitState
}

type executionFactCommitState struct {
	once sync.Once
	done chan struct{}
	err  error
}

func NewExecutionFactCommit(fact ExecutionFact) (ExecutionFactCommit, ExecutionFactReceipt, error) {
	if lo.IsNil(fact) {
		return ExecutionFactCommit{}, ExecutionFactReceipt{}, errors.New("runs: execution fact commit requires a fact")
	}
	owned, supported := cloneExecutionFact(fact)
	if !supported {
		return ExecutionFactCommit{}, ExecutionFactReceipt{}, fmt.Errorf(
			"runs: execution fact commit does not support %T",
			fact,
		)
	}
	state := &executionFactCommitState{done: make(chan struct{})}
	return ExecutionFactCommit{fact: owned, state: state}, ExecutionFactReceipt{state: state}, nil
}

func (e ExecutionFactCommit) validate() error {
	if e.fact == nil || e.state == nil || e.state.done == nil {
		return errors.New("runs: malformed execution fact commit")
	}
	return nil
}

func (e ExecutionFactCommit) Fact() ExecutionFact {
	fact, _ := cloneExecutionFact(e.fact)
	return fact
}

func cloneExecutionFact(fact ExecutionFact) (ExecutionFact, bool) {
	switch value := fact.(type) {
	case MessageDelta, ReasoningDelta, ModelCallStarted, ToolCallStarted, PlanUpdated:
		return fact, true
	case ModelCallCompleted:
		return value.clone(), true
	case ModelCallFailed:
		return value.clone(), true
	case ToolCallPrepared:
		value.Context = value.Context.Clone()
		return value, true
	case ExecutionTreeSettled:
		return value.clone()
	case ToolResultsCommitted:
		return value.clone(), true
	case CompactionBoundary:
		if value.Rewrite != nil {
			value.Rewrite = new(*value.Rewrite)
		}
		return value, true
	case SegmentEnded:
		if run.ValidateEnding(value.Ending) != nil {
			return nil, false
		}
		return value.WithUnresolvedEffects(value.unresolvedEffects), true
	case SteerMessagesApplied:
		return value.clone(), true
	default:
		return nil, false
	}
}

func (c ModelCallCompleted) clone() ModelCallCompleted {
	if c.FirstOutputLatencyMillis != nil {
		c.FirstOutputLatencyMillis = new(*c.FirstOutputLatencyMillis)
	}
	if c.ReportedUsage != nil {
		c.ReportedUsage = new(c.ReportedUsage.Clone())
	}
	if c.Message != nil {
		c.Message = new(c.Message.Clone())
	}
	return c
}

func (f ModelCallFailed) clone() ModelCallFailed {
	f.KnownResponse = f.KnownResponse.Clone()
	if f.Accounting != nil {
		f.Accounting = new(f.Accounting.clone())
	}
	if f.FirstOutputLatencyMillis != nil {
		f.FirstOutputLatencyMillis = new(*f.FirstOutputLatencyMillis)
	}
	return f
}

func (s SteerMessagesApplied) clone() SteerMessagesApplied {
	s.Messages = slices.Clone(s.Messages)
	for index := range s.Messages {
		s.Messages[index].Content = transcript.CloneContent(s.Messages[index].Content)
	}
	return s
}

func (e ExecutionFactCommit) Complete(err error) {
	if e.state == nil {
		return
	}
	e.state.once.Do(func() {
		e.state.err = err
		close(e.state.done)
	})
}

func (e ExecutionFactReceipt) Await(ctx context.Context) error {
	if e.state == nil || e.state.done == nil {
		return errors.New("runs: malformed execution fact receipt")
	}
	if err := completion.Wait(ctx, e.state.done); err != nil {
		return err
	}
	return e.state.err
}
