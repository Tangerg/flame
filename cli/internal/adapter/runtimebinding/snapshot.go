package runtimebinding

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

type snapshotBinding interface {
	GetSessionSnapshot(context.Context, protocol.GetSessionSnapshotRequest, flameruntime.CallOptions) (*protocol.SessionSnapshot, error)
}

// GetSession projects one complete Runtime snapshot into terminal presentation.
func (r *Connection) GetSession(ctx context.Context, sessionID string) (agent.SessionSnapshot, error) {
	request := protocol.GetSessionSnapshotRequest{
		SessionID: sessionID, IncludeDescendants: r.profile.Supports(protocol.FeatureSubagents),
	}
	if err := request.ValidateWire(); err != nil {
		return agent.SessionSnapshot{}, fmt.Errorf("get session: %w", err)
	}
	material, err := r.readMaterialSnapshot(ctx, request)
	if err != nil {
		return agent.SessionSnapshot{}, err
	}
	projected, err := projectSnapshot(material)
	if err != nil {
		return agent.SessionSnapshot{}, runtimeContractViolation("get session projection is invalid: %v", err)
	}
	return projected, nil
}

func (r *Connection) readMaterialSnapshot(
	ctx context.Context,
	request protocol.GetSessionSnapshotRequest,
) (protocol.SessionSnapshot, error) {
	snapshot, err := r.snapshot.GetSessionSnapshot(ctx, request, r.callOptions())
	if err != nil {
		return protocol.SessionSnapshot{}, classifyError(err)
	}
	if snapshot == nil {
		return protocol.SessionSnapshot{}, runtimeContractViolation("get session snapshot returned nil")
	}
	planEnabled := r.profile.Supports(protocol.FeaturePlan)
	if planEnabled && snapshot.Plan == nil {
		return protocol.SessionSnapshot{}, runtimeContractViolation("get session snapshot omitted plan while the plan feature is enabled")
	}
	if !planEnabled && snapshot.Plan != nil {
		return protocol.SessionSnapshot{}, runtimeContractViolation("get session snapshot returned plan while the plan feature is disabled")
	}
	if !r.profile.Supports(protocol.FeatureGoals) && snapshot.Goal != nil {
		return protocol.SessionSnapshot{}, runtimeContractViolation("get session snapshot returned goal while the goals feature is disabled")
	}
	if err := protocol.ValidateWireTree(*snapshot); err != nil {
		return protocol.SessionSnapshot{}, runtimeContractViolation("get session snapshot is invalid: %v", err)
	}
	if snapshot.Session.ID != request.SessionID {
		return protocol.SessionSnapshot{}, runtimeContractViolation("get session snapshot returned id %q for %q", snapshot.Session.ID, request.SessionID)
	}
	return *snapshot, nil
}

func projectSnapshot(read protocol.SessionSnapshot) (agent.SessionSnapshot, error) {
	session := read.Session
	var err error
	snapshot := agent.SessionSnapshot{Session: session, Transcript: make([]agent.Block, 0, len(read.Items))}
	for _, value := range read.Items {
		block, projectItemErr := projectItem(value)
		if projectItemErr != nil {
			return agent.SessionSnapshot{}, projectItemErr
		}
		snapshot.Transcript = append(snapshot.Transcript, block)
	}
	orderedRuns := slices.Clone(read.Runs)
	slices.SortFunc(orderedRuns, func(first, second protocol.RunRef) int {
		return cmp.Or(first.CreatedAt.Compare(second.CreatedAt), cmp.Compare(first.ID, second.ID))
	})
	snapshot.Runs = make([]agent.Run, 0, len(orderedRuns))
	for _, value := range orderedRuns {
		run, projectRunErr := projectRun(value)
		if projectRunErr != nil {
			return agent.SessionSnapshot{}, projectRunErr
		}
		snapshot.Runs = append(snapshot.Runs, run)
	}
	if read.Plan != nil {
		snapshot.Plan, err = projectPlan(read.Plan)
		if err != nil {
			return agent.SessionSnapshot{}, err
		}
	}
	if read.Goal != nil {
		projected := cloneGoal(*read.Goal)
		snapshot.Goal = &projected
	}
	if active, ok := snapshot.ActiveRun(); ok && active.Status == protocol.RunStatusWaiting {
		if len(read.Interrupts) != 1 {
			return agent.SessionSnapshot{}, fmt.Errorf("waiting run %s has %d pending interrupt sets", active.ID, len(read.Interrupts))
		}
		set := read.Interrupts[0]
		if err := protocol.ValidateWireTree(set); err != nil {
			return agent.SessionSnapshot{}, fmt.Errorf("waiting run %s has an invalid pending interrupt set: %w", active.ID, err)
		}
		if set.SessionID != session.ID {
			return agent.SessionSnapshot{}, fmt.Errorf("waiting run %s has a pending interrupt set for session %s", active.ID, set.SessionID)
		}
		if set.RootRunID != active.ID {
			return agent.SessionSnapshot{}, fmt.Errorf("waiting run %s has a pending interrupt set for root %s", active.ID, set.RootRunID)
		}
		snapshot.Interactions, err = projectInteractions(set.Interrupts)
		if err != nil {
			return agent.SessionSnapshot{}, err
		}
	} else if len(read.Interrupts) != 0 {
		return agent.SessionSnapshot{}, fmt.Errorf("session %s has interrupts without a waiting root run", session.ID)
	}

	return snapshot, nil
}
