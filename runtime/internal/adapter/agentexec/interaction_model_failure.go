package agentexec

import (
	"errors"
	"sync"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	agent "github.com/Tangerg/scope/agent"
)

// interactionModelFailures retains the provider-neutral classification that
// Scope's snapshot-safe Process failure cannot carry through its string-only
// diagnostic. A failure belongs to the Process whose model call observed it
// and is consumed exactly once when that Process is projected terminal.
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
	if classified, ok := errors.AsType[*run.FailureError](cause); ok {
		candidate := run.Failure{
			Kind:       classified.Kind,
			Detail:     failure.Detail,
			RetryAfter: classified.RetryAfter,
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
