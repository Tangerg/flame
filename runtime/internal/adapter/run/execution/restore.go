package execution

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"

	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
)

func hasUnknownEffects(process scoperun.ProcessInspection) bool {
	for _, pending := range process.PendingEffects {
		if pending.Outcome.Status() == scoperun.SettlementStatusUnknown {
			return true
		}
	}
	return false
}

func (i *interactionSession) validateContinuationTree(ctx context.Context, continuation runs.ExecutionContinuation, checkpoint interactionCheckpointState, accepted bool) error {
	// A retained Unknown settlement is a restorable fact to the Engine; refusing
	// to resume on top of one is this Runtime's policy, so it stays here.
	inspection, err := checkpoint.tree.Inspect()
	if err != nil {
		return err
	}
	snapshots := make(map[scoperun.ProcessID]scoperun.ProcessInspection)
	for _, snapshot := range inspection.Processes {
		snapshots[snapshot.ID] = snapshot
		if hasUnknownEffects(snapshot) {
			return fmt.Errorf("%w: waiting tree contains unresolved effects", runs.ErrExecutorStateLost)
		}
	}
	// Scope's pure restore validation may wrap cancellation as an invalid
	// snapshot. An interrupted check proves nothing about checkpoint validity.
	if err := validateRecovery(ctx, i.deployment, checkpoint.tree); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if errors.Is(err, scoperun.ErrInvalidLog) {
			return fmt.Errorf("%w: %w", runs.ErrExecutorStateLost, err)
		}
		return err
	}
	members, err := i.restoredExecutionMembers(continuation, snapshots, checkpoint.tree.Root)
	if err != nil {
		return fmt.Errorf("%w: waiting members: %w", runs.ErrExecutorStateLost, err)
	}
	_, children, err := i.restoreDelegateCalls(checkpoint.tree, snapshots, members)
	if err != nil {
		return fmt.Errorf("%w: waiting Delegate bindings: %w", runs.ErrExecutorStateLost, err)
	}
	if accepted {
		_, _, err := acceptedReleaseBoundary(checkpoint.handoff, inspection, acceptedRetiredProcesses(checkpoint.handoff, checkpoint.tree, children))
		return err
	}
	return nil
}

func (i *interactionSession) restoreContinuationProjection(
	continuation runs.ExecutionContinuation,
	checkpoint interactionCheckpointState,
	boundary interactionBoundary,
) error {
	if boundary != interactionBoundaryWaiting && boundary != interactionBoundaryAccepted {
		return errors.New("invalid restored Interaction boundary")
	}
	inspection, err := checkpoint.tree.Inspect()
	if err != nil {
		return err
	}
	snapshots := make(map[scoperun.ProcessID]scoperun.ProcessInspection, len(inspection.Processes))
	for _, snapshot := range inspection.Processes {
		snapshots[snapshot.ID] = snapshot
	}
	members, err := i.restoredExecutionMembers(continuation, snapshots, checkpoint.tree.Root)
	if err != nil {
		return err
	}
	delegateCalls, delegateChildren, err := i.restoreDelegateCalls(checkpoint.tree, snapshots, members)
	if err != nil {
		return fmt.Errorf("%w: restore Delegate bindings: %w", runs.ErrExecutorStateLost, err)
	}
	i.material.restoreCalibrations(checkpoint.contextByProcess)
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	if i.state.phase != interactionPending || i.state.process != nil {
		return runs.ErrExecutionClaimed
	}
	i.state.restoreTree = checkpoint.tree
	if boundary == interactionBoundaryWaiting {
		i.state.phase = interactionBegun
	}
	i.state.boundary = boundary
	i.state.dispatchReady = make(chan struct{})
	i.state.waitingCheckpoint = continuation.Checkpoint.Clone()
	i.state.delegateCalls = delegateCalls
	i.state.delegateChildren = delegateChildren
	i.state.pendingSteers = checkpoint.pendingSteers
	i.state.pendingResumeInputs = checkpoint.pendingResumeInputs
	i.state.toolMetadata = checkpoint.toolMetadata
	return nil
}

func (i *interactionSession) recoverTree() error {
	process, err := i.engine.Recover(runExecutionContext(i.lifetime.execution, i.scope, i.start), i.deployment, i.rootID)
	if err != nil {
		return fmt.Errorf("execution: recover acknowledged Interaction tree: %w", err)
	}
	i.state.mu.Lock()
	i.state.process = process
	i.state.restoreTree = scoperun.Log{}
	i.state.mu.Unlock()
	return nil
}

