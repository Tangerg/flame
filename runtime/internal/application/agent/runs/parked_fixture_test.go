package runs

import (
	"slices"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

// parkedFacts are the facts a test hand-off's Runs own and the hand-off only
// names: the root's Goal incarnation and capabilities.
type parkedFacts struct {
	goalIncarnationID string
	capabilities      run.Capabilities
}

// fixtureFacts is the contract a fixture hand-off implies: exactly the interrupt
// kinds it parks on, and child Runs when it has more than one member.
func fixtureFacts(pending Pending) parkedFacts {
	var kinds []interrupt.Kind
	for _, value := range pending.Interrupts {
		if !slices.Contains(kinds, value.Kind) {
			kinds = append(kinds, value.Kind)
		}
	}
	return parkedFacts{capabilities: run.Capabilities{
		ChildRuns:      len(pending.Continuations) > 1,
		InterruptKinds: kinds,
	}.Normalized()}
}

// resumedTreeCapabilities is the contract of the question tree fixtures, held
// even when a fixture has been reduced to fewer members or kinds.
func resumedTreeCapabilities() run.Capabilities {
	return run.Capabilities{ChildRuns: true, InterruptKinds: []interrupt.Kind{interrupt.Question}}
}

// fixtureLineage is the topology every tree fixture in this package shares:
// run_a and run_b are children of the root, and run_grandchild is run_a's.
func fixtureLineage(rootRunID, runID string) run.Lineage {
	switch runID {
	case rootRunID:
		return run.Lineage{}
	case "run_a", "run_b":
		return run.Lineage{SpawnedByItemID: "item_spawn_" + runID[len("run_"):], ParentRunID: rootRunID, RootRunID: rootRunID}
	case "run_grandchild":
		return run.Lineage{SpawnedByItemID: "item_spawn_grandchild", ParentRunID: "run_a", RootRunID: rootRunID}
	default:
		panic("fixture Run " + runID + " has no lineage")
	}
}

func runForPending(pending Pending) run.Run {
	root, _ := pending.RootContinuation()
	return runForContinuation(pending, root)
}

func runForContinuation(pending Pending, continuation Continuation) run.Run {
	return runWithFacts(pending, continuation, fixtureFacts(pending))
}

func runWithFacts(pending Pending, continuation Continuation, facts parkedFacts) run.Run {
	goalIncarnationID := ""
	if continuation.RunID == pending.RootRunID {
		goalIncarnationID = facts.goalIncarnationID
	}
	return testsupport.MustRestoreRun(run.Snapshot{ID: continuation.RunID,
		SessionID:         pending.SessionID,
		ModelSelection:    testsupport.DefaultModelSelection(),
		GoalIncarnationID: goalIncarnationID,
		State:             run.Waiting,
		Capabilities:      facts.capabilities,
		CreatedAt:         pending.CreatedAt.Add(-time.Second),
		MessageMark:       run.UnknownMessageMark,
		Lineage:           fixtureLineage(pending.RootRunID, continuation.RunID),
	})
}

// parkedTree is every Run a fixture hand-off names, in its continuation order.
func parkedTree(pending Pending, facts parkedFacts) []run.Run {
	parked := make([]run.Run, 0, len(pending.Continuations))
	for _, continuation := range pending.Continuations {
		parked = append(parked, runWithFacts(pending, continuation, facts))
	}
	return parked
}
