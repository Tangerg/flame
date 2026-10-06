package execution

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
)

func (i *interactionSession) validateWaitingTree(ctx context.Context, continuation runs.WaitingContinuation, checkpoint interactionCheckpointState) error {

	// A retained Unknown settlement is a restorable fact to the Engine; refusing
	// to resume on top of one is this Runtime's policy, so it stays here.
	snapshots := make(map[agent.ProcessID]agent.ProcessSnapshot)
	for _, snapshot := range checkpoint.tree.ProcessSnapshots() {
		snapshots[snapshot.ProcessID()] = snapshot
		if len(snapshot.UnknownEffectIDs()) != 0 {
			return fmt.Errorf("%w: waiting tree contains unresolved effects", runs.ErrExecutorStateLost)
		}
	}
	// Scope's pure restore validation may wrap cancellation as an invalid
	// snapshot. An interrupted check proves nothing about checkpoint validity.
	if err := i.engine.ValidateRestorableTree(ctx, i.deployment, checkpoint.tree); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if errors.Is(err, agent.ErrInvalidTreeSnapshot) {
			return fmt.Errorf("%w: %w", runs.ErrExecutorStateLost, err)
		}
		return err
	}
	members, err := i.restoredWaitingMembers(continuation, snapshots, checkpoint.tree.RootID())
	if err != nil {
		return fmt.Errorf("%w: waiting members: %w", runs.ErrExecutorStateLost, err)
	}
	if _, err := restoreInteractionAccounting(checkpoint, members); err != nil {
		return fmt.Errorf("%w: waiting accounting: %w", runs.ErrExecutorStateLost, err)
	}
	if _, _, err := i.restoreDelegateCalls(snapshots, members); err != nil {
		return fmt.Errorf("%w: waiting Delegate bindings: %w", runs.ErrExecutorStateLost, err)
	}
	return nil
}

func (i *interactionSession) initializeRestoredContinuation(
	root *agent.Process,
	continuation runs.WaitingContinuation,
	checkpoint interactionCheckpointState,
	boundary interactionBoundary,
) error {
	if boundary != interactionBoundaryWaiting && boundary != interactionBoundaryContinuationStaged {
		return errors.New("invalid restored Interaction boundary")
	}
	snapshots := make(map[agent.ProcessID]agent.ProcessSnapshot, len(checkpoint.tree.ProcessSnapshots()))
	for _, snapshot := range checkpoint.tree.ProcessSnapshots() {
		snapshots[snapshot.ProcessID()] = snapshot
	}
	// The boundary being restored is the checkpoint's, so the checkpoint is what
	// has to carry it. A live status would describe the tree after restore
	// rather than the cut this continuation answers.
	rootSnapshot, captured := snapshots[checkpoint.tree.RootID()]
	if root == nil || root.ID() != checkpoint.tree.RootID() || !captured ||
		!isInteractionWaitingBoundary(rootSnapshot.Status()) {
		return fmt.Errorf("%w: restored Interaction root is not at a waiting boundary", runs.ErrExecutorStateLost)
	}
	members, err := i.restoredWaitingMembers(continuation, snapshots, root.ID())
	if err != nil {
		return err
	}
	usageByProcess, err := restoreInteractionAccounting(checkpoint, members)
	if err != nil {
		return fmt.Errorf("%w: restore Interaction accounting: %w", runs.ErrExecutorStateLost, err)
	}
	delegateCalls, delegateChildren, err := i.restoreDelegateCalls(snapshots, members)
	if err != nil {
		return fmt.Errorf("%w: restore Delegate bindings: %w", runs.ErrExecutorStateLost, err)
	}
	i.accounting.restore(usageByProcess, checkpoint.contextByProcess)
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	if i.state.phase != interactionPending || i.state.process != root {
		return runs.ErrExecutionClaimed
	}
	i.state.admittedProcessID = root.ID()
	i.state.phase = interactionBegun
	i.state.boundary = boundary
	i.state.dispatchReady = make(chan struct{})
	i.state.waitingCheckpoint = continuation.Checkpoint.Clone()
	i.state.delegateCalls = delegateCalls
	i.state.delegateChildren = delegateChildren
	i.state.pendingSteers = checkpoint.pendingSteers
	i.state.pendingContinuation = checkpoint.pendingContinuation
	i.state.toolMetadata = checkpoint.toolMetadata
	return nil
}

