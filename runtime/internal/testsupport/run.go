package testsupport

import (
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
)

// RunMetricsInput names the values accepted by MustRunMetrics.
type RunMetricsInput struct {
	Usage          *accounting.Usage
	Steps          int
	ActiveDuration time.Duration
}

// DefaultModelSelection returns the deterministic model identity MustRestoreRun supplies when
// a fixture omits one.
func DefaultModelSelection() modelref.Selection {
	selection, _ := modelref.New("anthropic", "claude")
	return selection
}

// RunDraft supplies the deterministic model identity used by valid Run fixtures
// when the behavior under test does not care which model executes the Run.
func RunDraft(draft run.Draft) run.Draft {
	if draft.ModelSelection.Provider() == "" && draft.ModelSelection.Model() == "" &&
		draft.ModelSelection.ReasoningEffort() == "" {
		draft.ModelSelection = DefaultModelSelection()
	}
	return draft
}

// MustRunMetrics constructs valid metrics or panics. It is intended only for
// fixtures whose validity is not the behavior under test.
func MustRunMetrics(input RunMetricsInput) run.Metrics {
	metrics, err := run.NewMetrics(input.Usage, input.Steps, input.ActiveDuration)
	if err != nil {
		panic(err)
	}
	return metrics
}

// Pointer returns an owned pointer for explicit optional fixture values.
func Pointer[T any](value T) *T { return &value }

// MustRunLimits constructs a valid limited execution policy or panics. Use
// run.UnlimitedLimits when a fixture intentionally has no execution cap.
func MustRunLimits(values run.LimitValues) run.Limits {
	limits, err := run.NewLimits(values)
	if err != nil {
		panic(err)
	}
	return limits
}

// MustRestoreRun constructs a valid Run or panics. Tests exercising invalid
// snapshots must call run.Restore themselves and assert the returned error.
func MustRestoreRun(snapshot run.Snapshot) run.Run {
	if snapshot.ID == "" {
		snapshot.ID = "run_fixture"
	}
	if snapshot.SessionID == "" {
		snapshot.SessionID = "session_fixture"
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Unix(1, 0).UTC()
	}
	if snapshot.ModelSelection.Provider() == "" && snapshot.ModelSelection.Model() == "" &&
		snapshot.ModelSelection.ReasoningEffort() == "" {
		snapshot.ModelSelection = DefaultModelSelection()
	}
	if snapshot.State == "" {
		snapshot.State = run.Running
	}
	if snapshot.Outcome != nil && snapshot.State == run.Running {
		if terminal, ok := run.Running.Terminate(*snapshot.Outcome); ok {
			snapshot.State = terminal
		}
	}
	if snapshot.State.IsTerminal() && snapshot.Outcome == nil {
		var outcome run.Outcome
		switch snapshot.State {
		case run.Completed:
			outcome = run.OutcomeCompleted
		case run.Canceled:
			outcome = run.OutcomeCanceled
		case run.Failed:
			outcome = run.OutcomeFailed
		}
		snapshot.Outcome = &outcome
	}
	if snapshot.State == run.Running && snapshot.ActiveSegmentID == "" {
		snapshot.ActiveSegmentID = "segment_fixture"
	}
	if snapshot.State != run.Running {
		snapshot.ActiveSegmentID = ""
	}
	if snapshot.State.IsTerminal() {
		if snapshot.FinishedAt.IsZero() {
			snapshot.FinishedAt = snapshot.CreatedAt
		}
		if snapshot.Failure == nil {
			switch *snapshot.Outcome {
			case run.OutcomeFailed:
				snapshot.Failure = &run.Failure{Kind: run.FailureInternal}
			case run.OutcomeTimedOut:
				snapshot.Failure = &run.Failure{Kind: run.FailureTimeout}
			case run.OutcomeLost:
				snapshot.Failure = &run.Failure{Kind: run.FailureLost}
			}
		}
	} else {
		snapshot.MessageMark = run.UnknownMessageMark
	}
	if snapshot.UpdatedAt.IsZero() {
		if !snapshot.FinishedAt.IsZero() {
			snapshot.UpdatedAt = snapshot.FinishedAt
		} else {
			snapshot.UpdatedAt = snapshot.CreatedAt
		}
	}
	restored, err := run.Restore(snapshot)
	if err != nil {
		panic(err)
	}
	return restored
}
