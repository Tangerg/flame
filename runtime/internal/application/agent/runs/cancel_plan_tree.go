package runs

import (
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
)

func newCancellationPlan(
	targetRunID identity.RunID,
	runs []rundomain.Run,
	executor ExecutorRef,
	memberIDsByRunID map[identity.RunID]string,
	pending *Pending,
) (cancellationPlan, error) {
	runTree, err := buildCancellationRunTree(targetRunID, runs)
	if err != nil {
		return cancellationPlan{}, err
	}
	bindings, err := cancellationBindings(runTree.byRunID, memberIDsByRunID)
	if err != nil {
		return cancellationPlan{}, err
	}
	openRunIDs, err := runTree.openRunIDs(bindings)
	if err != nil {
		return cancellationPlan{}, err
	}
	if err := validateCancellationMemberBindings(runTree.byRunID, bindings); err != nil {
		return cancellationPlan{}, err
	}
	if err := validateCancellationPending(runTree.root, openRunIDs, bindings, pending); err != nil {
		return cancellationPlan{}, err
	}

	targetRunIDs, exists := runTree.topology.SubtreePostorder(targetRunID)
	if !exists {
		return cancellationPlan{}, fmt.Errorf(
			"runs: build cancellation plan: target Run %q is outside tree %q",
			targetRunID,
			runTree.root.ID(),
		)
	}
	targetRunIDSet := make(map[identity.RunID]struct{}, len(targetRunIDs))
	for _, runID := range targetRunIDs {
		targetRunIDSet[runID] = struct{}{}
	}
	plan := cancellationPlan{
		root:     bindings[runTree.root.ID()],
		target:   bindings[targetRunID],
		executor: executor,
		pending:  pending,
	}
	for _, runID := range runTree.topology.Postorder() {
		member := bindings[runID]
		if _, targeted := targetRunIDSet[runID]; targeted {
			plan.targetSubtree = append(plan.targetSubtree, member)
		} else {
			plan.survivingTree = append(plan.survivingTree, member)
		}
	}
	return plan, nil
}

type cancellationRunTree struct {
	root     rundomain.Run
	topology rundomain.Tree
	byRunID  map[identity.RunID]rundomain.Run
}

func buildCancellationRunTree(
	targetRunID identity.RunID,
	runs []rundomain.Run,
) (cancellationRunTree, error) {
	target, found := runByID(runs, targetRunID)
	if !found {
		return cancellationRunTree{}, fmt.Errorf(
			"runs: build cancellation plan: target Run %q is missing",
			targetRunID,
		)
	}
	rootRunID := target.Lineage().TreeRootID(target.ID())
	root, found := runByID(runs, rootRunID)
	if !found {
		return cancellationRunTree{}, fmt.Errorf(
			"runs: build cancellation plan: root Run %q is missing",
			rootRunID,
		)
	}

	byRunID := make(map[identity.RunID]rundomain.Run, len(runs))
	treeMembers := make([]rundomain.TreeMember, 0, len(runs))
	for _, run := range runs {
		if run.SessionID() != root.SessionID() {
			return cancellationRunTree{}, fmt.Errorf(
				"runs: build cancellation plan for tree %q: Run %q belongs to session %q, want %q",
				rootRunID,
				run.ID(),
				run.SessionID(),
				root.SessionID(),
			)
		}
		if _, duplicate := byRunID[run.ID()]; duplicate {
			return cancellationRunTree{}, fmt.Errorf(
				"runs: build cancellation plan for tree %q: duplicate Run %q",
				rootRunID,
				run.ID(),
			)
		}
		byRunID[run.ID()] = run
		treeMembers = append(treeMembers, rundomain.TreeMember{RunID: run.ID(), Lineage: run.Lineage()})
	}
	tree, err := rundomain.NewTree(rootRunID, treeMembers)
	if err != nil {
		return cancellationRunTree{}, fmt.Errorf("runs: build cancellation plan: %w", err)
	}
	runTree := cancellationRunTree{
		root:     root,
		topology: tree,
		byRunID:  byRunID,
	}
	if err := tree.CheckCancellation(targetRunID, byRunID); err != nil {
		return cancellationRunTree{}, fmt.Errorf("runs: build cancellation plan: %w", err)
	}
	return runTree, nil
}

