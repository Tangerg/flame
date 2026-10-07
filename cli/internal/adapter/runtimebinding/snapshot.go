package runtimebinding

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"
)

type snapshotBinding interface {
	GetSessionSnapshot(context.Context, protocol.GetSessionSnapshotRequest, flameruntime.CallOptions) (*protocol.SessionSnapshot, error)
}

type coldRead struct {
	session    protocol.Session
	runs       []protocol.RunRef
	items      []protocol.Item
	plan       *protocol.Plan
	goal       *protocol.Goal
	interrupts []protocol.PendingInterruptSet
}

// GetSession reads one Session and its material from a single Runtime
// snapshot, which carries the Session it was read from.
func (r *Connection) GetSession(ctx context.Context, sessionID string) (conversation.SessionSnapshot, error) {
	request := protocol.GetSessionSnapshotRequest{
		SessionID: sessionID, IncludeDescendants: r.profile.Supports(protocol.FeatureSubagents),
	}
	if err := request.ValidateWire(); err != nil {
		return conversation.SessionSnapshot{}, fmt.Errorf("get session: %w", err)
	}
	snapshot, err := r.snapshot.GetSessionSnapshot(ctx, request, r.callOptions())
	if err != nil {
		return conversation.SessionSnapshot{}, classifyError(err)
	}
	if snapshot == nil {
		return conversation.SessionSnapshot{}, runtimeContractViolation("get session snapshot returned nil")
	}
	material, err := r.snapshotMaterial(sessionID, snapshot)
	if err != nil {
		return conversation.SessionSnapshot{}, err
	}
	projected, err := projectSnapshot(material)
	if err != nil {
		return conversation.SessionSnapshot{}, runtimeContractViolation("get session projection is invalid: %v", err)
	}
	return projected, nil
}

func (r *Connection) snapshotMaterial(sessionID string, snapshot *protocol.SessionSnapshot) (coldRead, error) {
	if snapshot.Session.ID != sessionID {
		return coldRead{}, runtimeContractViolation("session snapshot returned session %q for %q", snapshot.Session.ID, sessionID)
	}
	planEnabled := r.profile.Supports(protocol.FeaturePlan)
	if planEnabled && snapshot.Plan == nil {
		return coldRead{}, runtimeContractViolation("get session snapshot omitted plan while the plan feature is enabled")
	}
	if !planEnabled && snapshot.Plan != nil {
		return coldRead{}, runtimeContractViolation("get session snapshot returned plan while the plan feature is disabled")
	}
	if !r.profile.Supports(protocol.FeatureGoals) && snapshot.Goal != nil {
		return coldRead{}, runtimeContractViolation("get session snapshot returned goal while the goals feature is disabled")
	}
	return coldRead{
		session: snapshot.Session, runs: snapshot.Runs, items: snapshot.Items, plan: snapshot.Plan,
		goal: snapshot.Goal, interrupts: snapshot.Interrupts,
	}, nil
}

func (r *Connection) subscribeSnapshot(ctx context.Context, input conversation.SubscribeRun) (conversation.SegmentStream, error) {
	streamCtx, release := context.WithCancel(ctx)
	stream, snapshot, err := r.subscribeRun(streamCtx, input)
	if err != nil {
		release()
		return conversation.SegmentStream{}, err
	}
	material, err := r.snapshotMaterial(input.SessionID, snapshot)
	if err != nil {
		release()
		return conversation.SegmentStream{}, err
	}
	for _, run := range material.runs {
		if run.SessionID != input.SessionID {
			release()
			return conversation.SegmentStream{}, runtimeContractViolation("subscribe run snapshot returned run %s from session %s for %s", run.ID, run.SessionID, input.SessionID)
		}
	}
	projected, err := projectSnapshot(material)
	if err != nil {
		release()
		return conversation.SegmentStream{}, runtimeContractViolation("subscribe run snapshot projection is invalid: %v", err)
	}
	stream.Snapshot = &projected
	events := stream.Events
	stream.Events = func(yield func(conversation.RunEvent, error) bool) {
		defer release()
		events(yield)
	}
	return stream, nil
}

func projectSnapshot(read coldRead) (conversation.SessionSnapshot, error) {
	snapshot := conversation.SessionSnapshot{
		Session:    projectSession(read.session),
		Transcript: make([]conversation.Block, 0, len(read.items)),
	}
	for _, value := range read.items {
		block, projectItemErr := projectItem(value)
		if projectItemErr != nil {
			return conversation.SessionSnapshot{}, projectItemErr
		}
		snapshot.Transcript = append(snapshot.Transcript, block)
	}
	orderedRuns := slices.Clone(read.runs)
	slices.SortFunc(orderedRuns, func(first, second protocol.RunRef) int {
		return cmp.Or(first.CreatedAt.Compare(second.CreatedAt), cmp.Compare(first.ID, second.ID))
	})
	snapshot.Runs = make([]conversation.Run, 0, len(orderedRuns))
	for _, value := range orderedRuns {
		run, projectRunErr := projectRun(value)
		if projectRunErr != nil {
			return conversation.SessionSnapshot{}, projectRunErr
		}
		snapshot.Runs = append(snapshot.Runs, run)
	}
	if read.plan != nil {
		plan, err := projectPlan(read.plan)
		if err != nil {
			return conversation.SessionSnapshot{}, err
		}
		snapshot.Plan = plan
	}
	if read.goal != nil {
		projected := cloneGoal(*read.goal)
		snapshot.Goal = &projected
	}
	if active, ok := snapshot.ActiveRun(); ok && active.Status == protocol.RunStatusWaiting {
		if len(read.interrupts) != 1 {
			return conversation.SessionSnapshot{}, fmt.Errorf("waiting run %s has %d pending interrupt sets", active.ID, len(read.interrupts))
		}
		set := read.interrupts[0]
		if set.SessionID != snapshot.Session.ID {
			return conversation.SessionSnapshot{}, fmt.Errorf("waiting run %s has a pending interrupt set for session %s", active.ID, set.SessionID)
		}
		if set.RootRunID != active.ID {
			return conversation.SessionSnapshot{}, fmt.Errorf("waiting run %s has a pending interrupt set for root %s", active.ID, set.RootRunID)
		}
		interrupts, err := projectInterrupts(set.Interrupts)
		if err != nil {
			return conversation.SessionSnapshot{}, err
		}
		snapshot.Interrupts = interrupts
	} else if len(read.interrupts) != 0 {
		return conversation.SessionSnapshot{}, fmt.Errorf("session %s has interrupts without a waiting root run", snapshot.Session.ID)
	}
	return snapshot, nil
}
