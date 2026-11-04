package execution

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"

	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func (i *InteractionExecutor) PrepareUncertainExecution(ctx context.Context, active runs.ActiveExecution) (_ runs.UncertainExecutionPlan, _ runs.RecoveryVerdict, resultErr error) {
	var zero runs.UncertainExecutionPlan
	if active.Run.State() != run.Running || !active.Run.Lineage().IsRoot() || active.Run.SessionID() != active.Session.ID() || !i.acceptsBuild(active.Context.BuildID) || len(active.Context.Dependencies()) == 0 {
		return zero, runs.RecoveryUnavailable, nil
	}
	if err := active.Context.ValidateOwnership(active.Context.RootMemberID, active.Run.SessionID()); err != nil {
		return zero, runs.RecoveryUnavailable, err
	}
	checkpoint, err := decodeExecutorCheckpoint(active.Context)
	if err != nil {
		return zero, runs.RecoveryUnavailable, err
	}
	rootID, err := scoperun.ParseProcessID(active.Context.RootMemberID)
	if err != nil {
		return zero, runs.RecoveryUnavailable, err
	}
	if checkpoint.handoff.rootID != rootID {
		return zero, runs.RecoveryUnavailable, errors.New("execution: context root differs from its owner")
	}
	head, found, err := i.config.ExecutionTrees.LoadExecutionTree(ctx, active.Run.SessionID(), rootID.String())
	if err != nil {
		return zero, runs.RecoveryUnavailable, err
	}
	if !found {
		return zero, runs.RecoveryUnavailable, errors.New("execution: retained context has no acknowledged tree")
	}
	checkpoint.tree, err = decodeExecutionTree(head, rootID)
	if err != nil {
		return zero, runs.RecoveryUnavailable, err
	}
	if i.config.OriginalToolResults == nil {
		return zero, runs.RecoveryUnavailable, nil
	}
	proofs, err := checkpoint.recoverableReceipts(rootID)
	if err != nil || len(proofs) == 0 {
		return zero, runs.RecoveryUnavailable, err
	}
	items := make(map[string]transcript.Item, len(active.Items))
	for _, item := range active.Items {
		items[item.ID()] = item
	}
	var drained []runs.DrainedTool
	var tools []runs.RecoveredTool
	pendingCalls := make(map[string]bool)
	for _, invocation := range active.Tools {
		if invocation.RunID != active.Run.ID() || invocation.SegmentID != active.Run.ActiveSegmentID() {
			continue
		}
		metadata, found := checkpoint.toolMetadata[invocation.CallID]
		if !found || metadata.Receipt == nil {
			return zero, runs.RecoveryUnavailable, nil
		}
		item, found := items[invocation.ItemID]
		if !found || item.RunID() != active.Run.ID() || item.SessionID() != active.Run.SessionID() || item.Status() != transcript.ItemRunning {
			return zero, runs.RecoveryUnavailable, errors.New("execution: captured Tool Item is unavailable")
		}
		original, found := item.ToolCall()
		if !found || !original.Invocation.MatchesSource(metadata.Start.ToolName, metadata.Receipt.Tool) {
			return zero, runs.RecoveryUnavailable, errors.New("execution: captured Tool attribution differs from the original Item")
		}
		drained = append(drained, runs.DrainedTool{ItemID: item.ID(), CallID: invocation.CallID, SourceCallID: metadata.Start.SourceCallID})
		metadata.Start.Source = metadata.Receipt.Tool
		tools = append(tools, runs.RecoveredTool{Start: metadata.Start, Invocation: invocation})
		pendingCalls[invocation.CallID] = true
	}
	slices.SortFunc(drained, func(a, b runs.DrainedTool) int { return strings.Compare(a.CallID, b.CallID) })
	continuation, err := runs.NewExecutionContinuation(runs.ExecutionContinuation{SessionID: active.Run.SessionID(), ExecutorID: "exec_recovery_" + active.Run.ID(), RootRunID: active.Run.ID(), Checkpoint: active.Context.Clone(), Capabilities: active.Run.Capabilities(), GoalIncarnationID: active.Run.GoalIncarnationID(), Workspace: active.Session.Workspace().Path(), Isolated: active.Session.Isolated(), Members: []runs.ExecutionMember{{RunID: active.Run.ID(), MemberID: rootID.String(), ModelSelection: active.Run.ModelSelection(), Metrics: active.Run.Metrics(), DrainedTools: drained}}})
	if err != nil {
		return zero, runs.RecoveryUnavailable, err
	}
	if err := validateRestoreWorkspace(continuation); err != nil {
		if errors.Is(err, runs.ErrExecutorStateLost) {
			return zero, runs.RecoveryUnavailable, nil
		}
		return zero, runs.RecoveryUnavailable, err
	}
	available, err := i.originalSourcesAvailable(ctx, checkpoint, rootID, proofs)
	if err != nil || !available {
		return zero, runs.RecoveryUnavailable, err
	}
	session, err := i.assembleInteraction(ctx, runs.ExecutorRef{SessionID: continuation.SessionID, ExecutorID: continuation.ExecutorID}, checkpoint.restoredStart(continuation), &checkpoint)
	if err != nil {
		return zero, runs.RecoveryUnavailable, err
	}
	defer func() { resultErr = errors.Join(resultErr, i.discardInteraction(session)) }()
	if pendingCheckpointSources(session, checkpoint) {
		return zero, runs.RecoveryDeferred, nil
	}
	if err := i.verifySettledTools(ctx, session, active.Run.SessionID(), checkpoint, proofs, pendingCalls); err != nil {
		return zero, runs.RecoveryUnavailable, err
	}
	if err := session.validateCapturedToolArguments(proofs); err != nil {
		return zero, runs.RecoveryUnavailable, err
	}
	if err := validateRecovery(ctx, session.deployment, checkpoint.tree); err != nil {
		if errors.Is(err, scoperun.ErrInvalidLog) {
			return zero, runs.RecoveryUnavailable, nil
		}
		return zero, runs.RecoveryUnavailable, err
	}
	if _, err := restoreInteractionAccounting(checkpoint, map[scoperun.ProcessID]runs.ExecutionMember{rootID: continuation.Members[0]}); err != nil {
		return zero, runs.RecoveryUnavailable, err
	}
	return runs.UncertainExecutionPlan{Continuation: continuation, Tools: tools}, runs.RecoveryReady, nil
}

