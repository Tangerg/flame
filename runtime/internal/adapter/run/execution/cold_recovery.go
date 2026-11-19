package execution

import (
	"context"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"

	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func (i *InteractionExecutor) StageUncertainExecution(ctx context.Context, active runs.ActiveExecution) (runs.UncertainRecovery, error) {
	if active.Run.SessionID() != active.Session.ID() {
		return nil, errors.New("execution: recovery Run belongs to a different Session")
	}
	if active.Run.State() != run.Running || !active.Run.Lineage().IsRoot() {
		return runs.RecoveryUnavailable{}, nil
	}
	plan, available, err := i.planUncertainRecovery(ctx, active)
	if err != nil {
		return nil, err
	}
	if !available {
		return runs.RecoveryUnavailable{}, nil
	}
	return i.stageUncertainRecovery(ctx, plan)
}

// uncertainRecoveryPlan is a retained root checkpoint whose receipted Tool
// calls can still be verified against their original sources.
type uncertainRecoveryPlan struct {
	rootID       scoperun.ProcessID
	checkpoint   interactionCheckpointState
	inspection   scoperun.TreeInspection
	proofs       map[scoperun.EffectID]toolCallCapture
	tools        []runs.RecoveredTool
	ref          runs.ExecutorRef
	continuation runs.ExecutionContinuation
}

// planUncertainRecovery reports unavailable, not an error, for every missing
// proof that ordinary Run-loss recovery already settles.
func (i *InteractionExecutor) planUncertainRecovery(ctx context.Context, active runs.ActiveExecution) (uncertainRecoveryPlan, bool, error) {
	root := active.Run
	retained, checkpoint, rootID, available, err := i.loadRetainedRootCheckpoint(ctx, root)
	if err != nil || !available {
		return uncertainRecoveryPlan{}, false, err
	}
	inspection, err := checkpoint.tree.Inspect()
	if err != nil {
		return uncertainRecoveryPlan{}, false, err
	}
	proofs, err := checkpoint.recoverableReceipts(inspection)
	if err != nil || len(proofs) == 0 {
		return uncertainRecoveryPlan{}, false, err
	}
	tools, receipted, err := recoverSegmentTools(active, checkpoint)
	if err != nil || !receipted {
		return uncertainRecoveryPlan{}, false, err
	}
	ref := newExecutorRef(root.SessionID())
	continuation, err := runs.NewExecutionContinuation(runs.ExecutionContinuation{
		SessionID: root.SessionID(), ExecutorID: ref.ExecutorID, RootRunID: root.ID(),
		Checkpoint: retained, Capabilities: root.Capabilities(), GoalIncarnationID: root.GoalIncarnationID(),
		Workspace: active.Session.Workspace().Path(), Isolated: active.Session.Isolated(),
		Members: []runs.ExecutionMember{{RunID: root.ID(), MemberID: rootID.String(), ModelSelection: root.ModelSelection()}},
	})
	if err != nil {
		return uncertainRecoveryPlan{}, false, err
	}
	if err := validateRestoreWorkspace(continuation); err != nil {
		if errors.Is(err, runs.ErrExecutorStateLost) {
			return uncertainRecoveryPlan{}, false, nil
		}
		return uncertainRecoveryPlan{}, false, err
	}
	available, err = i.originalSourcesAvailable(ctx, checkpoint, inspection, proofs)
	if err != nil || !available {
		return uncertainRecoveryPlan{}, false, err
	}
	return uncertainRecoveryPlan{
		rootID: rootID, checkpoint: checkpoint, inspection: inspection, proofs: proofs,
		tools: tools, ref: ref, continuation: continuation,
	}, true, nil
}

// loadRetainedRootCheckpoint reads the root checkpoint and the acknowledged
// tree it extends; a missing or foreign-build checkpoint is unavailable.
func (i *InteractionExecutor) loadRetainedRootCheckpoint(ctx context.Context, root run.Run) (runs.ExecutorCheckpoint, interactionCheckpointState, scoperun.ProcessID, bool, error) {
	start := interactionStartID(root.SessionID(), root.ID())
	if !start.Valid() {
		return runs.ExecutorCheckpoint{}, interactionCheckpointState{}, scoperun.ProcessID{}, false, scoperun.ErrInvalidStart
	}
	rootID := start.Root()
	retained, err := i.config.ExecutionTrees.LoadCheckpoint(ctx, rootID.String())
	if errors.Is(err, runs.ErrExecutorCheckpointNotFound) {
		return runs.ExecutorCheckpoint{}, interactionCheckpointState{}, rootID, false, nil
	}
	if err != nil || !i.acceptsBuild(retained.BuildID) {
		return runs.ExecutorCheckpoint{}, interactionCheckpointState{}, rootID, false, err
	}
	checkpoint, err := decodeRunCheckpoint(retained, root.SessionID(), root.ID())
	if err != nil {
		return runs.ExecutorCheckpoint{}, interactionCheckpointState{}, rootID, false, err
	}
	head, found, err := i.config.ExecutionTrees.LoadExecutionTree(ctx, root.SessionID(), rootID.String())
	if err != nil {
		return runs.ExecutorCheckpoint{}, interactionCheckpointState{}, rootID, false, err
	}
	if !found {
		return runs.ExecutorCheckpoint{}, interactionCheckpointState{}, rootID, false, errors.New("execution: retained context has no acknowledged tree")
	}
	checkpoint.tree, err = decodeExecutionTree(head, rootID)
	if err != nil {
		return runs.ExecutorCheckpoint{}, interactionCheckpointState{}, rootID, false, err
	}
	return retained, checkpoint, rootID, true, nil
}

// stageUncertainRecovery assembles the recovered Interaction and verifies its
// captured Tool settlements before the executor can be published.
func (i *InteractionExecutor) stageUncertainRecovery(ctx context.Context, plan uncertainRecoveryPlan) (_ runs.UncertainRecovery, resultErr error) {
	finish, err := i.sessions.beginAssembly()
	if err != nil {
		return nil, err
	}
	defer finish()
	checkpoint := plan.checkpoint
	session, err := i.assembleInteraction(ctx, plan.ref, checkpoint.restoredStart(plan.continuation), &checkpoint)
	if err != nil {
		return nil, err
	}
	defer func() {
		if session != nil {
			resultErr = errors.Join(resultErr, i.discardInteraction(session))
		}
	}()
	if pendingCheckpointSources(session, checkpoint) {
		return runs.RecoveryDeferred{}, nil
	}
	if err := session.verifySettledTools(ctx, checkpoint.tree, plan.proofs, plan.tools); err != nil {
		return nil, err
	}
	if err := session.validateCapturedToolArguments(plan.inspection, plan.proofs); err != nil {
		return nil, err
	}
	if err := validateRecovery(ctx, session.deployment, checkpoint.tree); err != nil {
		if errors.Is(err, scoperun.ErrInvalidLog) && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			return runs.RecoveryUnavailable{}, nil
		}
		return nil, err
	}
	session.material.restoreCalibrations(checkpoint.contextByProcess)
	session.state.toolMetadata = checkpoint.toolMetadata
	session.state.pendingSteers = checkpoint.pendingSteers
	session.state.pendingResumeInputs = checkpoint.pendingResumeInputs
	session.state.restoreTree = checkpoint.tree
	session.state.boundary = interactionBoundaryRecovery
	session.state.dispatchReady = make(chan struct{})
	if err := i.publishInteraction(ctx, session); err != nil {
		return nil, err
	}
	session = nil
	return runs.StagedRecovery{Ref: plan.ref, MemberID: plan.rootID.String(), Tools: plan.tools}, nil
}

