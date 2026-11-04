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

// CancelRunningSubtree submits a product-owned cancellation to one exact live
// managed Delegate. Agent Framework owns propagation to descendants and the resulting
// child completion; Runtime observes those facts through the normal tree pump.
func (i *InteractionExecutor) CancelRunningSubtree(
	ctx context.Context,
	ref runs.ExecutorRef,
	memberID string,
	reason string,
) error {
	session, err := i.session(ref)
	if err != nil {
		return err
	}
	processID, err := scoperun.ParseProcessID(memberID)
	if err != nil {
		return fmt.Errorf("execution: parse running Interaction member: %w", err)
	}
	session.state.mu.Lock()
	root := session.state.process
	managed := session.state.delegateChildren[processID]
	available := session.state.phase != interactionFinished && session.state.boundary == interactionBoundaryInactive
	session.state.mu.Unlock()
	if !available || root == nil {
		return runs.ErrExecutorNotLive
	}
	if managed == nil || processID == root.ID() {
		return errors.New("execution: running subtree target is not a managed Delegate")
	}
	process, found := session.engine.Process(processID)
	if !found || process.ID().Root() != root.ID() {
		return runs.ErrExecutorNotLive
	}
	controlCtx, cancel := session.lifetime.bind(ctx)
	defer cancel()
	if err := process.Cancel(
		runExecutionContext(controlCtx, session.scope, session.start), scopeCancellationReason(reason),
	); err != nil {
		return fmt.Errorf("execution: cancel running Interaction member %s: %w", processID, err)
	}
	return nil
}

// captureHumanInputBarrier first proves that the tree contains an externally
// addressable input wait, then pauses every surviving nonterminal member at its
// next safe boundary — running branches, external input waiters, and parents
// waiting on children alike — so no answer recipient can advance while the
// complete answer set is delivered. Proving the wait first avoids capturing a
// half-step with no durable Interrupt to resume; retired child subtrees are
// excluded because the continuation never resumes them.
func (i *interactionSession) captureHumanInputBarrier(
	ctx context.Context,
) (scoperun.Log, []runs.MemberInterruption, bool, error) {
	root := i.state.processHandle()
	if root == nil {
		return scoperun.Log{}, nil, false, runs.ErrExecutorNotLive
	}
	// Prove a product input wait before asking Scope to drain in-flight work.
	inspection, readable := i.inspectInputWait(ctx)
	if !readable {
		return scoperun.Log{}, nil, false, nil
	}
	// A root that is not waiting is not yet a barrier rather than a fault: the
	// reconciler asks again. This is the only place that decides it, so the
	// caller no longer pre-reads a status that could move before the cut.
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
		parents := capturedParents(tree)
		captured, err := tree.Inspect()
		if err != nil {
			return scoperun.Log{}, nil, false, err
		}
		var unsettled bool
		for _, snapshot := range captured.Processes {
			status := snapshot.Status
			processID := snapshot.ID
			if status.Terminal() || i.retiredSubtreeMember(processID, parents) {
				continue
			}
			if status == scoperun.StatusPaused {
				continue
			}
			// A surviving nonterminal member the continuation will release is not yet
			// at the barrier. Pause acknowledges intent before its committed snapshot
			// advances, so the cut is safe only once a later capture observes every
			// intended member Paused.
			unsettled = true
			if _, requested := requestedPauses[processID]; requested {
				continue
			}
			process, found := i.engine.Process(processID)
			if !found {
				return scoperun.Log{}, nil, false, nil
			}
			if err := process.Pause(ctx, interactionBarrierPauseReason); err != nil &&
				!errors.Is(err, scoperun.ErrProcessFinished) {
				return scoperun.Log{}, nil, false, fmt.Errorf("pause Interaction member %s: %w", processID, err)
			}
			requestedPauses[processID] = struct{}{}
		}
		if unsettled {
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

func hasWaitingBarrier(inspection scoperun.TreeInspection, root scoperun.ProcessID) bool {
	member, found := inspection.Process(root)
	return found && member.Status == scoperun.StatusWaiting && externallyAddressedWait(inspection)
}

func (i *interactionSession) inspectInputWait(ctx context.Context) (scoperun.TreeInspection, bool) {
	history, err := i.committedTree(ctx, i.processRootID())
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

// inspectTree reads every member of this Interaction's tree in one pass. A
// Process no longer answers for itself: Status, unknown Effects and the wait it
// is addressed by are all facts of one inspection, so asking per member would
// compose an answer out of readings taken at different moments. Every way the
// reading can fail — a closed Engine, a tree that is gone, a caller that stopped
// waiting — reports that no reading is available, never that the Run is broken,
// so this answers availability instead of handing callers an error to classify.
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

// externallyAddressedWait reports whether any member holds a wait only the host
// can answer. The barrier needs that proof before it freezes dispatch, and the
// same inspection that proved the root is waiting already carries it.
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
		inputs = append(inputs, interaction.PendingToolInputs(program, inspection)...)
	}
	return inputs, nil
}

// WaitKey is process-local. Product requests retain both coordinates so sibling
// Interactions at the same step can never answer one another's question.
type inputAddress struct {
	process scoperun.ProcessID
	wait    scoperun.WaitKey
}

func inputRequestID(process scoperun.ProcessID, wait scoperun.WaitKey) string {
	return "input:" + scoperun.ComputeDigest([]byte(process.String()+"\x00"+wait.String())).String()
}

func pendingAddress(input interaction.PendingToolInput) inputAddress {
	return inputAddress{process: input.ProcessID(), wait: input.WaitKey()}
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
			// The cut can outlive a member the product retired, and a Tool wait
			// belongs to the member that called it. With that member gone the wait
			// died with it: nobody can be shown it and nobody can answer it.
			continue
		}
		prompt, err := runinput.DecodePrompt(pending.Prompt())
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
		// Terminal snapshots retain interrupted Effects as evidence, not work
		// that can be reconciled or resumed.
		if member.Status.Terminal() {
			continue
		}
		for _, pending := range member.PendingEffects {
			if pending.Outcome.Status() == scoperun.SettlementStatusUnknown {
				ids = append(ids, pending.Request.ID())
			}
		}
	}
	slices.SortFunc(ids, func(left, right scoperun.EffectID) int {
		return strings.Compare(left.String(), right.String())
	})
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

