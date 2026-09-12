package agentexec

import (
	"errors"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
)

// interactionModelFailures owns the product classification of stopped model
// calls. Scope preserves their unknown external outcome and records the host's
// cancellation; Runtime projects the cause for the member that observed it.
type interactionModelFailures struct {
	mu        sync.Mutex
	byProcess map[agent.ProcessID]run.Failure
}

func newInteractionModelFailures() interactionModelFailures {
	return interactionModelFailures{byProcess: make(map[agent.ProcessID]run.Failure)}
}

func (i *interactionModelFailures) record(processID agent.ProcessID, cause error) {
	if i == nil || !processID.Valid() || cause == nil {
		return
	}
	failure := run.Failure{
		Kind:   run.FailureProviderUnavailable,
		Detail: executorDiagnostic(cause),
	}
	if errors.Is(cause, interaction.ErrHostFailure) {
		failure.Kind = run.FailureInternal
	} else if classified, ok := errors.AsType[*run.FailureError](cause); ok {
		delay := classified.RetryAfter
		if delay < 0 || !classified.Kind.AllowsRetryAfter() {
			delay = 0
		}
		candidate := run.Failure{
			Kind:       classified.Kind,
			Detail:     failure.Detail,
			RetryAfter: delay,
		}
		if candidate.Validate() == nil {
			failure = candidate
		}
	}
	if failure.Detail == "" {
		failure.Detail = "model provider failed"
	}
	i.mu.Lock()
	i.byProcess[processID] = failure
	i.mu.Unlock()
}

func (i *interactionModelFailures) take(processID agent.ProcessID) (run.Failure, bool) {
	if i == nil || !processID.Valid() {
		return run.Failure{}, false
	}
	i.mu.Lock()
	defer i.mu.Unlock()
	failure, found := i.byProcess[processID]
	delete(i.byProcess, processID)
	return failure, found
}

func (i *interactionModelFailures) has(processID agent.ProcessID) bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	_, found := i.byProcess[processID]
	return found
}