// recoverSegmentTools reports receipted false when any active call lacks an
// original receipt; a mismatched Item is a contradiction, not unavailability.
func recoverSegmentTools(active runs.ActiveExecution, checkpoint interactionCheckpointState) ([]runs.RecoveredTool, bool, error) {
	var tools []runs.RecoveredTool
	for _, invocation := range active.Tools {
		if invocation.RunID != active.Run.ID() || invocation.SegmentID != active.Run.ActiveSegmentID() {
			continue
		}
		metadata, found := checkpoint.toolMetadata[invocation.CallID]
		if !found || metadata.Receipt == nil {
			return nil, false, nil
		}
		item, found := active.Items[invocation.ItemID]
		if !found || item.RunID() != active.Run.ID() || item.SessionID() != active.Run.SessionID() || item.Status() != transcript.ItemRunning {
			return nil, false, errors.New("execution: captured Tool Item is unavailable")
		}
		original, found := item.ToolCall()
		if !found || !original.Invocation.MatchesSource(metadata.Start.ToolName, metadata.Start.Source) {
			return nil, false, errors.New("execution: captured Tool attribution differs from the original Item")
		}
		tools = append(tools, runs.RecoveredTool{Start: metadata.Start, Invocation: invocation})
	}
	return tools, true, nil
}

func (i *InteractionExecutor) BeginUncertainExecution(ctx context.Context, ref runs.ExecutorRef) error {
	session, err := i.session(ref)
	if err != nil {
		return err
	}
	if err := session.state.begin(interactionBoundaryRecovery); err != nil {
		return err
	}
	if err := session.recoverTree(); err != nil {
		return err
	}
	session.reportUnknownEffects()
	session.state.mu.Lock()
	session.state.continueExecution()
	session.state.mu.Unlock()
	session.startWorkers()
	return nil
}

