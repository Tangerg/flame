package execution

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"iter"
	"slices"
	"strconv"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"

	"github.com/Tangerg/scope/agent/interaction"
	scoperun "github.com/Tangerg/scope/agent/run"
)

func (i *interactionSession) Append(ctx context.Context, batch scoperun.Batch) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := batch.Validate(); err != nil {
		return err
	}
	head, found, err := i.executionTrees.LoadExecutionTree(ctx, i.start.SessionID, batch.Root.String())
	if err != nil {
		return err
	}
	history := scoperun.Log{Root: batch.Root}
	if found {
		history, err = i.logCache.decode(head, batch.Root)
		if err != nil {
			return err
		}
	}
	var existing []scoperun.Entry
	if batch.After < history.Head() {
		end := min(uint64(batch.After)+uint64(len(batch.Records)), uint64(len(history.Entries)))
		existing = history.Entries[uint64(batch.After):end]
	}
	_, retry, err := batch.Admit(history.Epoch(), history.Head(), existing)
	if err != nil || retry {
		return err
	}
	history, entriesJSON, err := i.appendedHistory(head, found, history, batch)
	if err != nil {
		return err
	}
	update, err := i.treeUpdate(head, found, history, batch, entriesJSON)
	if err != nil {
		return err
	}
	if err := i.commitTree(ctx, history, update, batch.Records); err != nil {
		return err
	}
	i.logCache.remember(update.Head, history, entriesJSON)
	if err := i.material.acknowledge(history); err != nil {
		return err
	}
	i.lifetime.wakeState()
	i.lifetime.wakeUnknown()
	return nil
}

func (i *interactionSession) appendedHistory(head runs.ExecutionTreeHead, found bool, history scoperun.Log, batch scoperun.Batch) (scoperun.Log, string, error) {
	entryJSON, err := json.Marshal(batch.Entries(), json.Deterministic(true))
	if err != nil {
		return scoperun.Log{}, "", err
	}
	var entries []scoperun.Entry
	if err := json.Unmarshal(entryJSON, &entries); err != nil {
		return scoperun.Log{}, "", err
	}
	priorJSON := "[]"
	if found {
		priorJSON, err = i.logCache.encodedEntries(head, history)
		if err != nil {
			return scoperun.Log{}, "", err
		}
	}
	history.Entries = append(slices.Clip(history.Entries), entries...)
	return history, appendEntryJSON(priorJSON, entryJSON), nil
}

func (i *interactionSession) treeUpdate(head runs.ExecutionTreeHead, found bool, history scoperun.Log, batch scoperun.Batch, entriesJSON string) (runs.ExecutionTreeUpdate, error) {
	material, err := i.material.snapshot(history)
	if err != nil {
		return runs.ExecutionTreeUpdate{}, err
	}
	payload, err := encodeExecutionLog(history.Root, entriesJSON, material)
	if err != nil {
		return runs.ExecutionTreeUpdate{}, err
	}
	intent, err := json.Marshal(batch, json.Deterministic(true))
	if err != nil {
		return runs.ExecutionTreeUpdate{}, err
	}
	digest := scoperun.ComputeDigest(intent).String()
	update := runs.ExecutionTreeUpdate{Head: runs.ExecutionTreeHead{
		SessionID: i.start.SessionID, RootID: batch.Root.String(),
		Writer: strconv.FormatUint(uint64(history.Epoch()), 10), Sequence: uint64(history.Head()),
		Digest: scoperun.ComputeDigest(payload).String(), Payload: payload,
		CommitID: digest, CommitDigest: digest,
	}}
	if found {
		update.PreviousWriter, update.PreviousDigest = head.Writer, head.Digest
	}
	return update, nil
}

func (i *interactionSession) Load(ctx context.Context, root scoperun.ProcessID) (scoperun.Log, bool, error) {
	if err := ctx.Err(); err != nil {
		return scoperun.Log{}, false, err
	}
	if !root.Valid() || root.Depth() != 0 {
		return scoperun.Log{}, false, scoperun.ErrInvalidIdentity
	}
	head, found, err := i.executionTrees.LoadExecutionTree(ctx, i.start.SessionID, root.String())
	if err != nil || !found {
		return scoperun.Log{}, found, err
	}
	history, err := i.logCache.decode(head, root)
	if err != nil {
		return scoperun.Log{}, false, err
	}
	i.state.mu.Lock()
	expected := i.state.restoreTree
	i.state.mu.Unlock()
	if expected.Root.Valid() {
		if expected.Root != history.Root || expected.Head() != history.Head() || expected.Epoch() != history.Epoch() {
			return scoperun.Log{}, false, scoperun.ErrEpochConflict
		}
		var stored executionLogWire
		if err := json.Unmarshal(head.Payload, &stored, json.RejectUnknownMembers(true)); err != nil {
			return scoperun.Log{}, false, err
		}
		if err := i.material.restore(stored.PreparedRequests); err != nil {
			return scoperun.Log{}, false, err
		}
	}
	for index := len(history.Entries) - 1; index >= 0; index-- {
		if _, checkpoint := history.Entries[index].Record.(scoperun.Checkpoint); checkpoint {
			history.Entries = history.Entries[index:]
			break
		}
	}
	history, err = history.Clone()
	return history, err == nil, err
}

