package testsupport

import (
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

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
func MustSessionID(text string) identity.SessionID {
	id, err := identity.ParseSession(text)
	if err != nil {
		panic(err)
	}
	return id
}

func MustRunID(text string) identity.RunID {
	id, err := identity.ParseRun(text)
	if err != nil {
		panic(err)
	}
	return id
}

func MustSegmentID(text string) identity.SegmentID {
	id, err := identity.ParseSegment(text)
	if err != nil {
		panic(err)
	}
	return id
}

func MustItemID(text string) identity.ItemID {
	id, err := identity.ParseItem(text)
	if err != nil {
		panic(err)
	}
	return id
}

func MustRestoreRun(snapshot run.Snapshot) run.Run {
	defaultRunIdentity(&snapshot)
	defaultRunLifecycle(&snapshot)
	defaultRunTimes(&snapshot)
	restored, err := run.Restore(snapshot)
	if err != nil {
		panic(err)
	}
	return restored
}

func defaultRunIdentity(snapshot *run.Snapshot) {
	if snapshot.ID.IsZero() {
		snapshot.ID = MustRunID("run_fixture")
	}
	if snapshot.SessionID.IsZero() {
		snapshot.SessionID = MustSessionID("session_fixture")
	}
	if snapshot.ModelSelection.Provider() == "" && snapshot.ModelSelection.Model() == "" &&
		snapshot.ModelSelection.ReasoningEffort() == "" {
		snapshot.ModelSelection = DefaultModelSelection()
	}
}

// defaultRunLifecycle derives a consistent state from what the fixture names:
// an ending finishes the Run, a terminal Run has an ending, and only a
// Running Run has an active Segment.
func defaultRunLifecycle(snapshot *run.Snapshot) {
	if snapshot.State == "" {
		snapshot.State = run.Running
	}
	if snapshot.Ending != nil && snapshot.State == run.Running {
		snapshot.State = run.Finished
	}
	if snapshot.State.IsTerminal() && snapshot.Ending == nil {
		snapshot.Ending = run.Completed{}
	}
	if snapshot.State == run.Running && snapshot.ActiveSegmentID.IsZero() {
		snapshot.ActiveSegmentID = MustSegmentID("segment_fixture")
	}
	if snapshot.State != run.Running {
		snapshot.ActiveSegmentID = identity.SegmentID{}
	}
	if !snapshot.State.IsTerminal() || snapshot.Lineage.IsChild() {
		snapshot.MessageMark = run.UnknownMessageMark()
	}
}

func defaultRunTimes(snapshot *run.Snapshot) {
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Unix(1, 0).UTC()
	}
	if snapshot.State.IsTerminal() && snapshot.FinishedAt.IsZero() {
		snapshot.FinishedAt = snapshot.CreatedAt
	}
	if snapshot.UpdatedAt.IsZero() {
		snapshot.UpdatedAt = snapshot.CreatedAt
		if !snapshot.FinishedAt.IsZero() {
			snapshot.UpdatedAt = snapshot.FinishedAt
		}
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

func Usage(total accounting.Totals, byModel map[string]accounting.Totals) *accounting.Usage {
	usage, err := accounting.NewUsage(total, byModel)
	if err != nil {
		panic(err)
	}
	return &usage
}

// ChildLineage builds a child Run's lineage; an invalid edge set is a broken
// fixture, so it panics.
func ChildLineage(spawnedByItemID identity.ItemID, parentRunID, rootRunID identity.RunID) run.Lineage {
	lineage, err := run.NewLineage(spawnedByItemID, parentRunID, rootRunID)
	if err != nil {
		panic(err)
	}
	return lineage
}