func (i interactionCheckpointState) recoverableReceipts(inspection scoperun.TreeInspection) (map[scoperun.EffectID]toolCallCapture, error) {
	rootID := i.tree.Root
	proofs := make(map[scoperun.EffectID]toolCallCapture)
	for _, metadata := range i.toolMetadata {
		if metadata.Receipt == nil {
			continue
		}
		if metadata.MemberID != rootID.String() {
			return nil, nil
		}
		id := metadata.Receipt.EffectID
		if _, duplicate := proofs[id]; duplicate {
			return nil, errors.New("execution: duplicate retained Tool effect identity")
		}
		proofs[id] = metadata.toolCallCapture
	}
	if len(proofs) == 0 {
		return nil, nil
	}
	root, found := inspection.Process(rootID)
	// A committed Tool settlement can leave the program running before its
	// next step commits. Original receipts and every unresolved Effect below
	// still bound recovery; an explicitly paused process remains excluded.
	if !found || (root.Status != scoperun.StatusRunning && root.Status != scoperun.StatusWaiting && root.Status != scoperun.StatusDraining) {
		return nil, nil
	}
	for _, process := range inspection.Processes {
		if len(process.Waits) > 0 {
			return nil, nil
		}
		for _, pending := range process.PendingEffects {
			if pending.Outcome.Valid() && pending.Outcome.Status() != scoperun.SettlementStatusUnknown {
				continue
			}
			if _, supported := proofs[pending.Request.ID()]; !supported {
				return nil, nil
			}
		}
	}
	return proofs, nil
}

func (i *interactionSession) verifySettledTools(ctx context.Context, tree scoperun.Log, proofs map[scoperun.EffectID]toolCallCapture, open []runs.RecoveredTool) error {
	unverified := make(map[string]struct{}, len(proofs))
	for _, metadata := range proofs {
		unverified[metadata.Start.CallID] = struct{}{}
	}
	for _, call := range open {
		delete(unverified, call.Invocation.CallID)
	}
	if len(unverified) == 0 {
		return nil
	}
	rounds, err := i.settledToolResults(tree)
	if err != nil {
		return err
	}
	for _, round := range rounds {
		for _, entry := range round.Entries() {
			identity, err := logicalToolCallID(round.ProcessID(), round.ModelCallSequence(), entry.ToolCallIndex, entry.Call.ID, entry.Call.Name)
			if err != nil {
				return err
			}
			if _, required := unverified[identity.String()]; !required {
				continue
			}
			publication, err := resultPublication(identity, entry)
			if err != nil {
				return err
			}
			committed, err := i.executionTrees.ExecutionResultCommitted(ctx, i.scope.SessionID, publication)
			if err != nil {
				return err
			}
			if !committed {
				return errors.New("execution: settled Tool has no publication receipt")
			}
			delete(unverified, identity.String())
		}
	}
	if len(unverified) != 0 {
		return errors.New("execution: captured Tool has neither its original attempt nor a settlement")
	}
	return nil
}

func (i *InteractionExecutor) originalSourcesAvailable(ctx context.Context, checkpoint interactionCheckpointState, inspection scoperun.TreeInspection, proofs map[scoperun.EffectID]toolCallCapture) (bool, error) {
	for id, metadata := range proofs {
		proof := metadata.Receipt
		process, found := inspection.Process(id.Process)
		if !found {
			return false, errors.New("execution: retained Tool process is missing")
		}
		for _, pending := range process.PendingEffects {
			if pending.Request.ID() != id {
				continue
			}
			invocation, err := interaction.ToolInvocationFromRequest(pending.Request)
			if err != nil {
				return false, err
			}
			reference, err := toolProposalSource(checkpoint.tree, invocation.Reference(), invocation.ToolCall())
			if err != nil {
				return false, err
			}
			call := invocation.ToolCall()
			if reference.process != checkpoint.tree.Root || reference.sequence != metadata.Start.ModelCallSequence || reference.index != metadata.Start.ToolCallIndex || call.ID != metadata.Start.SourceCallID || call.Name != metadata.Start.ToolName {
				return false, errors.New("execution: retained request differs from its captured attribution")
			}
		}
		activation, err := originalToolActivation(metadata.Start.Source, proof.Release, checkpoint.bindings)
		if err != nil {
			return false, err
		}
		available, err := i.config.OriginalToolResults.AwaitOriginalToolSource(ctx, metadata.Start.Source, proof.Release, proof.Authority, activation)
		if err != nil {
			return false, err
		}
		// The ownership sweep can close the lost Core Run without consuming or
		// replaying the unavailable plugin's original private receipt.
		if !available {
			return false, nil
		}
	}
	return true, nil
}

func (i *interactionSession) validateCapturedToolArguments(inspection scoperun.TreeInspection, proofs map[scoperun.EffectID]toolCallCapture) error {
	for id, metadata := range proofs {
		process, found := inspection.Process(id.Process)
		if !found {
			return errors.New("execution: retained Tool process is missing")
		}
		owner := i.state.deployments.observedTools[process.Program][metadata.Start.Source]
		if !owner.matchesReceipt(*metadata.Receipt) {
			return errors.New("execution: captured Tool owner is unavailable")
		}
		if _, err := owner.binding.Contract().Prepare(toolcontract.Request{Name: metadata.Start.ToolName, Arguments: metadata.Start.Arguments}, toolcontract.Admission{}); err != nil {
			return fmt.Errorf("execution: validate captured effective arguments: %w", err)
		}
	}
	return nil
}
