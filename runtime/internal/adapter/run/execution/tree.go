package execution

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	runinput "github.com/Tangerg/flame/runtime/internal/adapter/run/input"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"

	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
)

const interactionBarrierPauseReason = "runtime human-input tree barrier"

func (i *InteractionExecutor) CancelRunningSubtree(
	ctx context.Context,
	ref runs.ExecutorRef,
	memberID string,
	cancellation runs.CancellationID,
) error {
	if cancellation.IsZero() {
		return errors.New("execution: child cancellation identity is required")
	}
	session, err := i.session(ref)
	if err != nil {
		return err
	}
	processID, err := scoperun.ParseProcessID(memberID)
	if err != nil {
		return fmt.Errorf("execution: parse running Interaction member: %w", err)
	}
	root, err := session.claimSubtreeCancellation(ctx, processID)
	if err != nil {
		return err
	}
	defer session.releaseSubtreeCancellation()
	process, found := session.engine.Process(processID)
	if !found || process.ID().Root() != root.ID() {
		return runs.ErrExecutorNotLive
	}
	if err := process.Cancel(
		runExecutionContext(ctx, session.scope, session.start), cancellationControlPrefix+cancellation.String(),
	); err != nil {
		return fmt.Errorf("execution: cancel running Interaction member %s: %w", processID, err)
	}
	return nil
}

// A capture in progress decides first: once it publishes the barrier, the
// cancellation belongs to the waiting tree instead.
func (i *interactionSession) claimSubtreeCancellation(ctx context.Context, processID scoperun.ProcessID) (*scoperun.Handle[scoperun.Payload], error) {
	i.state.mu.Lock()
	for i.state.barrierCapture != nil {
		captured := i.state.barrierCapture
		i.state.mu.Unlock()
		select {
		case <-captured:
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		}
		i.state.mu.Lock()
	}
	defer i.state.mu.Unlock()
	root := i.state.process
	switch {
	case i.state.phase == interactionFinished || root == nil:
		return nil, runs.ErrExecutorNotLive
	case i.state.delegateChildren[processID] == nil || processID == root.ID():
		return nil, errors.New("execution: running subtree target is not a managed Delegate")
	case i.state.boundary == interactionBoundaryWaiting:
		return nil, runs.ErrExecutorParking
	case i.state.boundary != interactionBoundaryInactive:
		return nil, runs.ErrExecutorNotLive
	}
	i.state.cancellationsInFlight++
	return root, nil
}

func (i *interactionSession) releaseSubtreeCancellation() {
	i.state.mu.Lock()
	i.state.cancellationsInFlight--
	i.state.mu.Unlock()
	i.lifetime.wakeState()
}

// Proving an input wait first avoids capturing a half-step with no durable
// Interrupt to resume; retired child subtrees are excluded because the
// continuation never resumes them.
func (i *interactionSession) holdTreeAtBarrier(ctx context.Context, tree scoperun.Log, requestedPauses map[scoperun.ProcessID]struct{}) (bool, error) {
	parents := capturedParents(tree)
	captured, err := tree.Inspect()
	if err != nil {
		return false, err
	}
	allHeld := true
	for _, snapshot := range captured.Processes {
		if snapshot.Status.Terminal() || i.retiredSubtreeMember(snapshot.ID, parents) {
			continue
		}
		held, err := i.holdAtBarrier(ctx, tree, snapshot, requestedPauses)
		if err != nil {
			return false, err
		}
		allHeld = allHeld && held
	}
	return allHeld, nil
}

