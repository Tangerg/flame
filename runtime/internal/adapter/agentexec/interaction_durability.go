package agentexec

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
)

const executionTreeCommitTimeout = 30 * time.Second

func (i *interactionSession) ActivateTree(ctx context.Context, activation agent.TreeActivation) error {
	return i.commitTree(ctx, activation.TreeSnapshot(), activation.PreviousIncarnationID().String(), activation.PreviousTreeDigest().String())
}

func (i *interactionSession) CommitEffect(ctx context.Context, boundary agent.EffectBoundary) error {
	writer, _ := boundary.TreeSnapshot().IncarnationID()
	return i.commitTree(ctx, boundary.TreeSnapshot(), writer.String(), boundary.PreviousTreeDigest().String())
}

func (i *interactionSession) CommitCheckpoint(ctx context.Context, checkpoint agent.TreeCheckpoint) error {
	writer, _ := checkpoint.TreeSnapshot().IncarnationID()
	previousWriter, previousDigest := writer.String(), checkpoint.PreviousTreeDigest().String()
	if checkpoint.Kind() == agent.TreeCheckpointKindStart {
		previousWriter, previousDigest = "", ""
	}
	return i.commitTree(ctx, checkpoint.TreeSnapshot(), previousWriter, previousDigest)
}

func (i *interactionSession) commitTree(ctx context.Context, tree agent.TreeSnapshot, previousWriter, previousDigest string) error {
	ctx, cancel := i.lifetime.publicationContext(ctx)
	defer cancel()
	ctx, timeout := context.WithTimeout(ctx, executionTreeCommitTimeout)
	defer timeout()
	if err := ctx.Err(); err != nil {
		return err
	}
	writer, _ := tree.IncarnationID()
	update := runs.ExecutionTreeUpdate{PreviousWriter: previousWriter, PreviousDigest: previousDigest,
		Head: runs.ExecutionTreeHead{SessionID: i.start.SessionID, RootID: tree.RootID().String(), Writer: writer.String(), Digest: tree.Digest().String(), Payload: tree.JSON()}}
	if i.executionTrees == nil {
		return errors.New("agentexec: execution tree store is required")
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
			case runs.AssistantMessageCompleted:
				return 1
			default:
				return 2
			}
		}
		return rank(a.Payload) - rank(b.Payload)
	})
	if len(fact.Facts) == 0 {
		err = i.executionTrees.SaveExecutionTree(ctx, update)
		if err != nil {
			readCtx, stop := i.lifetime.publicationContext(context.WithoutCancel(ctx))
			readCtx, deadline := context.WithTimeout(readCtx, executionTreeCommitTimeout)
			head, found, readErr := i.executionTrees.LoadExecutionTree(readCtx, update.Head.SessionID, update.Head.RootID)
			deadline()
			stop()
			if readErr == nil && found && head.Writer == update.Head.Writer && head.Digest == update.Head.Digest {
				err = nil
			} else {
				err = errors.Join(err, readErr)
			}
		}
	} else {
		err = i.commitFact(ctx, runs.ExecutorMember{MemberID: tree.RootID().String()}, fact)
	}
	if err != nil {
		return fmt.Errorf("agentexec: commit execution tree: %w", err)
	}
	for _, child := range terminalChildren {
		child.mu.Lock()
		child.segmentProjected, child.assistantProjected = true, true
		child.mu.Unlock()
		i.modelFailures.forget(child.childProcessID)
		i.committedReplies.forget(child.childProcessID)
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
		return agent.TreeSnapshot{}, errors.New("agentexec: committed execution tree is missing")
	}
	return decodeExecutionTree(head, rootID)
}

func decodeExecutionTree(head runs.ExecutionTreeHead, rootID agent.ProcessID) (agent.TreeSnapshot, error) {
	tree, err := agent.ParseTreeSnapshot(head.Payload)
	if err != nil {
		return agent.TreeSnapshot{}, err
	}
	writer, durable := tree.IncarnationID()
	if !durable || tree.RootID() != rootID || writer.String() != head.Writer || tree.Digest().String() != head.Digest {
		return agent.TreeSnapshot{}, errors.New("agentexec: execution tree storage identity mismatch")
	}
	return tree, nil
}
