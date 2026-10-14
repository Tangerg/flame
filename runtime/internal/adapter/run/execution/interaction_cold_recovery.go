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
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
	corechat "github.com/Tangerg/scope/core/chat"
)

func (i *InteractionExecutor) PrepareUncertainExecution(ctx context.Context, active runs.ActiveExecution) (_ runs.UncertainExecutionPlan, _ bool, resultErr error) {
	var zero runs.UncertainExecutionPlan
	if active.Run.State() != run.Running || !active.Run.Lineage().IsRoot() || active.Run.SessionID() != active.Session.ID() || !i.acceptsBuild(active.Context.BuildID) || len(active.Context.Installations) == 0 {
		return zero, false, nil
	}
	if err := active.Context.ValidateOwnership(active.Context.RootMemberID, active.Run.SessionID()); err != nil {
		return zero, false, err
	}
	checkpoint, err := decodeExecutorCheckpoint(active.Context)
	if err != nil {
		return zero, false, err
	}
	rootID, err := agent.ParseProcessID(active.Context.RootMemberID)
	if err != nil {
		return zero, false, err
	}
	if checkpoint.handoff.rootID != rootID {
		return zero, false, errors.New("execution: context root differs from its owner")
	}
	head, found, err := i.config.ExecutionTrees.LoadExecutionTree(ctx, active.Run.SessionID(), rootID.String())
	if err != nil {
		return zero, false, err
	}
	if !found {
		return zero, false, errors.New("execution: retained context has no acknowledged tree")
	}
	checkpoint.tree, err = decodeExecutionTree(head, rootID)
	if err != nil {
		return zero, false, err
	}
	proofs := make(map[agent.EffectID]toolResultMetadata)
	for _, metadata := range checkpoint.toolMetadata {
		if metadata.Create == nil {
			continue
		}
		if metadata.MemberID != rootID.String() {
			return zero, false, nil
		}
		id, _ := agent.ParseEffectID(metadata.Create.EffectID)
		proofs[id] = metadata
	}
	if len(proofs) == 0 {
		return zero, false, nil
	}
	if i.config.OriginalScheduleResults == nil {
		return zero, false, nil
	}
	rootWaiting := false
	for _, snapshot := range checkpoint.tree.ProcessSnapshots() {
		if snapshot.Relation().ProcessID() == rootID {
			rootWaiting = snapshot.Status() == agent.StatusWaiting || snapshot.Status().Terminal()
		}
		for _, id := range snapshot.UnknownEffectIDs() {
			if _, supported := proofs[id]; !supported || snapshot.Status().Terminal() {
				return zero, false, nil
			}
		}
	}
	if !rootWaiting {
		return zero, false, nil
	}
	inputs, err := interaction.PendingToolInputs(checkpoint.tree)
	if err != nil {
		return zero, false, err
	}
	if len(inputs) > 0 {
		return zero, false, nil
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
		if !found || metadata.Create == nil {
			return zero, false, nil
		}
		item, found := items[invocation.ItemID]
		if !found || item.RunID() != active.Run.ID() || item.SessionID() != active.Run.SessionID() || item.Status() != transcript.ItemRunning {
			return zero, false, errors.New("execution: captured Tool Item is unavailable")
		}
		original, found := item.ToolInvocation()
		if !found || original.Name != metadata.Start.ToolName {
			return zero, false, errors.New("execution: captured Tool attribution differs from the original Item")
		}
		drained = append(drained, runs.DrainedTool{ItemID: item.ID(), CallID: invocation.CallID, SourceCallID: metadata.Start.SourceCallID})
		tools = append(tools, runs.RecoveredTool{Start: metadata.Start, Invocation: invocation})
		pendingCalls[invocation.CallID] = true
	}
	var rounds []interaction.RoundResults
	for _, metadata := range proofs {
		if pendingCalls[metadata.Start.CallID] {
			continue
		}
		if rounds == nil {
			rounds, err = interaction.SettledResults(checkpoint.tree)
			if err != nil {
				return zero, false, err
			}
		}
		committed := false
		for _, round := range rounds {
			for _, entry := range round.Entries() {
				identity, err := logicalToolCallID(round.Relation().ProcessID(), round.ModelCallSequence(), entry.ToolCallIndex, entry.Call.ID, entry.Call.Name)
				if err != nil {
					return zero, false, err
				}
				if identity.String() != metadata.Start.CallID {
					continue
				}
				publication, err := resultPublication(identity, entry)
				if err != nil {
					return zero, false, err
				}
				committed, err = i.config.ExecutionTrees.ExecutionResultCommitted(ctx, active.Run.SessionID(), publication)
				if err != nil {
					return zero, false, err
				}
			}
		}
		if !committed {
			return zero, false, errors.New("execution: captured Tool has neither its original attempt nor a publication receipt")
		}
	}
	slices.SortFunc(drained, func(a, b runs.DrainedTool) int { return strings.Compare(a.CallID, b.CallID) })
	continuation, err := runs.NewExecutionContinuation(runs.ExecutionContinuation{SessionID: active.Run.SessionID(), ExecutorID: "exec_recovery_" + active.Run.ID(), RootRunID: active.Run.ID(), Checkpoint: active.Context.Clone(), Capabilities: active.Run.Capabilities(), GoalIncarnationID: active.Run.GoalIncarnationID(), Workspace: active.Session.Workspace().Path(), Isolated: active.Session.Isolated(), Members: []runs.ExecutionMember{{RunID: active.Run.ID(), MemberID: rootID.String(), ModelSelection: active.Run.ModelSelection(), Metrics: active.Run.Metrics(), DrainedTools: drained}}})
	if err != nil {
		return zero, false, err
	}
	if err := validateRestoreWorkspace(continuation); err != nil {
		if errors.Is(err, runs.ErrExecutorStateLost) {
			return zero, false, nil
		}
		return zero, false, err
	}
	for _, metadata := range proofs {
		proof := metadata.Create
		processID, _ := agent.ParseProcessID(proof.ProcessID)
		effectID, _ := agent.ParseEffectID(proof.EffectID)
		if request, found := checkpoint.tree.EffectRequest(processID, effectID); found {
			invocation, err := interaction.ToolInvocationFromRequest(request)
			if err != nil {
				return zero, false, err
			}
			reference, found := invocation.Reference()
			call := invocation.ToolCall()
			if !found || reference.ProcessID() != rootID || reference.ModelCallSequence() != metadata.Start.ModelCallSequence || reference.ToolCallIndex() != metadata.Start.ToolCallIndex || call.ID != metadata.Start.SourceCallID || call.Name != metadata.Start.ToolName {
				return zero, false, errors.New("execution: retained request differs from its captured attribution")
			}
		}
		available, err := i.config.OriginalScheduleResults.AwaitOriginalCreateSource(ctx, proof.Tool, proof.Release, proof.Authority)
		if err != nil {
			return zero, false, err
		}
		// The ownership sweep can close the lost Core Run without consuming or
		// replaying the unavailable plugin's original private receipt.
		if !available {
			return zero, false, nil
		}
	}
	session, err := i.assembleInteraction(ctx, runs.ExecutorRef{SessionID: continuation.SessionID, ExecutorID: continuation.ExecutorID}, checkpoint.restoredStart(continuation), &checkpoint.modelObservers, &checkpoint.modelContexts, &checkpoint.modelInputs)
	if err != nil {
		return zero, false, err
	}
	defer func() { resultErr = errors.Join(resultErr, i.discardInteraction(session)) }()
	if pendingCheckpointSources(session, checkpoint) {
		return zero, false, errors.New("execution: captured MCP source is still connecting")
	}
	for _, metadata := range proofs {
		var owner *observedInteractionTool
		for _, tools := range session.state.deployments.observedTools {
			if candidate := tools[metadata.Create.Tool]; candidate != nil {
				owner = candidate
				break
			}
		}
		if owner == nil {
			return zero, false, errors.New("execution: captured Tool owner is unavailable")
		}
		if _, err := owner.binding.Contract().Prepare(corechat.ToolCall{ID: metadata.Start.SourceCallID, Name: metadata.Start.ToolName, Arguments: metadata.Start.Arguments}); err != nil {
			return zero, false, fmt.Errorf("execution: validate captured effective arguments: %w", err)
		}
	}
	if err := session.engine.ValidateRestorableTree(ctx, session.deployment, checkpoint.tree); err != nil {
		if errors.Is(err, agent.ErrInvalidTreeSnapshot) {
			return zero, false, nil
		}
		return zero, false, err
	}
	if _, err := restoreInteractionAccounting(checkpoint, map[agent.ProcessID]runs.ExecutionMember{rootID: continuation.Members[0]}); err != nil {
		return zero, false, err
	}
	return runs.UncertainExecutionPlan{Continuation: continuation, Tools: tools}, true, nil
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
	assembly, abandon := i.installations.Begin()
	defer abandon()
	checkpoint, err := decodeExecutorCheckpoint(continuation.Checkpoint)
	if err != nil {
		return runs.ExecutorRef{}, err
	}
	rootID, _ := agent.ParseProcessID(continuation.Checkpoint.RootMemberID)
	head, found, err := i.config.ExecutionTrees.LoadExecutionTree(ctx, continuation.SessionID, rootID.String())
	if err != nil || !found {
		return runs.ExecutorRef{}, errors.Join(errors.New("execution: recovery head is unavailable"), err)
	}
	checkpoint.tree, err = decodeExecutionTree(head, rootID)
	if err != nil {
		return runs.ExecutorRef{}, err
	}
	ref := runs.ExecutorRef{SessionID: continuation.SessionID, ExecutorID: continuation.ExecutorID}
	session, err := i.assembleInteraction(ctx, ref, checkpoint.restoredStart(continuation), &checkpoint.modelObservers, &checkpoint.modelContexts, &checkpoint.modelInputs)
	if err != nil {
		return runs.ExecutorRef{}, err
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, i.discardInteraction(session))
		}
	}()
	if err := session.engine.ValidateRestorableTree(ctx, session.deployment, checkpoint.tree); err != nil {
		return runs.ExecutorRef{}, err
	}
	usage, err := restoreInteractionAccounting(checkpoint, map[agent.ProcessID]runs.ExecutionMember{rootID: continuation.Members[0]})
	if err != nil {
		return runs.ExecutorRef{}, err
	}
	session.accounting.restore(usage, checkpoint.contextByProcess)
	session.state.toolMetadata, session.state.pendingSteers, session.state.pendingContinuation = checkpoint.toolMetadata, checkpoint.pendingSteers, checkpoint.pendingContinuation
	session.state.restoreTree = checkpoint.tree
	session.state.boundary = interactionBoundaryRecovery
	session.state.dispatchReady = make(chan struct{})
	if err := i.installations.Publish(ctx, assembly, session.installationDependencies(), func() error { return i.sessions.register(session) }); err != nil {
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
	process, err := session.engine.RestoreTree(runExecutionContext(session.lifetime.execution, session.scope, session.start), session.deployment, session.state.restoreTree)
	if err != nil {
		return fmt.Errorf("execution: restore uncertain tree: %w", err)
	}
	session.state.setProcess(process)
	session.reportUnknownEffects()
	session.state.mu.Lock()
	session.state.restoreTree = agent.TreeSnapshot{}
	session.state.continueExecution()
	session.state.mu.Unlock()
	session.startWorkers()
	return nil
}
