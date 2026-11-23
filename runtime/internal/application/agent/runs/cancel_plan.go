package runs

import (
	"context"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

type cancellationRun struct {
	run       rundomain.Run
	memberID  string
	hasMember bool
}

type cancellationPlan struct {
	root                 cancellationRun
	target               cancellationRun
	targetSubtree        []cancellationRun
	survivingTree        []cancellationRun
	executor             ExecutorRef
	pending              *Pending
	spawningItem         *transcript.Item
	items                map[identity.ItemID]transcript.Item
	interrupts           []transcript.Interrupt
	targetInterruptItems []transcript.Item
	targetDrainedItems   []transcript.Item
}

func (p cancellationPlan) treeState() rundomain.State { return p.root.run.State() }

func (p cancellationPlan) treeRuns() []rundomain.Run {
	runs := p.survivingRuns()
	for _, member := range p.targetSubtree {
		runs = append(runs, member.run)
	}
	return runs
}

func (p cancellationPlan) survivingRuns() []rundomain.Run {
	runs := make([]rundomain.Run, len(p.survivingTree))
	for index, member := range p.survivingTree {
		runs[index] = member.run
	}
	return runs
}

type cancellationPlanSource struct {
	runs             []rundomain.Run
	pending          *Pending
	live             liveSegment
	hasLive          bool
	executor         ExecutorRef
	memberIDsByRunID map[identity.RunID]string
}

func (c *Cancellations) cancellationPlanFor(
	ctx context.Context,
	cmd CancelCommand,
) (cancellationPlan, liveSegment, bool, error) {
	source, err := c.readCancellationPlanSource(ctx, cmd)
	if err != nil {
		return cancellationPlan{}, liveSegment{}, false, err
	}

	plan, err := newCancellationPlan(
		cmd.RunID,
		source.runs,
		source.executor,
		source.memberIDsByRunID,
		source.pending,
	)
	if err != nil {
		return cancellationPlan{}, liveSegment{}, false, err
	}
	if plan.treeState() == rundomain.Waiting && plan.target.run.Lineage().IsChild() {
		if err := c.loadWaitingCancellationItems(ctx, &plan); err != nil {
			return cancellationPlan{}, liveSegment{}, false, err
		}
	}
	return plan, source.live, source.hasLive, nil
}

func (c *Cancellations) readCancellationPlanSource(
	ctx context.Context,
	cmd CancelCommand,
) (cancellationPlanSource, error) {
	runs, err := c.runs.Tree(ctx, cmd.RunID)
	if err != nil {
		return cancellationPlanSource{}, err
	}
	if len(runs) == 0 {
		return cancellationPlanSource{}, fmt.Errorf("%w: %q", ErrRunNotFound, cmd.RunID)
	}
	target, found := runByID(runs, cmd.RunID)
	if !found {
		return cancellationPlanSource{}, fmt.Errorf(
			"runs: tree containing target %q omitted the target",
			cmd.RunID,
		)
	}
	if target.Lineage().IsChild() && !cmd.AllowChildRun {
		return cancellationPlanSource{}, fmt.Errorf("%w: %q", ErrChildRunNotAllowed, cmd.RunID)
	}
	if target.State().IsTerminal() {
		return cancellationPlanSource{}, fmt.Errorf("%w: %q", ErrRunFinished, cmd.RunID)
	}

	rootRunID := target.Lineage().TreeRootID(target.ID())
	root, found := runByID(runs, rootRunID)
	if !found {
		return cancellationPlanSource{}, fmt.Errorf(
			"runs: cancellation tree for Run %q omits root %q",
			cmd.RunID,
			rootRunID,
		)
	}
	open, pendingFound, err := c.interrupts.LookupOpenInterrupt(ctx, rootRunID)
	if err != nil {
		return cancellationPlanSource{}, err
	}
	var pending *Pending
	if pendingFound {
		pending = &open
	}
	var live liveSegment
	var liveFound bool
	if root.State() == rundomain.Running {
		live, liveFound = c.segments.registry.Running(rootRunID)
	} else {
		live, liveFound = c.segments.registry.Get(rootRunID)
	}
	executor, memberIDsByRunID, err := c.resolveCancellationOwner(
		ctx,
		cmd.RunID,
		root,
		pending,
		live,
		liveFound,
	)
	if err != nil {
		return cancellationPlanSource{}, err
	}
	return cancellationPlanSource{
		runs:             runs,
		pending:          pending,
		live:             live,
		hasLive:          liveFound,
		executor:         executor,
		memberIDsByRunID: memberIDsByRunID,
	}, nil
}

func (c *Cancellations) resolveCancellationOwner(
	ctx context.Context,
	targetRunID identity.RunID,
	root rundomain.Run,
	pending *Pending,
	live liveSegment,
	hasLive bool,
) (ExecutorRef, map[identity.RunID]string, error) {
	switch root.State() {
	case rundomain.Running:
		if pending != nil {
			return ExecutorRef{}, nil, c.classifyCancellationOwnerDrift(
				ctx,
				targetRunID,
				root.State(),
				fmt.Errorf(
					"runs: running tree %q has an open interrupt",
					root.ID(),
				),
			)
		}
		if !hasLive || live.owner == nil {
			return ExecutorRef{}, nil, c.classifyCancellationOwnerDrift(
				ctx,
				targetRunID,
				root.State(),
				fmt.Errorf(
					"runs: running tree %q has no live root owner",
					root.ID(),
				),
			)
		}
		if err := live.owner.admitCancellation(root, live.record); err != nil {
			return ExecutorRef{}, nil, c.classifyCancellationOwnerDrift(
				ctx, targetRunID, root.State(), err,
			)
		}
		return ExecutorRef{
			SessionID:  root.SessionID(),
			ExecutorID: live.record.ExecutorID,
		}, live.owner.executorMemberSnapshot(), nil
	case rundomain.Waiting:
		if pending == nil {
			return ExecutorRef{}, nil, c.classifyCancellationOwnerDrift(
				ctx,
				targetRunID,
				root.State(),
				fmt.Errorf(
					"runs: waiting tree %q has no open interrupt",
					root.ID(),
				),
			)
		}
		members := make(map[identity.RunID]string, len(pending.Continuations))
		for _, continuation := range pending.Continuations {
			members[continuation.RunID] = continuation.MemberID
		}
		if hasLive {
			if live.owner == nil {
				return ExecutorRef{}, nil, c.classifyCancellationOwnerDrift(
					ctx,
					targetRunID,
					root.State(),
					fmt.Errorf(
						"runs: waiting tree %q has a live registry entry without a Run-tree owner",
						root.ID(),
					),
				)
			}

			if live.record.ExecutorID != pending.ExecutorID {
				return ExecutorRef{}, nil, c.classifyCancellationOwnerDrift(
					ctx,
					targetRunID,
					root.State(),
					fmt.Errorf(
						"runs: waiting tree %q live executor %q differs from pending executor %q",
						root.ID(),
						live.record.ExecutorID,
						pending.ExecutorID,
					),
				)
			}
			if err := live.owner.admitCancellation(root, live.record); err != nil {
				return ExecutorRef{}, nil, c.classifyCancellationOwnerDrift(
					ctx, targetRunID, root.State(), err,
				)
			}
		}
		return ExecutorRef{
			SessionID:  root.SessionID(),
			ExecutorID: pending.ExecutorID,
		}, members, nil
	default:
		return ExecutorRef{}, nil, fmt.Errorf(
			"runs: cancellation root %q has state %s while target %q remains non-terminal",
			root.ID(),
			root.State(),
			targetRunID,
		)
	}
}

func (c *Cancellations) classifyCancellationOwnerDrift(
	ctx context.Context,
	targetRunID identity.RunID,
	sourceState rundomain.State,
	cause error,
) error {
	refreshed, found, err := c.runs.Run(ctx, targetRunID)
	switch {
	case err != nil:
		return err
	case !found:
		return fmt.Errorf(
			"runs: Run %q disappeared after its cancellation tree was resolved: %w",
			targetRunID,
			cause,
		)
	case refreshed.State().IsTerminal():
		return fmt.Errorf(
			"%w: %q completed as %s",
			ErrRunFinished,
			targetRunID,
			refreshed.State(),
		)
	case refreshed.State() != sourceState:
		// The refreshed state decides the category; the drift cause stays text so its own sentinels cannot compete.
		return fmt.Errorf(
			"%w: Run %q moved from %s to %s while cancellation ownership was resolved: %s",
			rundomain.ErrSessionBusy,
			targetRunID,
			sourceState,
			refreshed.State(),
			cause.Error(),
		)
	default:
		return cause
	}
}

func runByID(runs []rundomain.Run, runID identity.RunID) (rundomain.Run, bool) {
	for _, run := range runs {
		if run.ID() == runID {
			return run, true
		}
	}
	return rundomain.Run{}, false
}
