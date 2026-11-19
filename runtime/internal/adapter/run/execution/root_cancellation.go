package execution

import (
	"errors"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	scoperun "github.com/Tangerg/scope/agent/run"
)

type interactionRootCancellation struct {
	reason    string
	published bool
}

func (i *interactionSession) rootCancellationPublished() bool {
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	return i.state.rootCancellation != nil && i.state.rootCancellation.published
}

// Only a matching Scope Exit with no unresolved work authorizes the application
// cancellation. An internal stop or an uncertain Effect keeps its own outcome.
func (i *interactionSession) confirmedRootCancellation(tree scoperun.Log, records []scoperun.Record) (runs.SegmentEnded, *interactionRootCancellation, error) {
	i.state.mu.Lock()
	cancellation := i.state.rootCancellation
	i.state.mu.Unlock()
	if cancellation == nil {
		return runs.SegmentEnded{}, nil, nil
	}
	for _, record := range records {
		exit, ok := record.(scoperun.Exited)
		if !ok || exit.Process != i.rootID || exit.Termination.Cause() != scoperun.TerminationCauseHostCancellation ||
			exit.Termination.Reason() != cancellation.reason {
			continue
		}
		inspection, err := tree.Inspect()
		if err != nil {
			return runs.SegmentEnded{}, nil, err
		}
		for _, process := range inspection.Processes {
			if !process.Status.Terminal() || len(unresolvedProcessEffects(process)) != 0 {
				return runs.SegmentEnded{}, nil, nil
			}
		}
		root, found := inspection.Process(i.rootID)
		if !found {
			return runs.SegmentEnded{}, nil, errors.New("execution: canceled Scope root is missing from its inspection")
		}
		end := runs.NewSegmentEnded(run.Canceled{}, i.segmentClock.duration(root.StartedAt, root.FinishedAt))
		return end, cancellation, nil
	}
	return runs.SegmentEnded{}, nil, nil
}
