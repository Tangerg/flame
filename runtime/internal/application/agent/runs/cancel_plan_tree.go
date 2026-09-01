package runs

import (
	"fmt"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
)

func newCancellationPlan(
	targetRunID string,
	runs []rundomain.Run,
	executor ExecutorRef,
	memberIDsByRunID map[string]string,
	pending *Pending,
) (cancellationPlan, error) {
	runTree, err := buildCancellationRunTree(targetRunID, runs, executor)
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
	targetRunIDSet := make(map[string]struct{}, len(targetRunIDs))
	for _, runID := range targetRunIDs {
		targetRunIDSet[runID] = struct{}{}
	}
	plan := cancellationPlan{
		root:                 bindings[runTree.root.ID()],
		target:               bindings[targetRunID],
		treeState:            runTree.root.State(),
		executor:             executor,
		hasPending:           pending != nil,
		completePostorderIDs: runTree.topology.Postorder(),
	}
	if pending != nil {
		plan.pending = *pending
	}
	for _, runID := range plan.completePostorderIDs {
		member := bindings[runID]
		if _, targeted := targetRunIDSet[runID]; targeted {
			plan.targetSubtree = append(plan.targetSubtree, member)
		} else {
			plan.survivingTree = append(plan.survivingTree, member)
		}
	}
	return plan, nil
}

// cancellationRunTree is the validated durable Run snapshot on which one
// cancellation decision is based. It owns product lifecycle and topology
// facts only; process-local executor bindings remain outside this value.
type cancellationRunTree struct {
	root     rundomain.Run
	target   rundomain.Run
	topology rundomain.Tree
	byRunID  map[string]rundomain.Run
}

func buildCancellationRunTree(
	targetRunID string,
	runs []rundomain.Run,
	executor ExecutorRef,
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
	if err := executor.ValidateFor(root.SessionID()); err != nil {
		return cancellationRunTree{}, fmt.Errorf(
			"runs: build cancellation plan for tree %q: executor: %w",
			rootRunID,
			err,
		)
	}

	byRunID := make(map[string]rundomain.Run, len(runs))
	treeMembers := make([]rundomain.TreeMember, 0, len(runs))
	for index, run := range runs {
		if err := run.Validate(); err != nil {
			return cancellationRunTree{}, fmt.Errorf(
				"runs: build cancellation plan for tree %q: Run[%d] %q: %w",
				rootRunID,
				index,
				run.ID(),
				err,
			)
		}
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
		target:   target,
		topology: tree,
		byRunID:  byRunID,
	}
	if err := runTree.validateLifecycle(); err != nil {
		return cancellationRunTree{}, err
	}
	return runTree, nil
}

func (c cancellationRunTree) validateLifecycle() error {
	root := c.root
	if root.State() != rundomain.Running && root.State() != rundomain.Waiting {
		return fmt.Errorf(
			"runs: build cancellation plan: root Run %q is %s",
			root.ID(),
			root.State(),
		)
	}
	if c.target.State().IsTerminal() {
		return fmt.Errorf(
			"runs: build cancellation plan: target Run %q is %s",
			c.target.ID(),
			c.target.State(),
		)
	}
	for _, run := range c.byRunID {
		if !run.State().IsTerminal() && run.State() != root.State() {
			return fmt.Errorf(
				"runs: build cancellation plan: non-terminal Run %q is %s while root %q is %s",
				run.ID(),
				run.State(),
				root.ID(),
				root.State(),
			)
		}
	}
	return nil
}

func (c cancellationRunTree) openRunIDs(
	bindings map[string]cancellationRun,
) ([]string, error) {
	openRunIDs := make([]string, 0, len(c.byRunID))
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
	runs map[string]rundomain.Run,
	memberIDsByRunID map[string]string,
) (map[string]cancellationRun, error) {
	bindings := make(map[string]cancellationRun, len(runs))
	for runID, run := range runs {
		bindings[runID] = cancellationRun{run: run}
	}
	memberOwners := make(map[string]string, len(memberIDsByRunID))
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
	runs map[string]rundomain.Run,
	bindings map[string]cancellationRun,
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
		parent := bindings[run.Lineage().ParentRunID]
		if !parent.hasMember {
			return fmt.Errorf(
				"runs: build cancellation plan: child Run %q has no bound parent Run %q",
				runID,
				run.Lineage().ParentRunID,
			)
		}
	}
	return nil
}

func validateCancellationPending(
	root rundomain.Run,
	openRunIDs []string,
	bindings map[string]cancellationRun,
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
	if pending.RootRunID != root.ID() || pending.SessionID != root.SessionID() {
		return fmt.Errorf(
			"runs: build cancellation plan: pending scope %q/%q differs from tree %q/%q",
			pending.SessionID,
			pending.RootRunID,
			root.SessionID(),
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
