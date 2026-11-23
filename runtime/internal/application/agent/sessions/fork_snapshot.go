package sessions

import (
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

func (c *Coordinator) copyForkMaterial(
	source ForkMaterial,
	child session.Session,
	boundary ForkBoundary,
) (ForkMaterial, error) {
	if len(boundary.RunIDs) == 0 {
		return ForkMaterial{HistorySnapshot: HistorySnapshot{Session: child, Messages: boundary.Messages}}, nil
	}
	projection := newForkSnapshotProjection(c, source, child, boundary)
	if err := projection.selectRuns(); err != nil {
		return ForkMaterial{}, err
	}
	projection.selectItems()
	if err := projection.copyRuns(); err != nil {
		return ForkMaterial{}, err
	}
	if err := projection.copyItems(); err != nil {
		return ForkMaterial{}, err
	}
	projection.copyToolResults()
	for _, capture := range source.PlanBoundaries {
		if id, copied := projection.runIDs[capture.RunID]; copied {
			capture.RunID = id
			projection.forked.PlanBoundaries = append(projection.forked.PlanBoundaries, capture)
		}
	}
	return projection.forked, nil
}

type forkSnapshotProjection struct {
	coordinator *Coordinator
	source      ForkMaterial
	childID     identity.SessionID
	boundary    ForkBoundary
	runByID     map[identity.RunID]run.Run
	runIDs      map[identity.RunID]identity.RunID
	itemIDs     map[identity.ItemID]identity.ItemID
	forked      ForkMaterial
}

func newForkSnapshotProjection(
	coordinator *Coordinator,
	source ForkMaterial,
	child session.Session,
	boundary ForkBoundary,
) *forkSnapshotProjection {
	childID := child.ID()
	runByID := make(map[identity.RunID]run.Run, len(source.Runs))
	for _, value := range source.Runs {
		runByID[value.ID()] = value
	}
	return &forkSnapshotProjection{
		coordinator: coordinator,
		source:      source,
		childID:     childID,
		boundary:    boundary,
		runByID:     runByID,
		runIDs:      make(map[identity.RunID]identity.RunID, len(boundary.RunIDs)),
		itemIDs:     make(map[identity.ItemID]identity.ItemID),
		forked: ForkMaterial{
			HistorySnapshot: HistorySnapshot{Session: child, Messages: boundary.Messages},
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
		projection.runIDs[sourceID] = projection.coordinator.newRunID()
	}
	projection.forked.Runs = make([]run.Run, 0, len(projection.boundary.RunIDs))
	return nil
}

func (projection *forkSnapshotProjection) selectItems() {
	for _, item := range projection.source.Items {
		if _, selected := projection.runIDs[item.RunID()]; selected {
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
		copied, err := value.Fork(projection.childID, projection.runIDs[sourceID], lineage)
		if err != nil {
			return fmt.Errorf("sessions: copy fork run %q: %w", sourceID, err)
		}
		projection.forked.Runs = append(projection.forked.Runs, copied)
	}
	return nil
}

func (projection *forkSnapshotProjection) remapLineage(
	sourceID identity.RunID,
	lineage run.Lineage,
) (run.Lineage, error) {
	if !lineage.IsChild() {
		return lineage, nil
	}
	spawnedBy, itemFound := projection.itemIDs[lineage.SpawnedByItemID()]
	parentID, parentFound := projection.runIDs[lineage.ParentRunID()]
	rootID, rootFound := projection.runIDs[lineage.RootRunID()]
	if !itemFound || !parentFound || !rootFound {
		return run.Lineage{}, fmt.Errorf("sessions: fork run %q has lineage outside the selected boundary", sourceID)
	}
	return run.NewLineage(spawnedBy, parentID, rootID)
}

func (projection *forkSnapshotProjection) copyItems() error {
	for _, value := range projection.source.Items {
		newID, selected := projection.itemIDs[value.ID()]
		if !selected {
			continue
		}
		copied, err := value.Fork(
			projection.childID,
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
	blobs := make(map[transcript.ToolResultID]transcript.ToolResultBlob, len(projection.source.ToolResults))
	for _, blob := range projection.source.ToolResults {
		blobs[blob.ID] = blob
	}
	for _, item := range projection.forked.Items {
		call, present := item.ToolCall()
		invocation := call.Invocation
		if !present || invocation.Offload == nil {
			continue
		}
		blob, found := blobs[invocation.Offload.ID]
		if !found {
			continue
		}
		blob.SessionID = projection.childID
		projection.forked.ToolResults = append(projection.forked.ToolResults, blob)
	}
}