func (i *interactionSession) captureHumanInputBarrier(
	ctx context.Context,
) (scoperun.Log, []runs.MemberInterruption, bool, error) {
	root := i.state.processHandle()
	if root == nil {
		return scoperun.Log{}, nil, false, runs.ErrExecutorNotLive
	}
	inspection, readable := i.inspectInputWait(ctx)
	if !readable {
		return scoperun.Log{}, nil, false, nil
	}
	if !hasWaitingBarrier(inspection, root.ID()) {
		return scoperun.Log{}, nil, false, nil
	}
	requestedPauses := make(map[scoperun.ProcessID]struct{})
	ticker := time.NewTicker(i.statePollInterval)
	defer ticker.Stop()
	for {
		tree, err := i.engine.History(ctx, root.ID().Root())
		if err != nil {
			return scoperun.Log{}, nil, false, err
		}
		interruptions, err := i.pendingInterruptions(tree)
		if err != nil {
			return scoperun.Log{}, nil, false, err
		}
		if len(interruptions) == 0 {
			return scoperun.Log{}, nil, false, nil
		}
		held, err := i.holdTreeAtBarrier(ctx, tree, requestedPauses)
		if err != nil {
			return scoperun.Log{}, nil, false, err
		}
		if !held {
			select {
			case <-i.lifetime.stateWake:
			case <-ticker.C:
			case <-ctx.Done():
				return scoperun.Log{}, nil, false, ctx.Err()
			}
			continue
		}
		return tree, interruptions, true, nil
	}
}

// Scope records a pause without waiting for work under way: an in-flight step
// still commits and a starting child joins unpaused, so a cut taken first would
// publish less input than the tree raises.
func (i *interactionSession) holdAtBarrier(
	ctx context.Context,
	tree scoperun.Log,
	member scoperun.ProcessInspection,
	requestedPauses map[scoperun.ProcessID]struct{},
) (bool, error) {
	switch member.Status {
	case scoperun.StatusPaused:
		rest, err := pausedAtRest(tree, member.ID)
		if err != nil {
			return false, err
		}
		if rest {
			return !startsChild(member), nil
		}
		delete(requestedPauses, member.ID)
		process, found := i.engine.Process(member.ID)
		if !found {
			return false, nil
		}
		if err := process.Resume(ctx); err != nil && !errors.Is(err, scoperun.ErrProcessFinished) {
			return false, fmt.Errorf("resume Interaction member %s: %w", member.ID, err)
		}
		return false, nil
	case scoperun.StatusWaiting:
		if _, requested := requestedPauses[member.ID]; requested {
			return false, nil
		}
		process, found := i.engine.Process(member.ID)
		if !found {
			return false, nil
		}
		if err := process.Pause(ctx, interactionBarrierPauseReason); err != nil &&
			!errors.Is(err, scoperun.ErrProcessFinished) {
			return false, fmt.Errorf("pause Interaction member %s: %w", member.ID, err)
		}
		requestedPauses[member.ID] = struct{}{}
		return false, nil
	default:
		return false, nil
	}
}

func pausedAtRest(tree scoperun.Log, member scoperun.ProcessID) (bool, error) {
	pause := -1
	for index, entry := range tree.Entries {
		if control, ok := entry.Record.(scoperun.ControlRequested); ok && control.Process == member && control.Kind == scoperun.ControlPause {
			pause = index
		}
	}
	if pause < 0 {
		return true, nil
	}
	for _, entry := range tree.Entries[pause+1:] {
		switch record := entry.Record.(type) {
		case scoperun.Stepped:
			if record.Process == member {
				return true, nil
			}
		case scoperun.Checkpoint:
			return true, nil
		}
	}
	before, err := scoperun.Log{Root: tree.Root, Entries: tree.Entries[:pause]}.Inspect()
	if err != nil {
		return false, err
	}
	snapshot, found := before.Process(member)
	return found && snapshot.Status == scoperun.StatusWaiting, nil
}

func startsChild(member scoperun.ProcessInspection) bool {
	for _, pending := range member.PendingEffects {
		if _, spawn := pending.Request.Effect().ChildStart(); spawn && !pending.Outcome.Valid() {
			return true
		}
	}
	return false
}

