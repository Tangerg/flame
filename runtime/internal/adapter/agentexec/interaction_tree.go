package agentexec

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/adapter/agentexec/interactioninput"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
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
	processID, err := agent.ParseProcessID(memberID)
	if err != nil {
		return fmt.Errorf("agentexec: parse running Interaction member: %w", err)
	}
	session.state.mu.Lock()
	root := session.state.process
	managed := session.state.delegateChildren[processID]
	available := !session.state.finished && session.state.boundary == interactionBoundaryInactive
	session.state.mu.Unlock()
	if !available || root == nil {
		return runs.ErrExecutorNotLive
	}
	if managed == nil || processID == root.ID() {
		return errors.New("agentexec: running subtree target is not a managed Delegate")
	}
	process, found := session.engine.Process(processID)
	if !found || process.Relation().RootID() != root.ID() {
		return runs.ErrExecutorNotLive
	}
	controlCtx, cancel := session.lifetime.bind(ctx)
	defer cancel()
	if err := process.RequestCancellation(
		runExecutionContext(controlCtx, session.scope, session.start), reason,
	); err != nil {
		return fmt.Errorf("agentexec: cancel running Interaction member %s: %w", processID, err)
	}
	session.cancelSubtreeDispatches(processID)
	return nil
}

// captureHumanInputBarrier first proves that the tree contains an externally
// addressable input wait, then pauses other Running branches at their next safe
// boundary. A parent waiting only on children is not itself a product barrier;
// pausing its still-opening child before this proof would capture a half-step
// with no durable Interrupt to resume.
func (i *interactionSession) captureHumanInputBarrier(
	ctx context.Context,
) (agent.TreeSnapshot, []runs.MemberInterruption, bool, error) {
	root := i.state.processHandle()
	if root == nil {
		return agent.TreeSnapshot{}, nil, false, runs.ErrExecutorNotLive
	}
	// CaptureTree freezes dispatch while awaiting in-flight effects. An internal
	// Delegate join can still need another child to start before an effect returns.
	inspection, readable := i.inspectTree(ctx)
	if !readable {
		return agent.TreeSnapshot{}, nil, false, nil
	}
	// A root that is not waiting is not yet a barrier rather than a fault: the
	// reconciler asks again. This is the only place that decides it, so the
	// caller no longer pre-reads a status that could move before the cut.
	rootMember, found := inspection.Process(root.ID())
	if !found || rootMember.Snapshot.Status() != agent.StatusWaiting {
		return agent.TreeSnapshot{}, nil, false, nil
	}
	if !externallyAddressedWait(inspection) {
		return agent.TreeSnapshot{}, nil, false, nil
	}
	for {
		tree, err := i.engine.CaptureTree(ctx, root.Relation().RootID())
		if err != nil {
			return agent.TreeSnapshot{}, nil, false, err
		}
		interruptions, err := i.pendingInterruptions(tree)
		if err != nil {
			return agent.TreeSnapshot{}, nil, false, err
		}
		if len(interruptions) == 0 {
			return agent.TreeSnapshot{}, nil, false, nil
		}
		var paused bool
		for _, snapshot := range tree.ProcessSnapshots() {
			if snapshot.Status() != agent.StatusRunning {
				continue
			}
			process, found := i.engine.Process(snapshot.ProcessID())
			if !found {
				paused = true
				continue
			}
			// The cut says this member was running; by now it may have finished or
			// reached a wait, and only a running member can be armed with a pause.
			// Either way the tree moved under the cut, so the barrier asks again
			// rather than treating its own stale reading as a fault.
			if err := process.Pause(ctx, interactionBarrierPauseReason); err != nil &&
				!errors.Is(err, agent.ErrProcessFinished) &&
				!errors.Is(err, agent.ErrProcessNotRunning) {
				return agent.TreeSnapshot{}, nil, false, fmt.Errorf("pause Interaction member %s: %w", snapshot.ProcessID(), err)
			}
			paused = true
		}
		if paused {
			continue
		}
		return tree, interruptions, true, nil
	}
}