func (i *interactionSession) recoverInputHandoff(ctx context.Context) error {
	if err := i.recoverTree(); err != nil {
		return err
	}
	unknown, err := i.unknownEffectIDs(ctx)
	if err != nil {
		return fmt.Errorf("execution: inspect restored Interaction tree: %w", err)
	}
	if len(unknown) != 0 {
		return fmt.Errorf("%w: restored Interaction has unresolved effects", runs.ErrExecutorStateLost)
	}
	return nil
}

func (i *interactionSession) restoredExecutionMembers(
	continuation runs.ExecutionContinuation,
	snapshots map[scoperun.ProcessID]scoperun.ProcessInspection,
	rootID scoperun.ProcessID,
) (map[scoperun.ProcessID]runs.ExecutionMember, error) {
	i.state.mu.Lock()
	deployments := i.state.deployments
	i.state.mu.Unlock()
	if deployments == nil {
		return nil, errors.New("execution: Interaction deployments are unavailable")
	}
	members := make(map[scoperun.ProcessID]runs.ExecutionMember, len(continuation.Members))
	for _, member := range continuation.Members {
		processID, err := scoperun.ParseProcessID(member.MemberID)
		if err != nil {
			return nil, fmt.Errorf("waiting member %q: %w", member.MemberID, err)
		}
		snapshot, found := snapshots[processID]
		if !found || snapshot.Status.Terminal() {
			return nil, fmt.Errorf("waiting member %s has no active Process", processID)
		}
		members[processID] = member
	}
	rootMember, found := members[rootID]
	if !found || rootMember.RunID != continuation.RootRunID || !rootMember.ParentRunID.IsZero() {
		return nil, errors.New("restored root Process differs from the product root member")
	}
	if err := validateRestoredLineage(deployments, members, snapshots, rootID); err != nil {
		return nil, err
	}
	for processID, snapshot := range snapshots {
		if snapshot.Status.Terminal() || deployments.interactionChild(snapshot.Program) {
			continue
		}
		if _, survives := members[processID]; !survives {
			return nil, fmt.Errorf("active Process %s has no surviving product member", processID)
		}
	}
	return members, nil
}

func validateRestoredLineage(deployments *interactionDeploymentSet, members map[scoperun.ProcessID]runs.ExecutionMember, snapshots map[scoperun.ProcessID]scoperun.ProcessInspection, rootID scoperun.ProcessID) error {
	for processID, member := range members {
		relation := snapshots[processID].ID
		if processID == rootID {
			if relation.Depth() != 0 {
				return errors.New("restored product root has a child Process relation")
			}
			continue
		}
		parentID, child := relation.Parent()
		for child && deployments.interactionChild(snapshots[parentID].Program) {
			parentID, child = parentID.Parent()
		}
		parentMember, parentSurvives := members[parentID]
		if !child || !parentSurvives || parentMember.RunID != member.ParentRunID {
			return fmt.Errorf("restored member %s differs from product lineage", processID)
		}
	}
	return nil
}

func (i *interactionSession) restoreDelegateCalls(
	history scoperun.Log,
	snapshots map[scoperun.ProcessID]scoperun.ProcessInspection,
	members map[scoperun.ProcessID]runs.ExecutionMember,
) (map[delegateCallIdentity]*managedDelegateCall, map[scoperun.ProcessID]*managedDelegateCall, error) {
	deployments := i.state.deployments
	if deployments == nil {
		return nil, nil, errors.New("execution: Interaction programs are unavailable")
	}
	calls := make(map[delegateCallIdentity]*managedDelegateCall)
	children := make(map[scoperun.ProcessID]*managedDelegateCall)
	for _, program := range deployments.interactions {
		attributions, err := program.history.DelegateAttributions(history)
		if err != nil {
			return nil, nil, err
		}
		for _, child := range attributions {
			parentID, _ := child.ProcessID().Parent()
			parentSnapshot, parentExists := snapshots[parentID]
			if !parentExists {
				continue
			}
			if _, childExists := snapshots[child.ProcessID()]; !childExists {
				if _, survives := members[child.ProcessID()]; survives {
					return nil, nil, errors.New("execution: surviving child was reaped")
				}
				continue
			}
			managed, err := restoreManagedDelegateCall(deployments, snapshots, members, parentID, parentSnapshot, child)
			if err != nil {
				return nil, nil, err
			}
			calls[managed.identity], children[child.ProcessID()] = managed, managed
		}
	}
	for processID := range members {
		if processID.Depth() == 0 {
			continue
		}
		if children[processID] == nil {
			return nil, nil, fmt.Errorf("surviving child %s has no Delegate attribution", processID)
		}
	}
	return calls, children, nil
}