// stagedTree reads the latest acknowledged head bound to the retained context.
// Signal admission may advance this head while every member stays paused.
func (i *interactionSession) stagedTree() (scoperun.Log, error) {
	i.state.mu.Lock()
	checkpoint := i.state.waitingCheckpoint.Clone()
	i.state.mu.Unlock()
	state, err := decodeExecutorCheckpoint(checkpoint)
	if err != nil {
		return scoperun.Log{}, err
	}
	return i.committedTree(i.lifetime.execution, state.handoff.rootID)
}

// stagedPendingInputs indexes the Tool waits of the staged cut by their WaitID.
// A member may hold several concurrent waits, so the wait — not the member that
// owns it — is what an answer addresses.
func (i *interactionSession) stagedPendingInputs() (map[inputAddress]interaction.PendingToolInput, error) {
	tree, err := i.stagedTree()
	if err != nil {
		return nil, err
	}
	pendingInputs, err := i.pendingInputs(tree)
	if err != nil {
		return nil, fmt.Errorf("inspect staged Interaction inputs: %w", err)
	}
	byWait := make(map[inputAddress]interaction.PendingToolInput, len(pendingInputs))
	for _, pending := range pendingInputs {
		byWait[pendingAddress(pending)] = pending
	}
	return byWait, nil
}

func (i *interactionSession) pausedProcessIDs() ([]scoperun.ProcessID, error) {
	tree, err := i.stagedTree()
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
		// A committed subtree cancellation can still be draining in this cut.
		// Its product owner has retired the member and never resumes its descendants.
		if i.retiredSubtreeMember(processID, parents) {
			continue
		}
		paused = append(paused, processID)
	}
	return paused, nil
}

// retiredSubtreeMember reports whether processID or one of its captured ancestors
// is a managed Delegate child whose product terminal was already projected. The
// continuation never resumes such a draining-cancellation subtree, so the human-
// input barrier does not pause it and release does not revive it.
func (i *interactionSession) retiredSubtreeMember(
	processID scoperun.ProcessID,
	parents map[scoperun.ProcessID]scoperun.ProcessID,
) bool {
	for id := processID; id.Valid(); id = parents[id] {
		i.state.mu.Lock()
		managed := i.state.delegateChildren[id]
		i.state.mu.Unlock()
		if managed == nil {
			continue
		}
		managed.mu.Lock()
		projected := managed.segmentProjected
		managed.mu.Unlock()
		if projected {
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