// inspectTree reads every member of this Interaction's tree in one pass. A
// Process no longer answers for itself: Status, unknown Effects and the wait it
// is addressed by are all facts of one inspection, so asking per member would
// compose an answer out of readings taken at different moments. Every way the
// reading can fail — a closed Engine, a tree that is gone, a caller that stopped
// waiting — reports that no reading is available, never that the Run is broken,
// so this answers availability instead of handing callers an error to classify.
func (i *interactionSession) inspectTree(ctx context.Context) (agent.TreeInspection, bool) {
	root := i.state.processHandle()
	if root == nil {
		return agent.TreeInspection{}, false
	}
	inspection, err := i.engine.InspectTree(ctx, root.Relation().RootID())
	if err != nil {
		return agent.TreeInspection{}, false
	}
	return inspection, true
}

// externallyAddressedWait reports whether any member holds a wait only the host
// can answer. The barrier needs that proof before it freezes dispatch, and the
// same inspection that proved the root is waiting already carries it.
func externallyAddressedWait(inspection agent.TreeInspection) bool {
	for _, member := range inspection.Processes {
		if member.Snapshot.Status() != agent.StatusWaiting {
			continue
		}
		if kind, addressed := member.Snapshot.WaitKind(); addressed && kind == agent.WaitKindExternal {
			return true
		}
	}
	return false
}

