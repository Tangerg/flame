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

// MustModelSelection builds the exact provider/model pair a fixture names. A
// pair that does not resolve is a broken fixture rather than a behavior under
// test, so it panics like the rest of this package instead of reporting a
// failure the test would have to interpret.
func MustModelSelection(provider, model string) modelref.Selection {
	selection, err := modelref.New(provider, model)
	if err != nil {
		panic(err)
	}
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
	}
	if !snapshot.State.IsTerminal() || snapshot.Lineage.IsChild() {
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

// MustRunReplacement derives one Run replacement or panics.
func MustRunReplacement(expected run.Run, transition func(run.Run) (run.Run, error)) run.Replacement {
	replacement, err := run.Replace(expected, transition)
	if err != nil {
		panic(err)
	}
	return replacement
}

// DecidedRun is a transition that yields an already-built state, for tests
// that need a replacement whose state they constructed directly.
func DecidedRun(state run.Run) func(run.Run) (run.Run, error) {
	return func(run.Run) (run.Run, error) { return state, nil }
}

// MustResumeRuns derives one tree resume from parked Runs given in postorder
// (root last), reopening each into its paired Segment at resumedAt.
func MustResumeRuns(resumedAt time.Time, parked []run.Run, segmentIDs []string) run.TreeResumeDraft {
	if len(parked) != len(segmentIDs) {
		panic("testsupport: every parked Run needs one Segment")
	}
	resume := run.TreeResumeDraft{Runs: make([]run.Replacement, len(parked))}
	for index, waiting := range parked {
		segmentID := segmentIDs[index]
		resume.Runs[index] = MustRunReplacement(waiting, func(waiting run.Run) (run.Run, error) {
			return waiting.Resume(segmentID, resumedAt)
		})
	}
	return resume
}

// MustParkedRun restores a waiting Run of sessionID created at createdAt.
func MustParkedRun(runID, sessionID string, createdAt time.Time) run.Run {
	return MustRestoreRun(run.Snapshot{ID: runID, SessionID: sessionID, State: run.Waiting, CreatedAt: createdAt})
}