func (i *interactionSession) Read(ctx context.Context, root scoperun.ProcessID, after scoperun.LSN) iter.Seq2[scoperun.Entry, error] {
	return func(yield func(scoperun.Entry, error) bool) {
		if !root.Valid() || root.Depth() != 0 {
			yield(scoperun.Entry{}, scoperun.ErrInvalidIdentity)
			return
		}
		head, found, err := i.executionTrees.LoadExecutionTree(ctx, i.start.SessionID, root.String())
		if err != nil {
			yield(scoperun.Entry{}, err)
			return
		}
		if !found {
			return
		}
		if after >= scoperun.LSN(head.Sequence) {
			return
		}
		history, err := i.logCache.decode(head, root)
		if err != nil {
			yield(scoperun.Entry{}, err)
			return
		}
		for _, entry := range history.Entries {
			if entry.LSN <= after {
				continue
			}
			if err := ctx.Err(); err != nil {
				yield(scoperun.Entry{}, err)
				return
			}
			cloned, err := entry.Clone()
			if !yield(cloned, err) || err != nil {
				return
			}
		}
	}
}

func (i *interactionSession) commitTree(ctx context.Context, tree scoperun.Log, update runs.ExecutionTreeUpdate, records []scoperun.Record) error {
	ctx, cancel := i.lifetime.publicationContext(ctx)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	rounds, err := i.changedToolResults(tree, records)
	if err != nil {
		return err
	}
	children, err := i.childPublications(records)
	if err != nil {
		return err
	}
	i.state.mu.Lock()
	hasChildren := len(i.state.delegateChildren) > 0
	contextAdvances := (len(i.state.pendingSteers) > 0 || i.state.deployments.hasToolReceipts()) && i.state.boundary != interactionBoundaryAccepted
	i.state.mu.Unlock()
	acceptances, err := i.admittedSteers(tree.Root, records)
	if err != nil {
		return err
	}
	if hasChildren {
		i.childProjection.lock()
		defer i.childProjection.unlock()
	}
	settled := settledTree{fact: runs.ExecutionTreeSettled{Update: update, Children: children, InputAcceptances: acceptances}}
	if i.state.observerAttached() {
		if err := i.observeSettlement(ctx, tree, records, rounds, contextAdvances, &settled); err != nil {
			return err
		}
	}
	orderExecutionFacts(tree, settled.fact.Facts)
	if err := i.persistSettledTree(ctx, tree.Root, update, settled.fact); err != nil {
		return fmt.Errorf("execution: commit execution tree: %w", scopeCommitConflict(err))
	}
	i.publishChildBindings(settled.fact.Children)
	for _, child := range settled.terminalChildren {
		child.mu.Lock()
		child.segmentProjected = true
		child.mu.Unlock()
		i.modelFailures.forget(child.childProcessID)
	}
	i.retirePublishedToolMetadata(settled.retired)
	return nil
}

// A storage deadline must not abandon a receipt that can still commit.
func (i *interactionSession) persistSettledTree(ctx context.Context, root scoperun.ProcessID, update runs.ExecutionTreeUpdate, fact runs.ExecutionTreeSettled) error {
	if fact.AdvancesHeadOnly() {
		return i.saveExecutionTree(ctx, update)
	}
	publication, stop := i.lifetime.publicationContext(ctx)
	defer stop()
	return i.commitFact(publication, runs.ExecutorMember{MemberID: root.String()}, fact)
}

type settledTree struct {
	fact             runs.ExecutionTreeSettled
	terminalChildren []*managedDelegateCall
	retired          []runs.InvocationID
}

