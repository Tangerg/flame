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
	if snapshot.ModelSelection.Provider() == "" && snapshot.ModelSelection.Model() == "" &&
		snapshot.ModelSelection.ReasoningEffort() == "" {
		snapshot.ModelSelection = DefaultModelSelection()
	}
	restored, err := run.Restore(snapshot)
	if err != nil {
		panic(err)
	}
	return restored
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