func (i *InteractionExecutor) StageUncertainExecution(ctx context.Context, continuation runs.ExecutionContinuation) (_ runs.ExecutorRef, err error) {
	if err := continuation.Validate(); err != nil {
		return runs.ExecutorRef{}, err
	}
	if !i.acceptsBuild(continuation.Checkpoint.BuildID) {
		return runs.ExecutorRef{}, runs.ErrExecutorStateLost
	}
	finish, err := i.sessions.beginAssembly()
	if err != nil {
		return runs.ExecutorRef{}, err
	}
	defer finish()
	checkpoint, err := decodeExecutorCheckpoint(continuation.Checkpoint)
	if err != nil {
		return runs.ExecutorRef{}, err
	}
	rootID, _ := scoperun.ParseProcessID(continuation.Checkpoint.RootMemberID)
	head, found, err := i.config.ExecutionTrees.LoadExecutionTree(ctx, continuation.SessionID, rootID.String())
	if err != nil || !found {
		return runs.ExecutorRef{}, errors.Join(errors.New("execution: recovery head is unavailable"), err)
	}
	checkpoint.tree, err = decodeExecutionTree(head, rootID)
	if err != nil {
		return runs.ExecutorRef{}, err
	}
	ref := runs.ExecutorRef{SessionID: continuation.SessionID, ExecutorID: continuation.ExecutorID}
	session, err := i.assembleInteraction(ctx, ref, checkpoint.restoredStart(continuation), &checkpoint)
	if err != nil {
		return runs.ExecutorRef{}, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, i.discardInteraction(session))
		}
	}()
	if err := validateRecovery(ctx, session.deployment, checkpoint.tree); err != nil {
		return runs.ExecutorRef{}, err
	}
	usage, err := restoreInteractionAccounting(checkpoint, map[scoperun.ProcessID]runs.ExecutionMember{rootID: continuation.Members[0]})
	if err != nil {
		return runs.ExecutorRef{}, err
	}
	session.accounting.restore(usage, checkpoint.contextByProcess)
	session.state.admittedProcessID = rootID
	session.state.toolMetadata, session.state.pendingSteers, session.state.pendingContinuation = checkpoint.toolMetadata, checkpoint.pendingSteers, checkpoint.pendingContinuation
	session.state.restoreTree = checkpoint.tree
	session.state.boundary = interactionBoundaryRecovery
	session.state.dispatchReady = make(chan struct{})
	if err := i.publishInteraction(ctx, session); err != nil {
		return runs.ExecutorRef{}, err
	}
	return ref, nil
}

func (i *InteractionExecutor) BeginUncertainExecution(ctx context.Context, ref runs.ExecutorRef) error {
	session, err := i.session(ref)
	if err != nil {
		return err
	}
	if !session.state.observerAttached() || !session.state.begin() {
		return errors.New("execution: recovery must be staged and observed")
	}
	process, err := session.engine.Recover(runExecutionContext(session.lifetime.execution, session.scope, session.start), session.deployment, session.state.restoreTree.Root)
	if err != nil {
		return fmt.Errorf("execution: restore uncertain tree: %w", err)
	}
	session.state.setProcess(process)
	session.state.mu.Lock()
	session.state.restoreTree = scoperun.Log{}
	session.state.mu.Unlock()
	session.reportUnknownEffects()
	session.state.mu.Lock()
	session.state.continueExecution()
	session.state.mu.Unlock()
	session.startWorkers()
	return nil
}