func (i *interactionSession) observeSettlement(ctx context.Context, tree scoperun.Log, records []scoperun.Record, rounds []interaction.RoundResults, contextAdvances bool, settled *settledTree) error {
	if contextAdvances {
		checkpoint, err := i.executorCheckpoint(tree)
		if err != nil {
			return err
		}
		settled.fact.Context = &checkpoint
	}
	if containsProcessExit(records) {
		facts, children, err := i.terminalFacts(tree)
		if err != nil {
			return err
		}
		settled.fact.Facts, settled.terminalChildren = facts, children
	}
	retired, err := i.projectSettledToolResults(ctx, tree, rounds, &settled.fact)
	if err != nil {
		return err
	}
	settled.retired = retired
	return nil
}

func scopeCommitConflict(err error) error {
	switch {
	case errors.Is(err, runs.ErrExecutionTreeCommitConflict):
		return errors.Join(scoperun.ErrCommitConflict, err)
	case errors.Is(err, runs.ErrExecutionTreeWriterConflict):
		return errors.Join(scoperun.ErrEpochConflict, err)
	default:
		return err
	}
}

func (i *interactionSession) saveExecutionTree(ctx context.Context, update runs.ExecutionTreeUpdate) error {
	err := i.executionTrees.SaveExecutionTree(ctx, update)
	if err != nil {
		readCtx, stop := i.lifetime.publicationContext(ctx)
		head, found, readErr := i.executionTrees.LoadExecutionTree(readCtx, update.Head.SessionID, update.Head.RootID)
		stop()
		if readErr == nil && found && head.SameCommit(update.Head) {
			err = nil
		} else {
			err = errors.Join(err, readErr)
		}
	}
	return err
}

func containsProcessExit(records []scoperun.Record) bool {
	for _, record := range records {
		if _, exited := record.(scoperun.Exited); exited {
			return true
		}
	}
	return false
}

func (i *interactionSession) projectSettledToolResults(ctx context.Context, tree scoperun.Log, rounds []interaction.RoundResults, fact *runs.ExecutionTreeSettled) ([]runs.InvocationID, error) {
	var retired []runs.InvocationID
	for _, round := range rounds {
		projections, roundRetired, err := i.projectSettledRound(ctx, tree, round, fact)
		if err != nil {
			return nil, err
		}
		retired = append(retired, roundRetired...)
		if len(projections) > 0 && round.Complete() {
			last := &projections[len(projections)-1]
			for _, entry := range round.Entries() {
				last.committed.ModelResults = append(last.committed.ModelResults, entry.Result.Clone())
			}
		}
		for _, projection := range projections {
			fact.Facts = append(fact.Facts, projection.event())
		}
	}
	return retired, nil
}

func (i *interactionSession) projectSettledRound(ctx context.Context, tree scoperun.Log, round interaction.RoundResults, fact *runs.ExecutionTreeSettled) ([]toolResultFact, []runs.InvocationID, error) {
	var projections []toolResultFact
	var retired []runs.InvocationID
	for _, entry := range round.Entries() {
		identity, err := logicalToolCallID(round.ProcessID(), round.ModelCallSequence(), entry.ToolCallIndex, entry.Call.ID, entry.Call.Name)
		if err != nil {
			return nil, nil, err
		}
		// Durable receipts precede metadata lookup: overlapping cuts may
		// contain results whose presentation data was already retired.
		publication, err := resultPublication(identity, entry)
		if err != nil {
			return nil, nil, err
		}
		committed, err := i.executionTrees.ExecutionResultCommitted(ctx, i.start.SessionID, publication)
		if err != nil {
			return nil, nil, err
		}
		retired = append(retired, identity)
		if committed {
			continue
		}
		failedChild, failed, err := i.failedChildPublication(tree, round.ProcessID(), round.ModelCallSequence(), entry.Call)
		if err != nil {
			return nil, nil, err
		}
		if failed {
			fact.Children = append(fact.Children, failedChild)
		}
		projection, err := i.toolResultProjection(round.ProcessID(), round.ModelCallSequence(), entry)
		if err != nil {
			return nil, nil, err
		}
		projections = append(projections, projection)
	}
	return projections, retired, nil
}

func orderExecutionFacts(tree scoperun.Log, facts []runs.ExecutorEvent) {
	parents := capturedParents(tree)
	depth := func(member runs.ExecutorMember) int {
		id, _ := scoperun.ParseProcessID(member.MemberID)
		count := 0
		for parent, found := parents[id]; found; parent, found = parents[parent] {
			count++
		}
		return count
	}
	// Own results precede the child's terminal, which precedes its parent's result.
	slices.SortStableFunc(facts, func(a, b runs.ExecutorEvent) int {
		if difference := depth(b.Member) - depth(a.Member); difference != 0 {
			return difference
		}
		rank := func(payload any) int {
			switch payload.(type) {
			case runs.ToolResultsCommitted:
				return 0
			default:
				return 1
			}
		}
		return rank(a.Payload) - rank(b.Payload)
	})
}

