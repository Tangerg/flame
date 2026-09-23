package agentexec

import (
	"context"
	json "encoding/json/v2"
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

type treeCommit struct {
	PreviousWriter string
	PreviousDigest string
	Sequence       uint64
	Kind           string
	ID             string
}

func (i *interactionSession) ActivateTree(ctx context.Context, activation agent.TreeActivation) error {
	return i.commitTree(ctx, activation.TreeSnapshot(), treeCommit{
		PreviousWriter: activation.PreviousIncarnationID().String(), PreviousDigest: activation.PreviousTreeDigest().String(),
		Kind: "activation", ID: fmt.Sprintf("activation:%q", activation.IncarnationID().String()),
	})
}

func (i *interactionSession) CommitEffect(ctx context.Context, boundary agent.EffectBoundary) error {
	return i.commitTree(ctx, boundary.TreeSnapshot(), treeCommit{
		PreviousWriter: boundary.TreeSnapshot().IncarnationID().String(), PreviousDigest: boundary.PreviousTreeDigest().String(),
		Sequence: boundary.Sequence(), Kind: boundary.Kind().String(),
		ID: fmt.Sprintf("effect:%q:%s", boundary.Request().ID().String(), boundary.Kind()),
	})
}

func (i *interactionSession) CommitCheckpoint(ctx context.Context, checkpoint agent.TreeCheckpoint) error {
	writer := checkpoint.TreeSnapshot().IncarnationID()
	commit := treeCommit{
		PreviousWriter: writer.String(), PreviousDigest: checkpoint.PreviousTreeDigest().String(),
		Sequence: checkpoint.Sequence(), Kind: checkpoint.Kind().String(),
		ID: fmt.Sprintf("checkpoint:%q:%d", writer.String(), checkpoint.Sequence()),
	}
	if checkpoint.Kind() == agent.TreeCheckpointKindStart {
		commit.PreviousWriter, commit.PreviousDigest = "", ""
	}
	return i.commitTree(ctx, checkpoint.TreeSnapshot(), commit)
}

func (i *interactionSession) commitTree(ctx context.Context, tree agent.TreeSnapshot, commit treeCommit) error {
	ctx, cancel := i.lifetime.publicationContext(ctx)
	defer cancel()
	ctx, timeout := context.WithTimeout(ctx, executionTreeCommitTimeout)
	defer timeout()
	if err := ctx.Err(); err != nil {
		return err
	}
	// The validated snapshot contains the frozen Effect request and settlement;
	// the envelope also binds commit kind and predecessor, which snapshots omit.
	content, err := json.Marshal(struct {
		treeCommit
		Snapshot string
	}{commit, tree.Digest().String()})
	if err != nil {
		return err
	}
	update := runs.ExecutionTreeUpdate{
		PreviousWriter: commit.PreviousWriter, PreviousDigest: commit.PreviousDigest,
		Head: runs.ExecutionTreeHead{
			SessionID: i.start.SessionID, RootID: tree.RootID().String(), Writer: tree.IncarnationID().String(),
			Sequence: commit.Sequence, CommitID: commit.ID, CommitDigest: agent.ComputeDigest(content).String(),
			Digest: tree.Digest().String(), Payload: tree.JSON(),
		},
	}

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
			default:
				return 1
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
			if readErr == nil && found && head.SameCommit(update.Head) {
				err = nil
			} else {
				err = errors.Join(err, readErr)
			}
		}
	} else {
		err = i.commitFact(ctx, runs.ExecutorMember{MemberID: tree.RootID().String()}, fact)
	}
	if err != nil {
		switch {
		case errors.Is(err, runs.ErrExecutionTreeCommitConflict):
			err = errors.Join(agent.ErrCommitConflict, err)
		case errors.Is(err, runs.ErrExecutionTreeWriterConflict):
			err = errors.Join(agent.ErrTreeIncarnationConflict, err)
		}
		return fmt.Errorf("agentexec: commit execution tree: %w", err)
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
		return agent.TreeSnapshot{}, errors.New("agentexec: committed execution tree is missing")
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
		return agent.TreeSnapshot{}, errors.New("agentexec: execution tree storage identity mismatch")
	}
	return tree, nil
}