func (i interactionCheckpointState) recoverableReceipts(rootID scoperun.ProcessID) (map[scoperun.EffectID]toolResultMetadata, error) {
	proofs := make(map[scoperun.EffectID]toolResultMetadata)
	for _, metadata := range i.toolMetadata {
		if metadata.Receipt == nil {
			continue
		}
		if metadata.MemberID != rootID.String() {
			return nil, nil
		}
		id, err := scoperun.ParseEffectID(metadata.Receipt.EffectID)
		if err != nil {
			return nil, fmt.Errorf("execution: retained tool receipt: %w", err)
		}
		if _, duplicate := proofs[id]; duplicate {
			return nil, errors.New("execution: duplicate retained Tool effect identity")
		}
		proofs[id] = metadata
	}
	if len(proofs) == 0 {
		return nil, nil
	}
	inspection, err := i.tree.Inspect()
	if err != nil {
		return nil, err
	}
	root, found := inspection.Process(rootID)
	if !found || (root.Status != scoperun.StatusWaiting && root.Status != scoperun.StatusDraining && !root.Status.Terminal()) {
		return nil, nil
	}
	for _, process := range inspection.Processes {
		if len(process.Waits) > 0 {
			return nil, nil
		}
		for _, id := range unresolvedProcessEffects(process) {
			if _, supported := proofs[id]; !supported || process.Status.Terminal() {
				return nil, nil
			}
		}
	}
	return proofs, nil
}

func (i *InteractionExecutor) verifySettledTools(ctx context.Context, session *interactionSession, sessionID string, checkpoint interactionCheckpointState, proofs map[scoperun.EffectID]toolResultMetadata, pendingCalls map[string]bool) error {
	var rounds []interaction.RoundResults
	for _, metadata := range proofs {
		if pendingCalls[metadata.Start.CallID] {
			continue
		}
		if rounds == nil {
			var err error
			rounds, err = session.settledToolResults(checkpoint.tree)
			if err != nil {
				return err
			}
		}
		committed := false
		for _, round := range rounds {
			for _, entry := range round.Entries() {
				identity, err := logicalToolCallID(round.ProcessID(), round.ModelCallSequence(), entry.ToolCallIndex, entry.Call.ID, entry.Call.Name)
				if err != nil {
					return err
				}
				if identity.String() != metadata.Start.CallID {
					continue
				}
				publication, err := resultPublication(identity, entry)
				if err != nil {
					return err
				}
				committed, err = i.config.ExecutionTrees.ExecutionResultCommitted(ctx, sessionID, publication)
				if err != nil {
					return err
				}
			}
		}
		if !committed {
			return errors.New("execution: captured Tool has neither its original attempt nor a publication receipt")
		}
	}
	return nil
}

func (i *InteractionExecutor) originalSourcesAvailable(ctx context.Context, checkpoint interactionCheckpointState, rootID scoperun.ProcessID, proofs map[scoperun.EffectID]toolResultMetadata) (bool, error) {
	inspection, err := checkpoint.tree.Inspect()
	if err != nil {
		return false, err
	}
	for _, metadata := range proofs {
		proof := metadata.Receipt
		processID, err := scoperun.ParseProcessID(proof.ProcessID)
		if err != nil {
			return false, fmt.Errorf("execution: retained tool receipt: %w", err)
		}
		effectID, err := scoperun.ParseEffectID(proof.EffectID)
		if err != nil {
			return false, fmt.Errorf("execution: retained tool receipt: %w", err)
		}
		process, found := inspection.Process(processID)
		if !found {
			return false, errors.New("execution: retained Tool process is missing")
		}
		for _, pending := range process.PendingEffects {
			if pending.Request.ID() != effectID {
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
			if reference.process != rootID || reference.sequence != metadata.Start.ModelCallSequence || reference.index != metadata.Start.ToolCallIndex || call.ID != metadata.Start.SourceCallID || call.Name != metadata.Start.ToolName {
				return false, errors.New("execution: retained request differs from its captured attribution")
			}
		}
		activation, err := originalToolActivation(*proof, checkpoint.bindings)
		if err != nil {
			return false, err
		}
		available, err := i.config.OriginalToolResults.AwaitOriginalToolSource(ctx, proof.Tool, proof.Release, proof.Authority, activation)
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

func (i *interactionSession) validateCapturedToolArguments(proofs map[scoperun.EffectID]toolResultMetadata) error {
	for _, metadata := range proofs {
		var owner *observedInteractionTool
		for _, tools := range i.state.deployments.observedTools {
			if candidate := tools[metadata.Receipt.Tool]; candidate != nil {
				owner = candidate
				break
			}
		}
		if owner == nil {
			return errors.New("execution: captured Tool owner is unavailable")
		}
		if _, err := owner.binding.Contract().Prepare(toolcontract.Request{Name: metadata.Start.ToolName, Arguments: metadata.Start.Arguments}, toolcontract.Admission{}); err != nil {
			return fmt.Errorf("execution: validate captured effective arguments: %w", err)
		}
	}
	return nil
}
