package execution

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
)

func (i *interactionSession) ActivateTree(ctx context.Context, activation agent.TreeActivation) error {
	digest, err := activation.ContentDigest()
	if err != nil {
		return err
	}
	return i.commitTree(ctx, activation.TreeSnapshot(), runs.ExecutionTreeUpdate{
		PreviousWriter: activation.PreviousIncarnationID().String(), PreviousDigest: activation.PreviousTreeDigest().String(),
		Head: runs.ExecutionTreeHead{CommitID: activation.Identity(), CommitDigest: digest.String()},
	})
}

func (i *interactionSession) CommitEffect(ctx context.Context, boundary agent.EffectBoundary) error {
	digest, err := boundary.ContentDigest()
	if err != nil {
		return err
	}
	return i.commitTree(ctx, boundary.TreeSnapshot(), runs.ExecutionTreeUpdate{
		PreviousWriter: boundary.TreeSnapshot().IncarnationID().String(), PreviousDigest: boundary.PreviousTreeDigest().String(),
		Head: runs.ExecutionTreeHead{
			Sequence: boundary.Sequence(), CommitID: boundary.Identity(), CommitDigest: digest.String(),
		},
	})
}

func (i *interactionSession) CommitCheckpoint(ctx context.Context, checkpoint agent.TreeCheckpoint) error {
	digest, err := checkpoint.ContentDigest()
	if err != nil {
		return err
	}
	update := runs.ExecutionTreeUpdate{
		PreviousWriter: checkpoint.TreeSnapshot().IncarnationID().String(), PreviousDigest: checkpoint.PreviousTreeDigest().String(),
		Head: runs.ExecutionTreeHead{
			Sequence: checkpoint.Sequence(), CommitID: checkpoint.Identity(), CommitDigest: digest.String(),
		},
	}
	if checkpoint.Kind() == agent.TreeCheckpointKindStart {
		update.PreviousWriter, update.PreviousDigest = "", ""
	}
	return i.commitTree(ctx, checkpoint.TreeSnapshot(), update)
}

func (i *interactionSession) commitTree(ctx context.Context, tree agent.TreeSnapshot, update runs.ExecutionTreeUpdate) error {
	ctx, cancel := i.lifetime.treeCommitContext(ctx)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	// Scope owns the boundary identity and digest; Runtime owns the atomic product write.
	update.Head.SessionID = i.start.SessionID
	update.Head.RootID = tree.RootID().String()
	update.Head.Writer = tree.IncarnationID().String()
	update.Head.Digest = tree.Digest().String()
	update.Head.Payload = tree.JSON()

	if i.executionTrees == nil {
		return errors.New("execution: execution tree store is required")
	}
	rounds, err := interaction.SettledResults(tree)
	if err != nil {
		return err
	}
	fact := runs.ExecutionTreeSettled{Update: update}
	i.state.mu.Lock()
	hasChildren := len(i.state.delegateChildren) > 0
	i.state.mu.Unlock()
	if hasChildren {
		i.childProjection.lock()
		defer i.childProjection.unlock()
	}
	var terminalChildren []*managedDelegateCall
	var retired []runtimeidentity.EffectID
	if i.state.observerAttached() {
		facts, children, err := i.childTerminalFacts(tree)
		if err != nil {
			return err
		}
		fact.Facts, terminalChildren = facts, children
		for _, round := range rounds {
			roundStart := len(fact.Facts)
			for _, entry := range round.Entries() {
				identity, err := logicalToolCallID(round.Relation().ProcessID(), round.ModelCallSequence(), entry.ToolCallIndex, entry.Call.ID, entry.Call.Name)
				if err != nil {
					return err
				}
				// Durable receipts precede metadata lookup: overlapping cuts may
				// contain results whose presentation data was already retired.
				publication, err := resultPublication(identity, entry)
				if err != nil {
					return err
				}
				committed, err := i.executionTrees.ExecutionResultCommitted(ctx, i.start.SessionID, publication)
				if err != nil {
					return err
				}
				retired = append(retired, identity)
				if committed {
					continue
				}
				projection, err := i.toolResultProjection(round.Relation(), round.ModelCallSequence(), entry)
				if err != nil {
					return err
				}
				fact.Facts = append(fact.Facts, projection)
			}
			if round.Complete() && len(fact.Facts) > roundStart {
				index := len(fact.Facts) - 1
				completed := fact.Facts[index].Payload.(runs.ToolResultsCommitted)
				for _, entry := range round.Entries() {
					completed.ModelResults = append(completed.ModelResults, entry.Result.Clone())
				}
				fact.Facts[index].Payload = completed
			}
		}
	}
	parents := capturedParents(tree)
	depth := func(member runs.ExecutorMember) int {
		id, _ := agent.ParseProcessID(member.MemberID)
		count := 0
		for parent, found := parents[id]; found; parent, found = parents[parent] {
			count++
		}
		return count
	}
	// Own results precede the child's terminal, which precedes its parent's result.
	slices.SortStableFunc(fact.Facts, func(a, b runs.ExecutorEvent) int {
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
	if len(fact.Facts) == 0 {
		err = i.executionTrees.SaveExecutionTree(ctx, update)
		if err != nil {
			readCtx, stop := i.lifetime.treeCommitContext(ctx)
			head, found, readErr := i.executionTrees.LoadExecutionTree(readCtx, update.Head.SessionID, update.Head.RootID)
			stop()
			if readErr == nil && found && head.SameCommit(update.Head) {
				err = nil
			} else {
				err = errors.Join(err, readErr)
			}
		}
	} else {
		// The product owner owns the transaction after handoff. A storage deadline
		// must not abandon its receipt while that transaction can still commit.
		publication, stop := i.lifetime.publicationContext(ctx)
		err = i.commitFact(publication, runs.ExecutorMember{MemberID: tree.RootID().String()}, fact)
		stop()
	}
	if err != nil {
		switch {
		case errors.Is(err, runs.ErrExecutionTreeCommitConflict):
			err = errors.Join(agent.ErrCommitConflict, err)
		case errors.Is(err, runs.ErrExecutionTreeWriterConflict):
			err = errors.Join(agent.ErrTreeIncarnationConflict, err)
		}
		return fmt.Errorf("execution: commit execution tree: %w", err)
	}
	for _, child := range terminalChildren {
		child.mu.Lock()
		child.segmentProjected = true
		child.mu.Unlock()
		i.modelFailures.forget(child.childProcessID)
	}
	i.retirePublishedToolMetadata(retired)
	return nil
}

func (i *interactionSession) committedTree(ctx context.Context, rootID agent.ProcessID) (agent.TreeSnapshot, error) {
	head, found, err := i.executionTrees.LoadExecutionTree(ctx, i.start.SessionID, rootID.String())
	if err != nil {
		return agent.TreeSnapshot{}, err
	}
	if !found {
		return agent.TreeSnapshot{}, errors.New("execution: committed execution tree is missing")
	}
	return decodeExecutionTree(head, rootID)
}

func decodeExecutionTree(head runs.ExecutionTreeHead, rootID agent.ProcessID) (agent.TreeSnapshot, error) {
	tree, err := agent.ParseTreeSnapshot(head.Payload)
	if err != nil {
		return agent.TreeSnapshot{}, err
	}
	writer := tree.IncarnationID()
	if tree.RootID() != rootID || writer.String() != head.Writer || tree.Digest().String() != head.Digest {
		return agent.TreeSnapshot{}, errors.New("execution: execution tree storage identity mismatch")
	}
	return tree, nil
}