func hasWaitingBarrier(inspection scoperun.TreeInspection, root scoperun.ProcessID) bool {
	member, found := inspection.Process(root)
	return found && member.Status == scoperun.StatusWaiting && externallyAddressedWait(inspection)
}

func (i *interactionSession) inspectInputWait(ctx context.Context) (scoperun.TreeInspection, bool) {
	history, err := i.committedTree(ctx, i.rootID)
	if err != nil || !hasWaitDeclaration(history) {
		return scoperun.TreeInspection{}, false
	}
	inspection, err := i.logCache.inspect(history)
	return inspection, err == nil
}

// Absence of a declaration rules out an input wait without replaying every
// payload. Presence only triggers Scope's authoritative inspection; an old
// declaration never proves that a wait is still live.
func hasWaitDeclaration(history scoperun.Log) bool {
	for _, entry := range history.Entries {
		switch record := entry.Record.(type) {
		case scoperun.Checkpoint:
			return true
		case scoperun.Stepped:
			if len(record.Transition.Waits()) > 0 {
				return true
			}
		}
	}
	return false
}

// One inspection: per-member reads would compose an answer from different moments.
func (i *interactionSession) inspectTree(ctx context.Context) (scoperun.TreeInspection, bool) {
	root := i.state.processHandle()
	if root == nil {
		return scoperun.TreeInspection{}, false
	}
	inspection, err := i.inspectCommittedTree(ctx)
	if err != nil {
		return scoperun.TreeInspection{}, false
	}
	return inspection, true
}

func externallyAddressedWait(inspection scoperun.TreeInspection) bool {
	for _, member := range inspection.Processes {
		if member.Status == scoperun.StatusWaiting && len(member.Waits) > 0 {
			return true
		}
	}
	return false
}

func (i *interactionSession) pendingInputs(history scoperun.Log) ([]interaction.PendingToolInput, error) {
	inspection, err := history.Inspect()
	if err != nil {
		return nil, err
	}
	var inputs []interaction.PendingToolInput
	for _, program := range i.state.deployments.interactions {
		inputs = append(inputs, interaction.PendingToolInputs(program.program, inspection)...)
	}
	return inputs, nil
}

// A WaitKey is process-local, so sibling Interactions at the same step could
// otherwise answer one another's question.
type inputAddress struct {
	process scoperun.ProcessID
	wait    scoperun.WaitKey
}

func inputRequestID(process scoperun.ProcessID, wait scoperun.WaitKey) string {
	return "input:" + scoperun.ComputeDigest([]byte(process.String()+"\x00"+wait.String())).String()
}

func (i *interactionSession) pendingInterruptions(
	tree scoperun.Log,
) ([]runs.MemberInterruption, error) {
	if err := tree.Validate(); err != nil {
		return nil, errors.New("execution: inspect pending inputs from invalid Interaction tree")
	}
	pendingInputs, err := i.pendingInputs(tree)
	if err != nil {
		return nil, fmt.Errorf("inspect Interaction tree inputs: %w", err)
	}
	interruptions := make([]runs.MemberInterruption, 0, len(pendingInputs))
	for _, pending := range pendingInputs {
		processID := pending.ProcessID()
		member, bound := i.toolCallMember(processID)
		if !bound {
			continue
		}
		prompt, err := runinput.DecodePrompt(pending.Prompt().JSON())
		if err != nil {
			return nil, fmt.Errorf("decode Interaction member %s prompt: %w", member.MemberID, err)
		}
		interruptions = append(interruptions, runs.MemberInterruption{
			MemberID: member.MemberID, RequestID: inputRequestID(processID, pending.WaitKey()), Interrupt: prompt,
		})
	}
	slices.SortFunc(interruptions, func(left, right runs.MemberInterruption) int {
		if order := strings.Compare(left.MemberID, right.MemberID); order != 0 {
			return order
		}
		return strings.Compare(left.RequestID, right.RequestID)
	})
	return interruptions, nil
}