func nearestProductMember(process scoperun.ProcessID, members map[scoperun.ProcessID]runs.ExecutionMember) (runs.ExecutionMember, bool) {
	for process.Valid() {
		if member, found := members[process]; found {
			return member, true
		}
		parent, child := process.Parent()
		if !child {
			break
		}
		process = parent
	}
	return runs.ExecutionMember{}, false
}

// delegateChildSnapshot is the child Process the attribution names, keyed
// under its parent exactly as the interaction state recorded it.
func delegateChildSnapshot(snapshots map[scoperun.ProcessID]scoperun.ProcessInspection, parentID scoperun.ProcessID, child interaction.DelegateAttribution) (scoperun.ProcessInspection, error) {
	childSnapshot, exists := snapshots[child.ProcessID()]
	if !exists {
		return scoperun.ProcessInspection{}, fmt.Errorf("delegate child %s is absent from the tree", child.ProcessID())
	}
	relationParent, hasParent := childSnapshot.ID.Parent()
	relationKey, hasKey := childSnapshot.ID.Key()
	if !hasParent || !hasKey || relationParent != parentID || relationKey != child.ChildKey() {
		return scoperun.ProcessInspection{}, fmt.Errorf("delegate child %s relation differs from interaction state", child.ProcessID())
	}
	return childSnapshot, nil
}

func restoreManagedDelegateCall(
	deployments *interactionDeploymentSet,
	snapshots map[scoperun.ProcessID]scoperun.ProcessInspection,
	members map[scoperun.ProcessID]runs.ExecutionMember,
	parentID scoperun.ProcessID,
	parentSnapshot scoperun.ProcessInspection,
	child interaction.DelegateAttribution,
) (*managedDelegateCall, error) {
	childSnapshot, err := delegateChildSnapshot(snapshots, parentID, child)
	if err != nil {
		return nil, err
	}
	member, survives := members[child.ProcessID()]
	if !survives && !childSnapshot.Status.Terminal() {
		return nil, fmt.Errorf("delegate child %s has no surviving run binding", child.ProcessID())
	}
	target, managed := deployments.delegateTarget(parentSnapshot.Program, child.ToolCall().Name)
	if !managed || target.ProgramRef() != childSnapshot.Program {
		return nil, fmt.Errorf("delegate child %s changed exact deployment", child.ProcessID())
	}
	input, arguments, err := decodeDelegateCall(child.ToolCall(), target.Descriptor())
	if err != nil {
		return nil, fmt.Errorf("decode Delegate child %s input: %w", child.ProcessID(), err)
	}
	var binding runs.ChildRunBinding
	if survives {
		binding = runs.ChildRunBinding{
			MemberID: child.ProcessID().String(), RunID: member.RunID, ParentRunID: member.ParentRunID,
		}
		if err := binding.Validate(); err != nil {
			return nil, err
		}
	}
	callID, err := logicalToolCallID(
		parentSnapshot.ID, child.ModelCallSequence(), child.ToolCallIndex(), child.ToolCall().ID, child.ToolCall().Name,
	)
	if err != nil {
		return nil, err
	}
	parentMember, _ := nearestProductMember(parentID, members)
	pending, err := parentToolPending(parentMember, callID, child.ToolCall().ID)
	if err != nil {
		return nil, fmt.Errorf("delegate child %s: %w", child.ProcessID(), err)
	}
	if survives && !pending {
		return nil, fmt.Errorf("delegate child %s has no unfinished parent tool", child.ProcessID())
	}
	return &managedDelegateCall{
		source:            deployments.delegateSource,
		identity:          delegateCallIdentity{parentID: parentID, childKey: child.ChildKey()},
		parentRelation:    parentSnapshot.ID,
		target:            target.ProgramRef(),
		call:              child.ToolCall(),
		input:             input,
		arguments:         arguments,
		modelCallSequence: child.ModelCallSequence(),
		toolCallIndex:     child.ToolCallIndex(),
		callID:            callID,
		binding:           binding, childProcessID: child.ProcessID(),
		parentToolFinished: !pending,
		// Closed children have already published their product terminal. Scope
		// retains their result until the waiting parent can commit its Tool batch.
		segmentProjected: !survives,
	}, nil
}

// parentToolPending reports whether the parent still awaits the delegating
// Tool call, and refuses one that names a different model call.
func parentToolPending(parent runs.ExecutionMember, callID runs.InvocationID, sourceCallID string) (bool, error) {
	for _, drained := range parent.DrainedTools {
		if drained.CallID != callID.String() {
			continue
		}
		if drained.SourceCallID != sourceCallID {
			return false, errors.New("child differs from its unfinished parent tool")
		}
		return true, nil
	}
	return false, nil
}
