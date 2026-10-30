package testsupport

import (
	"time"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
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
func DefaultModelSelection() modelchoice.Selection {
	selection, _ := modelchoice.New("anthropic", "claude")
	return selection
}

// MustModelSelection builds the exact provider/model pair a fixture names. A
// pair that does not resolve is a broken fixture rather than a behavior under
// test, so it panics like the rest of this package instead of reporting a
// failure the test would have to interpret.
func MustModelSelection(provider, model string) modelchoice.Selection {
	selection, err := modelchoice.New(provider, model)
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
func pointer[T any](value T) *T { return &value }

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
	if snapshot.Ending != nil && snapshot.State == run.Running {
		snapshot.State = run.Finished
	}
	if snapshot.State.IsTerminal() && snapshot.Ending == nil {
		snapshot.Ending = run.Completed{}
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
	}
	if !snapshot.State.IsTerminal() || snapshot.Lineage.IsChild() {
		snapshot.MessageMark = run.UnknownMessageMark()
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

// MustRunReplacement returns one Run replacement or panics.
func MustRunReplacement(replacement run.Replacement, err error) run.Replacement {
	if err != nil {
		panic(err)
	}
	return replacement
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
		resume.Runs[index] = MustRunReplacement(waiting.Replace().Resume(segmentID, resumedAt))
	}
	return resume
}

// MustParkedRun restores a waiting Run of sessionID created at createdAt.
func MustParkedRun(runID, sessionID string, createdAt time.Time) run.Run {
	return MustRestoreRun(run.Snapshot{ID: runID, SessionID: sessionID, State: run.Waiting, CreatedAt: createdAt})
}

// MustProgressedRun admits draft and advances it to one model-response
// boundary, as the reducer does before committing progress, or panics.
func MustProgressedRun(draft run.Draft, metrics run.Metrics, contextTokens int64, at time.Time) run.Run {
	admitted, err := run.Admit(RunDraft(draft))
	if err != nil {
		panic(err)
	}
	progressed, err := admitted.AdvanceProgress(metrics, contextTokens, at)
	if err != nil {
		panic(err)
	}
	return progressed
}

// RunOutcome is the terminal reason of a finished fixture Run, or "" while open.
func RunOutcome(value run.Run) run.Outcome {
	outcome, _ := value.Outcome()
	return outcome
}

// RunFailed reports the outcomes that end a Run without completing or being canceled.
func RunFailed(value run.Run) bool {
	switch RunOutcome(value) {
	case run.OutcomeFailed, run.OutcomeTimedOut, run.OutcomeLost:
		return true
	default:
		return false
	}
}

// EndingFor returns a representative ending for outcome, for fixtures that
// only care which way a Run ended.
func EndingFor(outcome run.Outcome) run.Ending {
	switch outcome {
	case run.OutcomeCanceled:
		return run.Canceled{}
	case run.OutcomeFailed:
		return run.Failed{Failure: run.Failure{Kind: run.FailureInternal}}
	case run.OutcomeTimedOut:
		return run.TimedOut{}
	case run.OutcomeLost:
		return run.Lost{Loss: run.Loss{Reason: run.LossRestart}}
	default:
		return run.Completed{}
	}
}

// Cost returns an exact priced amount for test usage literals.
func Cost(usd float64) accounting.Cost {
	cost, err := accounting.NewCost(usd)
	if err != nil {
		panic(err)
	}
	return cost
}

// OptionalCost converts a test fixture's optional dollar amount; nil is unpriced.
func OptionalCost(usd *float64) accounting.Cost {
	cost, err := accounting.CostFromOptional(usd)
	if err != nil {
		panic(err)
	}
	return cost
}