func (i *interactionSession) restoredWaitingMembers(
	continuation runs.WaitingContinuation,
	snapshots map[agent.ProcessID]agent.ProcessSnapshot,
	rootID agent.ProcessID,
) (map[agent.ProcessID]runs.WaitingMember, error) {
	i.state.mu.Lock()
	deployments := i.state.deployments
	i.state.mu.Unlock()
	if deployments == nil {
		return nil, errors.New("execution: Interaction deployments are unavailable")
	}
	members := make(map[agent.ProcessID]runs.WaitingMember, len(continuation.Members))
	runByProcess := make(map[agent.ProcessID]string, len(continuation.Members))
	for _, member := range continuation.Members {
		processID, err := agent.ParseProcessID(member.MemberID)
		if err != nil {
			return nil, fmt.Errorf("waiting member %q: %w", member.MemberID, err)
		}
		snapshot, found := snapshots[processID]
		if !found || snapshot.Status().Terminal() {
			return nil, fmt.Errorf("waiting member %s has no active Process", processID)
		}
		members[processID] = member
		runByProcess[processID] = member.RunID
	}
	rootMember, found := members[rootID]
	if !found || rootMember.RunID != continuation.RootRunID || rootMember.ParentRunID != "" {
		return nil, errors.New("restored root Process differs from the product root member")
	}
	for processID, member := range members {
		relation := snapshots[processID].Relation()
		if processID == rootID {
			if !relation.IsRoot() {
				return nil, errors.New("restored product root has a child Process relation")
			}
			continue
		}
		parentID, child := relation.ParentID()
		parentRunID, parentSurvives := runByProcess[parentID]
		if !child || !parentSurvives || parentRunID != member.ParentRunID {
			return nil, fmt.Errorf("restored member %s differs from product lineage", processID)
		}
	}
	for processID, snapshot := range snapshots {
		if snapshot.Status().Terminal() || deployments.toolChild(snapshot.DeploymentRef()) {
			continue
		}
		if _, survives := members[processID]; !survives {
			return nil, fmt.Errorf("active Process %s has no surviving product member", processID)
		}
	}
	return members, nil
}

func (i *interactionSession) restoreDelegateCalls(
	snapshots map[agent.ProcessID]agent.ProcessSnapshot,
	members map[agent.ProcessID]runs.WaitingMember,
) (map[delegateCallIdentity]*managedDelegateCall, map[agent.ProcessID]*managedDelegateCall, error) {
	i.state.mu.Lock()
	deployments := i.state.deployments
	i.state.mu.Unlock()
	if deployments == nil {
		return nil, nil, errors.New("execution: Interaction deployments are unavailable")
	}
	calls := make(map[delegateCallIdentity]*managedDelegateCall)
	children := make(map[agent.ProcessID]*managedDelegateCall)
	for parentID, parentSnapshot := range snapshots {
		if _, active := members[parentID]; !active {
			continue
		}
		active, found, err := interaction.ActiveDelegateChildren(parentSnapshot)
		if err != nil {
			return nil, nil, fmt.Errorf("inspect parent %s: %w", parentID, err)
		}
		if !found {
			continue
		}
		for _, child := range active {
			managedCall, err := restoreManagedDelegateCall(
				deployments, snapshots, members, parentID, parentSnapshot, child,
			)
			if err != nil {
				return nil, nil, err
			}
			calls[managedCall.identity] = managedCall
			children[child.ProcessID()] = managedCall
		}
	}
	for processID := range members {
		if snapshots[processID].Relation().IsRoot() {
			continue
		}
		if children[processID] == nil {
			return nil, nil, fmt.Errorf("surviving child %s has no active Delegate attribution", processID)
		}
	}
	return calls, children, nil
}