func (i *interactionSession) unknownEffectIDs(ctx context.Context) ([]scoperun.EffectID, error) {
	root := i.state.processHandle()
	if root == nil {
		return nil, runs.ErrExecutorNotLive
	}
	history, err := i.committedTree(ctx, root.ID().Root())
	if err != nil || !hasUnknownSettlement(history) {
		return nil, err
	}
	inspection, err := i.logCache.inspect(history)
	if err != nil {
		return nil, err
	}
	ids := make([]scoperun.EffectID, 0)
	for _, member := range inspection.Processes {
		if member.Status.Terminal() {
			continue
		}
		for _, pending := range member.PendingEffects {
			if pending.Outcome.Status() == scoperun.SettlementStatusUnknown {
				ids = append(ids, pending.Request.ID())
			}
		}
	}
	slices.SortFunc(ids, scoperun.EffectID.Compare)
	ids = slices.Compact(ids)
	return ids, nil
}

func hasUnknownSettlement(history scoperun.Log) bool {
	for _, entry := range history.Entries {
		switch record := entry.Record.(type) {
		case scoperun.Checkpoint:
			return true
		case scoperun.EffectSettled:
			if record.Outcome.Status() == scoperun.SettlementStatusUnknown {
				return true
			}
		}
	}
	return false
}

func (i *interactionSession) stagedTree(ctx context.Context) (scoperun.Log, error) {
	i.state.mu.Lock()
	checkpoint := i.state.waitingCheckpoint.Clone()
	i.state.mu.Unlock()
	state, err := decodeExecutorCheckpoint(checkpoint)
	if err != nil {
		return scoperun.Log{}, err
	}
	return i.committedTree(ctx, state.handoff.rootID)
}

func (i *interactionSession) pausedProcessIDs(ctx context.Context) ([]scoperun.ProcessID, error) {
	tree, err := i.stagedTree(ctx)
	if err != nil {
		return nil, err
	}
	parents := capturedParents(tree)
	inspection, err := tree.Inspect()
	if err != nil {
		return nil, err
	}
	paused := make([]scoperun.ProcessID, 0)
	for _, snapshot := range inspection.Processes {
		if snapshot.Status != scoperun.StatusPaused {
			continue
		}
		processID := snapshot.ID
		if i.retiredSubtreeMember(processID, parents) {
			continue
		}
		paused = append(paused, processID)
	}
	return paused, nil
}

func (i *interactionSession) retiredSubtreeMember(
	processID scoperun.ProcessID,
	parents map[scoperun.ProcessID]scoperun.ProcessID,
) bool {
	for id := processID; id.Valid(); id = parents[id] {
		if i.state.projectedDelegate(id) {
			return true
		}
	}
	return false
}

func (i *interactionSession) resumePausedProcesses(
	ctx context.Context,
	processIDs []scoperun.ProcessID,
) error {
	// Every member is proved paused from one inspection before any of them is
	// resumed, so a member that already left the boundary cannot be discovered
	// half way through resuming its siblings.
	inspection, readable := i.inspectTree(ctx)
	if !readable {
		return runs.ErrExecutorNotLive
	}
	processes := make([]*scoperun.Handle[scoperun.Payload], len(processIDs))
	for index, processID := range processIDs {
		member, inspected := inspection.Process(processID)
		if !inspected || member.Status != scoperun.StatusPaused {
			return fmt.Errorf("execution: Interaction member %s left its paused boundary (status=%s, inspected=%t, root=%s)", processID, member.Status, inspected, inspection.Root)
		}
		process, found := i.engine.Process(processID)
		if !found {
			return fmt.Errorf("execution: paused Interaction member %s is unavailable", processID)
		}
		processes[index] = process
	}
	for index, process := range processes {
		if err := process.Resume(ctx); err != nil {
			return fmt.Errorf("execution: resume Interaction member %s: %w", processIDs[index], err)
		}
	}
	return nil
}
