package testsupport

import (
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goalref"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
)

type RunMetricsInput struct {
	Usage          *accounting.Usage
	Steps          int
	ActiveDuration time.Duration
}

func DefaultModelSelection() flamemodel.Selection {
	selection, _ := flamemodel.New("anthropic", "claude")
	return selection
}

func MustModelSelection(provider, model string) flamemodel.Selection {
	selection, err := flamemodel.New(provider, model)
	if err != nil {
		panic(err)
	}
	return selection
}

func RunDraft(draft run.Draft) run.Draft {
	if draft.ModelSelection.Provider() == "" && draft.ModelSelection.Model() == "" &&
		draft.ModelSelection.ReasoningEffort() == "" {
		draft.ModelSelection = DefaultModelSelection()
	}
	return draft
}

func MustRunMetrics(input RunMetricsInput) run.Metrics {
	metrics, err := run.NewMetrics(input.Usage, input.Steps, input.ActiveDuration)
	if err != nil {
		panic(err)
	}
	return metrics
}

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
		snapshot.ID = MustRunID(fixtureRunID)
	}
	if snapshot.SessionID.IsZero() {
		snapshot.SessionID = MustSessionID(fixtureSessionID)
	}
	if snapshot.ModelSelection.Provider() == "" && snapshot.ModelSelection.Model() == "" &&
		snapshot.ModelSelection.ReasoningEffort() == "" {
		snapshot.ModelSelection = DefaultModelSelection()
	}
}

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

func ChildLineage(spawnedByItemID identity.ItemID, parentRunID, rootRunID identity.RunID) run.Lineage {
	lineage, err := run.NewLineage(spawnedByItemID, parentRunID, rootRunID)
	if err != nil {
		panic(err)
	}
	return lineage
}

func MustIncarnation(text string) goalref.IncarnationID {
	return must(goalref.ParseIncarnation(text))
}