func (i *interactionSession) pendingInterruptions(
	tree agent.TreeSnapshot,
) ([]runs.MemberInterruption, error) {
	if !tree.Valid() {
		return nil, errors.New("agentexec: inspect pending inputs from invalid Interaction tree")
	}
	pendingInputs, err := interaction.PendingToolInputs(tree)
	if err != nil {
		return nil, fmt.Errorf("inspect Interaction tree inputs: %w", err)
	}
	relations := make(map[agent.ProcessID]agent.ProcessRelation, len(tree.ProcessSnapshots()))
	for _, snapshot := range tree.ProcessSnapshots() {
		relations[snapshot.ProcessID()] = snapshot.Relation()
	}
	interruptions := make([]runs.MemberInterruption, 0, len(pendingInputs))
	for _, pending := range pendingInputs {
		processID := pending.ProcessID()
		member, bound := i.toolCallMember(relations[processID])
		if !bound {
			// The cut can outlive a member the product retired, and a Tool wait
			// belongs to the member that called it. With that member gone the wait
			// died with it: nobody can be shown it and nobody can answer it.
			continue
		}
		prompt, err := interactioninput.DecodePrompt(pending.Prompt())
		if err != nil {
			return nil, fmt.Errorf("decode Interaction member %s prompt: %w", member.MemberID, err)
		}
		interruptions = append(interruptions, runs.MemberInterruption{
			MemberID: member.MemberID, RequestID: pending.WaitID().String(), Interrupt: prompt,
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

func (i *interactionSession) managedProcesses() ([]*agent.Process, error) {
	i.state.mu.Lock()
	root := i.state.process
	children := make([]agent.ProcessID, 0, len(i.state.delegateChildren))
	for processID := range i.state.delegateChildren {
		children = append(children, processID)
	}
	i.state.mu.Unlock()
	if root == nil {
		return nil, runs.ErrExecutorNotLive
	}
	slices.SortFunc(children, func(left, right agent.ProcessID) int {
		return strings.Compare(left.String(), right.String())
	})
	processes := make([]*agent.Process, 0, len(children)+1)
	processes = append(processes, root)
	for _, processID := range children {
		process, found := i.engine.Process(processID)
		if found {
			processes = append(processes, process)
		}
	}
	return processes, nil
}

func (i *interactionSession) unknownEffectIDs(ctx context.Context) ([]agent.EffectID, bool) {
	inspection, readable := i.inspectTree(ctx)
	if !readable {
		return nil, false
	}
	ids := make([]agent.EffectID, 0)
	for _, member := range inspection.Processes {
		// Terminal snapshots retain interrupted Effects as evidence, not work
		// that can be reconciled or resumed.
		if member.Snapshot.Status().Terminal() {
			continue
		}
		processID := member.Snapshot.ProcessID()
		i.state.mu.Lock()
		canceled := i.state.rootCancellationRequested || i.inCanceledSubtreeLocked(processID)
		i.state.mu.Unlock()
		if canceled || i.allowance.denial(processID) != interactionAllowanceOpen || i.modelFailures.has(processID) {
			continue
		}
		ids = append(ids, member.Snapshot.UnknownEffectIDs()...)
	}
	slices.SortFunc(ids, func(left, right agent.EffectID) int {
		return strings.Compare(left.String(), right.String())
	})
	ids = slices.Compact(ids)
	return ids, true
}

// stagedTree returns the frozen cut this Interaction published its waiting
// boundary from. Answers and paused members are both read from it rather than
// from a fresh capture, so they describe the same moment the Interrupts did.
func (i *interactionSession) stagedTree() (agent.TreeSnapshot, error) {
	i.state.mu.Lock()
	checkpoint := i.state.waitingCheckpoint.Clone()
	i.state.mu.Unlock()
	state, err := decodeInteractionCheckpointPayload(checkpoint.Payload)
	if err != nil {
		return agent.TreeSnapshot{}, err
	}
	return state.tree, nil
}

// stagedPendingInputs indexes the Tool waits of the staged cut by their WaitID.
// A member may hold several concurrent waits, so the wait — not the member that
// owns it — is what an answer addresses.
func (i *interactionSession) stagedPendingInputs() (map[agent.WaitID]interaction.PendingToolInput, error) {
	tree, err := i.stagedTree()
	if err != nil {
		return nil, err
	}
	pendingInputs, err := interaction.PendingToolInputs(tree)
	if err != nil {
		return nil, fmt.Errorf("inspect staged Interaction inputs: %w", err)
	}
	byWait := make(map[agent.WaitID]interaction.PendingToolInput, len(pendingInputs))
	for _, pending := range pendingInputs {
		byWait[pending.WaitID()] = pending
	}
	return byWait, nil
}

func (i *interactionSession) pausedProcessIDs() ([]agent.ProcessID, error) {
	tree, err := i.stagedTree()
	if err != nil {
		return nil, err
	}
	paused := make([]agent.ProcessID, 0)
	for _, snapshot := range tree.ProcessSnapshots() {
		if snapshot.Status() == agent.StatusPaused {
			paused = append(paused, snapshot.ProcessID())
		}
	}
	return paused, nil
}

func (i *interactionSession) resumePausedProcesses(
	ctx context.Context,
	processIDs []agent.ProcessID,
) error {
	// Every member is proved paused from one inspection before any of them is
	// resumed, so a member that already left the boundary cannot be discovered
	// half way through resuming its siblings.
	inspection, readable := i.inspectTree(ctx)
	if !readable {
		return runs.ErrExecutorNotLive
	}
	processes := make([]*agent.Process, len(processIDs))
	for index, processID := range processIDs {
		member, inspected := inspection.Process(processID)
		if !inspected || member.Snapshot.Status() != agent.StatusPaused {
			return fmt.Errorf("agentexec: Interaction member %s left its paused boundary", processID)
		}
		process, found := i.engine.Process(processID)
		if !found {
			return fmt.Errorf("agentexec: paused Interaction member %s is unavailable", processID)
		}
		processes[index] = process
	}
	for index, process := range processes {
		if err := process.Resume(ctx); err != nil {
			return fmt.Errorf("agentexec: resume Interaction member %s: %w", processIDs[index], err)
		}
	}
	return nil
}
