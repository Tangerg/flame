package agentexec

import (
	"errors"
	"fmt"
	"maps"
	"math"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/interaction"
)

func (i *interactionSession) initializeRestoredContinuation(
	root *agent.Process,
	continuation runs.WaitingContinuation,
	checkpoint interactionCheckpointState,
	boundary interactionBoundary,
) error {
	if boundary != interactionBoundaryWaiting && boundary != interactionBoundaryContinuationStaged {
		return errors.New("invalid restored Interaction boundary")
	}
	if root == nil || root.ID() != checkpoint.tree.RootID() ||
		!isInteractionWaitingBoundary(root.Status()) {
		return fmt.Errorf("%w: restored Interaction root is not at a waiting boundary", runs.ErrExecutorStateLost)
	}
	snapshots := make(map[agent.ProcessID]agent.ProcessSnapshot, len(checkpoint.tree.ProcessSnapshots()))
	for _, snapshot := range checkpoint.tree.ProcessSnapshots() {
		snapshots[snapshot.ProcessID()] = snapshot
	}
	members, err := restoredWaitingMembers(continuation, snapshots, root.ID())
	if err != nil {
		return err
	}
	usageByProcess, carriedUsage, err := restoreInteractionAccounting(
		continuation.Checkpoint.Usage, checkpoint, members,
	)
	if err != nil {
		return fmt.Errorf("%w: restore Interaction accounting: %w", runs.ErrExecutorStateLost, err)
	}
	delegateCalls, delegateChildren, err := i.restoreDelegateCalls(snapshots, members)
	if err != nil {
		return fmt.Errorf("%w: restore Delegate bindings: %w", runs.ErrExecutorStateLost, err)
	}
	i.accounting.restore(usageByProcess, carriedUsage, checkpoint.contextByProcess)
	i.state.mu.Lock()
	defer i.state.mu.Unlock()
	if i.state.begun || i.state.finished || i.state.process != nil {
		return runs.ErrExecutionClaimed
	}
	i.state.process = root
	i.state.admittedProcessID = root.ID()
	i.state.begun = true
	i.state.boundary = boundary
	i.state.waitingCheckpoint = continuation.Checkpoint.Clone()
	i.state.delegateCalls = delegateCalls
	i.state.delegateChildren = delegateChildren
	i.state.pendingSteers = checkpoint.pendingSteers
	i.state.pendingContinuation = checkpoint.pendingContinuation
	return nil
}

