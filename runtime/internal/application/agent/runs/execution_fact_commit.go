package runs

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// ExecutionFactCommit asks the Run pump to durably project one authoritative
// executor fact before the external invocation that produced it may settle.
// It travels on the same ordered executor stream as ordinary facts, preserving
// the pump as the sole reducer and persistence writer.
type ExecutionFactCommit struct {
	executorPayloadBase
	fact  ExecutionFact
	state *executionFactCommitState
}

// ExecutionFactReceipt is the producer side of an [ExecutionFactCommit]. Await
// returns only after the Run pump has either committed every derived write or
// rejected the fact. Abandoning observation completes outstanding receipts with
// context cancellation through the producer's own wait context.
type ExecutionFactReceipt struct {
	state *executionFactCommitState
}

type executionFactCommitState struct {
	once sync.Once
	done chan error
}

// NewExecutionFactCommit creates one authoritative fact request and its
// one-consumer receipt. The fact remains an Application-owned closed value;
// executor implementations receive no persistence handle or transaction capability.
func NewExecutionFactCommit(fact ExecutionFact) (ExecutionFactCommit, ExecutionFactReceipt, error) {
	if fact == nil {
		return ExecutionFactCommit{}, ExecutionFactReceipt{}, errors.New("runs: execution fact commit requires a fact")
	}
	owned, supported := cloneExecutionFact(fact)
	if !supported {
		return ExecutionFactCommit{}, ExecutionFactReceipt{}, fmt.Errorf(
			"runs: execution fact commit does not support %T",
			fact,
		)
	}
	state := &executionFactCommitState{done: make(chan error, 1)}
	return ExecutionFactCommit{fact: owned, state: state}, ExecutionFactReceipt{state: state}, nil
}

func (e ExecutionFactCommit) validate() error {
	if e.fact == nil || e.state == nil || e.state.done == nil {
		return errors.New("runs: malformed execution fact commit")
	}
	return nil
}

// Fact returns an ownership-isolated copy of the authoritative executor fact.
func (e ExecutionFactCommit) Fact() ExecutionFact {
	fact, _ := cloneExecutionFact(e.fact)
	return fact
}

func cloneExecutionFact(fact ExecutionFact) (ExecutionFact, bool) {
	switch value := fact.(type) {
	case MessageDelta:
		return value, true
	case ReasoningDelta:
		return value, true
	case AssistantMessageCompleted:
		value.message = value.message.Clone()
		return value, true
	case ModelCallStarted:
		return value, true
	case ModelCallCompleted:
		value.Message = value.Message.Clone()
		value.ByModel = slices.Clone(value.ByModel)
		return value, true
	case ModelCallFailed:
		return value, true
	case ToolCallStarted:
		return value, true
	case ToolCallFinished:
		if value.ModelResult != nil {
			modelResult := value.ModelResult.Clone()
			value.ModelResult = &modelResult
		}
		if value.Result != nil {
			result := *value.Result
			value.Result = &result
		}
		if value.Offload != nil {
			offload := *value.Offload
			value.Offload = &offload
		}
		value.MutatedPaths = slices.Clone(value.MutatedPaths)
		if value.Failure != nil {
			failure := *value.Failure
			value.Failure = &failure
		}
		return value, true
	case CompactionBoundary:
		return value, true
	case SegmentInterrupted:
		interrupts := make([]Interrupt, len(value.Interrupts))
		for index, request := range value.Interrupts {
			interrupts[index] = cloneInterrupt(request)
		}
		value.Interrupts = interrupts
		return value, true
	case SegmentEnded:
		if value.Failure != nil {
			failure := *value.Failure
			value.Failure = &failure
		}
		if value.Usage != nil {
			usage := *value.Usage
			usage.ByModel = slices.Clone(usage.ByModel)
			value.Usage = &usage
		}
		return value, true
	case UsageReported:
		value.ByModel = slices.Clone(value.ByModel)
		return value, true
	case PlanUpdated:
		return value, true
	case SteerMessagesApplied:
		value.Messages = slices.Clone(value.Messages)
		for index := range value.Messages {
			value.Messages[index].Content = transcript.CloneContent(value.Messages[index].Content)
		}
		return value, true
	default:
		return nil, false
	}
}

// Complete resolves the producer receipt after the consumer has committed or
// rejected the fact. The Run pump is the production consumer; focused executor
// harnesses may use the same handshake with their own transactional fake.
func (e ExecutionFactCommit) Complete(err error) {
	if e.state == nil {
		return
	}
	e.state.once.Do(func() {
		e.state.done <- err
		close(e.state.done)
	})
}

// Await waits for the authoritative projection result or for ctx to stop
// waiting. Context cancellation never turns an uncommitted fact into success.
func (e ExecutionFactReceipt) Await(ctx context.Context) error {
	if e.state == nil || e.state.done == nil {
		return errors.New("runs: malformed execution fact receipt")
	}
	select {
	case err := <-e.state.done:
		return err
	case <-ctx.Done():
		return context.Cause(ctx)
	}
}
