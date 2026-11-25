package runs

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

type waitingCancellationPlan struct {
	root                 rundomain.Run
	target               rundomain.Run
	tree                 []rundomain.Run
	targetSubtree        []rundomain.Run
	survivingRuns        []rundomain.Run
	pending              Pending
	items                map[identity.ItemID]transcript.Item
	interrupts           []transcript.Interrupt
	targetInterruptItems []transcript.Item
	targetDrainedItems   []transcript.Item
}

// Session admission pins the parked tree, its Pending, and its Items until the
// cancellation commits or releases the claim.
func (c *Cancellations) planWaitingChildCancellation(ctx context.Context, runID identity.RunID) (waitingCancellationPlan, error) {
	values, err := c.runs.Tree(ctx, runID)
	if err != nil {
		return waitingCancellationPlan{}, err
	}
	target, found := runByID(values, runID)
	switch {
	case !found:
		return waitingCancellationPlan{}, fmt.Errorf("%w: %q", ErrRunNotFound, runID)
	case target.State().IsTerminal():
		return waitingCancellationPlan{}, fmt.Errorf("%w: %q", ErrRunFinished, runID)
	case target.State() != rundomain.Waiting:
		return waitingCancellationPlan{}, fmt.Errorf("%w: Run %q is %s", rundomain.ErrSessionBusy, runID, target.State())
	}
	rootID := target.Lineage().RootRunID()
	root, found := runByID(values, rootID)
	if !found {
		return waitingCancellationPlan{}, fmt.Errorf("runs: waiting child Run %q has no root Run %q", runID, rootID)
	}
	if live, found := c.segments.registry.Get(rootID); found {
		if err := live.owner.wait(ctx); err != nil {
			return waitingCancellationPlan{}, err
		}
	}
	pending, found, err := c.interrupts.Get(ctx, rootID)
	if err != nil {
		return waitingCancellationPlan{}, err
	}
	if !found {
		return waitingCancellationPlan{}, fmt.Errorf("runs: waiting root Run %q has no open interrupt", rootID)
	}
	plan, err := partitionWaitingCancellation(root, target, values, pending)
	if err != nil {
		return waitingCancellationPlan{}, err
	}
	if err := c.loadWaitingCancellationItems(ctx, &plan); err != nil {
		return waitingCancellationPlan{}, err
	}
	return plan, nil
}

func partitionWaitingCancellation(root, target rundomain.Run, values []rundomain.Run, pending Pending) (waitingCancellationPlan, error) {
	topology, byRunID, err := pending.parkedTree(values)
	if err != nil {
		return waitingCancellationPlan{}, fmt.Errorf("runs: waiting root %q: %w", root.ID(), err)
	}
	subtree, found := topology.SubtreePostorder(target.ID())
	if !found {
		return waitingCancellationPlan{}, fmt.Errorf("runs: waiting child Run %q is outside the parked tree", target.ID())
	}
	plan := waitingCancellationPlan{root: root, target: target, tree: values, pending: pending}
	canceled := make(map[identity.RunID]struct{}, len(subtree))
	for _, runID := range subtree {
		canceled[runID] = struct{}{}
		plan.targetSubtree = append(plan.targetSubtree, byRunID[runID])
	}
	for _, continuation := range pending.Continuations {
		if _, removed := canceled[continuation.RunID]; !removed {
			plan.survivingRuns = append(plan.survivingRuns, byRunID[continuation.RunID])
		}
	}
	return plan, nil
}

func runByID(values []rundomain.Run, runID identity.RunID) (rundomain.Run, bool) {
	for _, value := range values {
		if value.ID() == runID {
			return value, true
		}
	}
	return rundomain.Run{}, false
}