func (i *interactionSession) toolResultsChanged(history scoperun.Log, records []scoperun.Record) (bool, error) {
	for _, record := range records {
		switch record := record.(type) {
		case scoperun.Exited:
			return true, nil
		case scoperun.EffectSettled:
			if record.Failure != nil || record.Outcome.Status() != scoperun.SettlementStatusSucceeded {
				continue
			}
			intent, found := declaredIntent(history, record.ID)
			if !found {
				continue
			}
			if intent.Effect.Target() != scoperun.EffectTargetDispatcher {
				return true, nil
			}
			program, found := capturedProgram(history, intent.ID.Process)
			if _, owned := i.state.deployments.interactions[program]; !found || !owned {
				continue
			}
			_, tool, err := interaction.ToolCallFromEffect(intent.Effect)
			if err != nil || tool {
				return tool, err
			}
		}
	}
	return false, nil
}

func declaredIntent(history scoperun.Log, id scoperun.EffectID) (scoperun.EffectIntent, bool) {
	for _, entry := range slices.Backward(history.Entries) {
		stepped, found := entry.Record.(scoperun.Stepped)
		if !found || stepped.Process != id.Process || stepped.Step != id.Step {
			continue
		}
		intents := stepped.Intents()
		if int(id.Index) >= len(intents) {
			return scoperun.EffectIntent{}, false
		}
		return intents[id.Index], true
	}
	return scoperun.EffectIntent{}, false
}

func capturedProgram(history scoperun.Log, process scoperun.ProcessID) (scoperun.ProgramRef, bool) {
	for _, entry := range history.Entries {
		switch record := entry.Record.(type) {
		case scoperun.Started:
			if record.Process == process {
				return record.Program, true
			}
		case scoperun.Checkpoint:
			for _, captured := range record.Processes {
				if captured.Started.Process == process {
					return captured.Started.Program, true
				}
			}
		}
	}
	return scoperun.ProgramRef{}, false
}

func (i *interactionSession) committedTree(ctx context.Context, rootID scoperun.ProcessID) (scoperun.Log, error) {
	head, found, err := i.executionTrees.LoadExecutionTree(ctx, i.start.SessionID, rootID.String())
	if err != nil {
		return scoperun.Log{}, err
	}
	if !found {
		return scoperun.Log{}, errors.New("execution: committed execution tree is missing")
	}
	return i.logCache.decode(head, rootID)
}

func decodeExecutionTree(head runs.ExecutionTreeHead, rootID scoperun.ProcessID) (scoperun.Log, error) {
	var stored executionLogWire
	if err := json.Unmarshal(head.Payload, &stored, json.RejectUnknownMembers(true)); err != nil {
		return scoperun.Log{}, err
	}
	history := stored.Log
	if err := history.Validate(); err != nil {
		return scoperun.Log{}, err
	}
	if history.Root != rootID || strconv.FormatUint(uint64(history.Epoch()), 10) != head.Writer || uint64(history.Head()) != head.Sequence || scoperun.ComputeDigest(head.Payload).String() != head.Digest {
		return scoperun.Log{}, errors.New("execution: execution log storage identity mismatch")
	}
	return history, nil
}

func (i *interactionSession) changedToolResults(tree scoperun.Log, records []scoperun.Record) ([]interaction.RoundResults, error) {
	changed, err := i.toolResultsChanged(tree, records)
	if err != nil || !changed {
		return nil, err
	}
	return i.settledToolResults(tree)
}

func (i *interactionSession) settledToolResults(history scoperun.Log) ([]interaction.RoundResults, error) {
	started := make(map[scoperun.ProgramRef]struct{})
	for _, entry := range history.Entries {
		switch record := entry.Record.(type) {
		case scoperun.Started:
			started[record.Program] = struct{}{}
		case scoperun.Checkpoint:
			for _, process := range record.Processes {
				started[process.Started.Program] = struct{}{}
			}
		}
	}
	var results []interaction.RoundResults
	for reference := range started {
		program, found := i.state.deployments.interactions[reference]
		if !found {
			continue
		}
		rounds, err := program.history.SettledResults(history)
		if err != nil {
			return nil, err
		}
		results = append(results, rounds...)
	}
	return results, nil
}

type executionLogWire struct {
	Log              scoperun.Log          `json:"log"`
	PreparedRequests []preparedRequestWire `json:"preparedRequests,omitempty"`
}
