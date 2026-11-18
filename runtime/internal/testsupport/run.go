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

func Usage(total accounting.Totals, byModel map[string]accounting.Totals) *accounting.Usage {
	usage, err := accounting.NewUsage(total, byModel)
	if err != nil {
		panic(err)
	}
	return &usage
}

// ChildLineage builds a child Run's lineage; an invalid edge set is a broken
// fixture, so it panics.
func ChildLineage(spawnedByItemID, parentRunID, rootRunID string) run.Lineage {
	lineage, err := run.NewLineage(spawnedByItemID, parentRunID, rootRunID)
	if err != nil {
		panic(err)
	}
	return lineage
}