func restoreManagedDelegateCall(
	deployments *interactionDeploymentSet,
	snapshots map[agent.ProcessID]agent.ProcessSnapshot,
	members map[agent.ProcessID]runs.WaitingMember,
	parentID agent.ProcessID,
	parentSnapshot agent.ProcessSnapshot,
	child interaction.ActiveDelegateChild,
) (*managedDelegateCall, error) {
	childSnapshot, exists := snapshots[child.ProcessID()]
	if !exists {
		return nil, fmt.Errorf("delegate child %s is absent from the tree", child.ProcessID())
	}
	relation := childSnapshot.Relation()
	relationParent, hasParent := relation.ParentID()
	relationKey, hasKey := relation.ChildKey()
	if !hasParent || !hasKey || relationParent != parentID || relationKey != child.ChildKey() {
		return nil, fmt.Errorf("delegate child %s relation differs from interaction state", child.ProcessID())
	}
	member, survives := members[child.ProcessID()]
	if !survives && !childSnapshot.Status().Terminal() {
		return nil, fmt.Errorf("delegate child %s has no surviving run binding", child.ProcessID())
	}
	target, managed := deployments.delegateTarget(parentSnapshot.DeploymentRef(), child.ToolCall().Name)
	if !managed || target.DeploymentRef() != childSnapshot.DeploymentRef() {
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
		parentSnapshot.Relation().ProcessID(), child.ModelCallSequence(), child.ToolCallIndex(), child.ToolCall().ID, child.ToolCall().Name,
	)
	if err != nil {
		return nil, err
	}
	var pending bool
	for _, drained := range members[parentID].DrainedTools {
		if drained.CallID != callID.String() {
			continue
		}
		if drained.SourceCallID != child.ToolCall().ID {
			return nil, fmt.Errorf("delegate child %s differs from its unfinished parent tool", child.ProcessID())
		}
		pending = true
		break
	}
	if survives && !pending {
		return nil, fmt.Errorf("delegate child %s has no unfinished parent tool", child.ProcessID())
	}
	return &managedDelegateCall{
		identity:          delegateCallIdentity{parentID: parentID, childKey: child.ChildKey()},
		parentRelation:    parentSnapshot.Relation(),
		target:            target.DeploymentRef(),
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

// restoreInteractionAccounting rebuilds per-Process usage for the surviving
// members only: each member's Run owns its usage, and the checkpoint adds the
// per-model call counts the Run does not record.
func restoreInteractionAccounting(
	checkpoint interactionCheckpointState,
	members map[agent.ProcessID]runs.WaitingMember,
) (map[agent.ProcessID]map[string]accounting.ModelUsage, error) {
	usageByProcess := make(map[agent.ProcessID]map[string]accounting.ModelUsage, len(members))
	for processID, member := range members {
		usage, err := accountingFromRunMetrics(member.Metrics, checkpoint.callsByProcess[processID])
		if err != nil {
			return nil, fmt.Errorf("member %s: %w", processID, err)
		}
		usageByProcess[processID] = usage
	}
	return usageByProcess, nil
}

func accountingFromRunMetrics(
	metrics run.Metrics,
	callsByModel map[string]int,
) (map[string]accounting.ModelUsage, error) {
	if err := metrics.Validate(); err != nil {
		return nil, err
	}
	usage, reported := metrics.Usage()
	if metrics.Steps() == 0 {
		return emptyRunMetricsAccounting(reported, callsByModel)
	}
	if !reported || len(usage.ByModel) == 0 {
		return nil, errors.New("model calls have no per-model usage")
	}
	result, err := modelUsageFromRunMetrics(usage.ByModel, callsByModel)
	if err != nil {
		return nil, err
	}
	total, err := interactionUsageSnapshot(result).Total()
	if err != nil {
		return nil, err
	}
	if total.Calls != metrics.Steps() || !sameTranscriptUsage(total, usage.Total) {
		return nil, errors.New("product metrics differ from reconstructed executor accounting")
	}
	return result, nil
}

func emptyRunMetricsAccounting(
	reported bool,
	callsByModel map[string]int,
) (map[string]accounting.ModelUsage, error) {
	if reported || len(callsByModel) != 0 {
		return nil, errors.New("zero-step member has accounting state")
	}
	return map[string]accounting.ModelUsage{}, nil
}

func modelUsageFromRunMetrics(
	byModel map[string]accounting.Totals,
	callsByModel map[string]int,
) (map[string]accounting.ModelUsage, error) {
	result := make(map[string]accounting.ModelUsage, len(byModel))
	for model, value := range byModel {
		calls := callsByModel[model]
		if calls <= 0 {
			return nil, fmt.Errorf("model %q has no durable call count", model)
		}
		cost, err := accounting.CostFromOptional(value.CostUSD)
		if err != nil {
			return nil, fmt.Errorf("model %q cost: %w", model, err)
		}
		usage := accounting.ModelUsage{
			Model: model,
			Tokens: accounting.Tokens{
				InputTokens: value.InputTokens, OutputTokens: value.OutputTokens,
				ReasoningTokens: value.ReasoningTokens, CacheReadTokens: value.CacheReadTokens,
				CacheWriteTokens: value.CacheWriteTokens,
			},
			Cost:  cost,
			Calls: calls,
		}
		if err := usage.Validate(); err != nil {
			return nil, fmt.Errorf("model %q: %w", model, err)
		}
		result[model] = usage
	}
	if len(result) != len(callsByModel) {
		return nil, errors.New("durable call counts name a model absent from product metrics")
	}
	return result, nil
}

func sameTranscriptUsage(total accounting.ModelUsage, value accounting.Totals) bool {
	cost, err := accounting.CostFromOptional(value.CostUSD)
	if err != nil {
		return false
	}
	return total.InputTokens == value.InputTokens && total.OutputTokens == value.OutputTokens &&
		total.ReasoningTokens == value.ReasoningTokens && total.CacheReadTokens == value.CacheReadTokens &&
		total.CacheWriteTokens == value.CacheWriteTokens && total.Cost.Equal(cost)
}
