package sessions

import (
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/toolresult"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
)

// copyForkSnapshot projects the selected terminal boundary into the child under
// fresh global identities. A Session fork is one new aggregate, not a second
// owner for the parent's Run/Item/blob primary keys. The remap happens here in
// the use case so persistence only commits an already-coherent write set and no
// client has to synthesize the history that the model received.
func (c *Coordinator) copyForkSnapshot(
	source Snapshot,
	child session.Session,
	boundary ForkBoundary,
	steps []plan.Step,
) (Snapshot, error) {
	if len(boundary.RunIDs) == 0 {
		return Snapshot{Session: child, Messages: boundary.Messages, Plan: steps}, nil
	}
	projection := newForkSnapshotProjection(c, source, child, boundary, steps)
	if err := projection.selectRuns(); err != nil {
		return Snapshot{}, err
	}
	projection.selectItems()
	if err := projection.copyRuns(); err != nil {
		return Snapshot{}, err
	}
	if err := projection.copyItems(); err != nil {
		return Snapshot{}, err
	}
	projection.copyToolResults()
	return projection.finish()
}

type forkSnapshotProjection struct {
	coordinator    *Coordinator
	source         Snapshot
	child          session.Session
	boundary       ForkBoundary
	runByID        map[string]run.Run
	selectedRunIDs map[string]struct{}
	runIDs         map[string]string
	itemIDs        map[string]string
	forked         Snapshot
}

func newForkSnapshotProjection(
	coordinator *Coordinator,
	source Snapshot,
	child session.Session,
	boundary ForkBoundary,
	steps []plan.Step,
) *forkSnapshotProjection {
	runByID := make(map[string]run.Run, len(source.Runs))
	for _, value := range source.Runs {
		runByID[value.ID()] = value
	}
	return &forkSnapshotProjection{
		coordinator:    coordinator,
		source:         source,
		child:          child,
		boundary:       boundary,
		runByID:        runByID,
		selectedRunIDs: make(map[string]struct{}, len(boundary.RunIDs)),
		runIDs:         make(map[string]string, len(boundary.RunIDs)),
		itemIDs:        make(map[string]string),
		forked: Snapshot{
			Session:  child,
			Messages: boundary.Messages,
			Plan:     steps,
		},
	}
}

func (projection *forkSnapshotProjection) selectRuns() error {
	for _, sourceID := range projection.boundary.RunIDs {
		value, found := projection.runByID[sourceID]
		if !found {
			return fmt.Errorf("sessions: fork boundary references missing run %q", sourceID)
		}
		if !value.State().IsTerminal() {
			return fmt.Errorf("sessions: fork boundary run %q is not terminal", sourceID)
		}
		projection.selectedRunIDs[sourceID] = struct{}{}
		projection.runIDs[sourceID] = projection.coordinator.newRunID()
	}
	projection.forked.Runs = make([]run.Run, 0, len(projection.boundary.RunIDs))
	return nil
}

func (projection *forkSnapshotProjection) selectItems() {
	for _, item := range projection.source.Items {
		if _, selected := projection.selectedRunIDs[item.RunID()]; selected {
			projection.itemIDs[item.ID()] = projection.coordinator.newItemID()
		}
	}
	projection.forked.Items = make([]transcript.Item, 0, len(projection.itemIDs))
}

func (projection *forkSnapshotProjection) copyRuns() error {
	for _, sourceID := range projection.boundary.RunIDs {
		value := projection.runByID[sourceID]
		lineage, err := projection.remapLineage(sourceID, value.Lineage())
		if err != nil {
			return err
		}
		copied, err := value.Fork(projection.child.ID(), projection.runIDs[sourceID], lineage)
		if err != nil {
			return fmt.Errorf("sessions: copy fork run %q: %w", sourceID, err)
		}
		projection.forked.Runs = append(projection.forked.Runs, copied)
	}
	return nil
}

func (projection *forkSnapshotProjection) remapLineage(
	sourceID string,
	lineage run.Lineage,
) (run.Lineage, error) {
	if !lineage.IsChild() {
		return lineage, nil
	}
	spawnedBy, itemFound := projection.itemIDs[lineage.SpawnedByItemID]
	parentID, parentFound := projection.runIDs[lineage.ParentRunID]
	rootID, rootFound := projection.runIDs[lineage.RootRunID]
	if !itemFound || !parentFound || !rootFound {
		return run.Lineage{}, fmt.Errorf("sessions: fork run %q has lineage outside the selected boundary", sourceID)
	}
	return run.Lineage{
		SpawnedByItemID: spawnedBy,
		ParentRunID:     parentID,
		RootRunID:       rootID,
	}, nil
}

func (projection *forkSnapshotProjection) copyItems() error {
	for _, value := range projection.source.Items {
		newID, selected := projection.itemIDs[value.ID()]
		if !selected {
			continue
		}
		copied, err := value.Fork(
			projection.child.ID(),
			projection.runIDs[value.RunID()],
			newID,
		)
		if err != nil {
			return fmt.Errorf("sessions: copy fork item %q: %w", value.ID(), err)
		}
		projection.forked.Items = append(projection.forked.Items, copied)
	}
	return nil
}

func (projection *forkSnapshotProjection) copyToolResults() {
	blobs := make(map[toolresult.ID]toolresult.Blob, len(projection.source.ToolResults))
	for _, blob := range projection.source.ToolResults {
		blobs[blob.ID] = blob
	}
	for _, item := range projection.forked.Items {
		invocation, present := item.ToolInvocation()
		if !present || invocation.Offload == nil {
			continue
		}
		blob, found := blobs[invocation.Offload.ID]
		if !found {
			continue
		}
		blob.SessionID = projection.child.ID()
		projection.forked.ToolResults = append(projection.forked.ToolResults, blob)
	}
}

func (projection *forkSnapshotProjection) finish() (Snapshot, error) {
	if err := projection.forked.Validate(); err != nil {
		return Snapshot{}, fmt.Errorf("sessions: validate fork snapshot: %w", err)
	}
	return projection.forked, nil
}