func (c cancellationRunTree) openRunIDs(
	bindings map[identity.RunID]cancellationRun,
) ([]identity.RunID, error) {
	openRunIDs := make([]identity.RunID, 0, len(c.byRunID))
	for _, runID := range c.topology.Postorder() {
		run := c.byRunID[runID]
		if run.State().IsTerminal() {
			continue
		}
		openRunIDs = append(openRunIDs, runID)
		binding := bindings[runID]
		if run.Lineage().IsChild() && !binding.hasMember {
			return nil, fmt.Errorf(
				"runs: build cancellation plan: non-terminal child Run %q has no executor binding",
				runID,
			)
		}
	}
	return openRunIDs, nil
}

func cancellationBindings(
	runs map[identity.RunID]rundomain.Run,
	memberIDsByRunID map[identity.RunID]string,
) (map[identity.RunID]cancellationRun, error) {
	bindings := make(map[identity.RunID]cancellationRun, len(runs))
	for runID, run := range runs {
		bindings[runID] = cancellationRun{run: run}
	}
	memberOwners := make(map[string]identity.RunID, len(memberIDsByRunID))
	for runID, memberID := range memberIDsByRunID {
		member, exists := bindings[runID]
		if !exists {
			return nil, fmt.Errorf(
				"runs: build cancellation plan: executor binding names unknown Run %q",
				runID,
			)
		}
		if memberID == "" {
			return nil, fmt.Errorf(
				"runs: build cancellation plan: Run %q executor binding has no member id",
				runID,
			)
		}
		if owner, duplicate := memberOwners[memberID]; duplicate {
			return nil, fmt.Errorf(
				"runs: build cancellation plan: member %q is bound to Runs %q and %q",
				memberID,
				owner,
				runID,
			)
		}
		memberOwners[memberID] = runID
		member.memberID = memberID
		member.hasMember = true
		bindings[runID] = member
	}
	return bindings, nil
}

func validateCancellationMemberBindings(
	runs map[identity.RunID]rundomain.Run,
	bindings map[identity.RunID]cancellationRun,
) error {
	for runID, run := range runs {
		if run.State().IsTerminal() {
			continue
		}
		binding := bindings[runID]
		if run.Lineage().IsRoot() {
			continue
		}
		if !binding.hasMember {
			continue
		}
		parent := bindings[run.Lineage().ParentRunID()]
		if !parent.hasMember {
			return fmt.Errorf(
				"runs: build cancellation plan: child Run %q has no bound parent Run %q",
				runID,
				run.Lineage().ParentRunID(),
			)
		}
	}
	return nil
}

func validateCancellationPending(
	root rundomain.Run,
	openRunIDs []identity.RunID,
	bindings map[identity.RunID]cancellationRun,
	pending *Pending,
) error {
	if root.State() == rundomain.Running {
		if pending != nil {
			return fmt.Errorf(
				"runs: build cancellation plan: running tree %q carries a pending set",
				root.ID(),
			)
		}
		return nil
	}
	if pending == nil {
		return fmt.Errorf(
			"runs: build cancellation plan: waiting tree %q has no pending set",
			root.ID(),
		)
	}
	if err := pending.Validate(); err != nil {
		return fmt.Errorf(
			"runs: build cancellation plan: pending tree %q: %w",
			root.ID(),
			err,
		)
	}
	activeRuns := make([]rundomain.Run, 0, len(openRunIDs))
	for _, runID := range openRunIDs {
		activeRuns = append(activeRuns, bindings[runID].run)
	}
	if err := validatePendingRunTree(*pending, activeRuns); err != nil {
		return fmt.Errorf("runs: build cancellation plan: %w", err)
	}
	if pending.RootRunID != root.ID() {
		return fmt.Errorf(
			"runs: build cancellation plan: pending root %q differs from tree %q",
			pending.RootRunID,
			root.ID(),
		)
	}
	if len(pending.Continuations) != len(openRunIDs) {
		return fmt.Errorf(
			"runs: build cancellation plan: %d pending continuations do not cover %d non-terminal Runs",
			len(pending.Continuations),
			len(openRunIDs),
		)
	}
	for index, continuation := range pending.Continuations {
		if continuation.RunID != openRunIDs[index] {
			return fmt.Errorf(
				"runs: build cancellation plan: continuation[%d] is Run %q, want %q",
				index,
				continuation.RunID,
				openRunIDs[index],
			)
		}
		binding := bindings[continuation.RunID]
		if !binding.hasMember || binding.memberID != continuation.MemberID {
			return fmt.Errorf(
				"runs: build cancellation plan: continuation Run %q differs from its executor binding",
				continuation.RunID,
			)
		}
	}
	return nil
}