func restoredWaitingMembers(
	continuation runs.WaitingContinuation,
	snapshots map[agent.ProcessID]agent.ProcessSnapshot,
	rootID agent.ProcessID,
) (map[agent.ProcessID]runs.WaitingMember, error) {
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
		if snapshot.Status().Terminal() {
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
		return nil, nil, errors.New("agentexec: Interaction deployments are unavailable")
	}
	calls := make(map[delegateCallIdentity]*managedDelegateCall)
	children := make(map[agent.ProcessID]*managedDelegateCall)
	for parentID, parentSnapshot := range snapshots {
		if _, active := members[parentID]; !active {
			continue
		}
		active, found, err := interaction.ActiveDelegateChildrenFromSnapshot(parentSnapshot)
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
	if !managed || target != childSnapshot.DeploymentRef() {
		return nil, fmt.Errorf("delegate child %s changed exact deployment", child.ProcessID())
	}
	input, arguments, err := decodeDelegateCall(child.ToolCall())
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
	callID, err := delegatedToolCallID(
		parentSnapshot.Relation(), child.ModelCallSequence(), child.ToolCallIndex(), child.ToolCall(),
	)
	if err != nil {
		return nil, err
	}
	var pending bool
	for _, drained := range members[parentID].DrainedTools {
		if drained.CallID != callID.String() {
			continue
		}
		if drained.SourceCallID != child.ToolCall().ID || drained.Name != child.ToolCall().Name ||
			drained.Arguments != arguments.Canonical() {
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
		target:            target,
		call:              child.ToolCall(),
		input:             input,
		arguments:         arguments,
		modelCallSequence: child.ModelCallSequence(),
		toolCallIndex:     child.ToolCallIndex(),
		callID:            callID,
		binding:           binding, childProcessID: child.ProcessID(), toolStarted: true,
		parentToolFinished: !pending,
		// Closed children have already published their product terminal. Scope
		// retains their result until the waiting parent can commit its Tool batch.
		assistantProjected: !survives,
		segmentProjected:   !survives,
	}, nil
}

func restoreInteractionAccounting(
	total accounting.Snapshot,
	checkpoint interactionCheckpointState,
	members map[agent.ProcessID]runs.WaitingMember,
) (map[agent.ProcessID]map[string]accounting.ModelUsage, map[string]accounting.ModelUsage, error) {
	if err := total.Validate(); err != nil {
		return nil, nil, err
	}
	usageByProcess, activeAggregate, err := restoreActiveInteractionAccounting(checkpoint, members)
	if err != nil {
		return nil, nil, err
	}
	carried, err := subtractInteractionUsage(total, activeAggregate)
	if err != nil {
		return nil, nil, err
	}
	expectedCarriedCalls, err := expectedCarriedInteractionCalls(checkpoint, members)
	if err != nil {
		return nil, nil, err
	}
	if err := validateCarriedInteractionCalls(carried, expectedCarriedCalls); err != nil {
		return nil, nil, err
	}
	return usageByProcess, carried, nil
}

func restoreActiveInteractionAccounting(
	checkpoint interactionCheckpointState,
	members map[agent.ProcessID]runs.WaitingMember,
) (map[agent.ProcessID]map[string]accounting.ModelUsage, map[string]accounting.ModelUsage, error) {
	usageByProcess := make(map[agent.ProcessID]map[string]accounting.ModelUsage, len(members))
	activeAggregate := make(map[string]accounting.ModelUsage)
	for processID, member := range members {
		usage, err := accountingFromRunMetrics(member.Metrics, checkpoint.callsByProcess[processID])
		if err != nil {
			return nil, nil, fmt.Errorf("member %s: %w", processID, err)
		}
		usageByProcess[processID] = usage
		if err := mergeInteractionUsage(activeAggregate, usage); err != nil {
			return nil, nil, err
		}
	}
	return usageByProcess, activeAggregate, nil
}

func expectedCarriedInteractionCalls(
	checkpoint interactionCheckpointState,
	members map[agent.ProcessID]runs.WaitingMember,
) (map[string]int, error) {
	expected, err := addInteractionCallCounts(nil, checkpoint.carriedCallCount)
	if err != nil {
		return nil, fmt.Errorf("carried call counts: %w", err)
	}
	for processID, byModel := range checkpoint.callsByProcess {
		if _, active := members[processID]; active {
			continue
		}
		expected, err = addInteractionCallCounts(expected, byModel)
		if err != nil {
			return nil, fmt.Errorf("retired member %s call counts: %w", processID, err)
		}
	}
	return expected, nil
}

func addInteractionCallCounts(current, additions map[string]int) (map[string]int, error) {
	next := maps.Clone(current)
	if next == nil {
		next = make(map[string]int)
	}
	for model, calls := range additions {
		if calls <= 0 {
			return nil, fmt.Errorf("model %q call count is not positive", model)
		}
		if existing := next[model]; existing > math.MaxInt-calls {
			return nil, fmt.Errorf("model %q call count overflows", model)
		}
		next[model] += calls
	}
	return next, nil
}

func validateCarriedInteractionCalls(
	carried map[string]accounting.ModelUsage,
	expected map[string]int,
) error {
	for model, usage := range carried {
		if expected[model] != usage.Calls {
			return fmt.Errorf("carried model %q call count differs from checkpoint", model)
		}
		delete(expected, model)
	}
	for model, calls := range expected {
		if calls != 0 {
			return fmt.Errorf("carried model %q has calls without aggregate usage", model)
		}
	}
	return nil
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
			TokenUsage: accounting.TokenUsage{
				PromptTokens: value.InputTokens, CompletionTokens: value.OutputTokens,
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
	return total.PromptTokens == value.InputTokens && total.CompletionTokens == value.OutputTokens &&
		total.ReasoningTokens == value.ReasoningTokens && total.CacheReadTokens == value.CacheReadTokens &&
		total.CacheWriteTokens == value.CacheWriteTokens && total.Cost.Equal(cost)
}

func subtractInteractionUsage(
	total accounting.Snapshot,
	active map[string]accounting.ModelUsage,
) (map[string]accounting.ModelUsage, error) {
	remaining := make(map[string]accounting.ModelUsage, len(total.Models))
	for _, usage := range total.Models {
		remaining[usage.Model] = usage
	}
	for model, used := range active {
		value, found := remaining[model]
		if !found {
			return nil, fmt.Errorf("active model %q usage exceeds tree checkpoint", model)
		}
		remainder, present, err := value.Subtract(used)
		if err != nil {
			return nil, fmt.Errorf("active model %q usage exceeds tree checkpoint: %w", model, err)
		}
		if !present {
			delete(remaining, model)
			continue
		}
		remaining[model] = remainder
	}
	return remaining, nil
}
