package runs

import (
	"context"
	"fmt"

	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// cancellationRun is one immutable member of a command-bound cancellation
// plan. MemberID is present only for a non-terminal Run whose executor member
// must remain addressable at this boundary; executor topology is not an
// application cancellation fact.
type cancellationRun struct {
	run       rundomain.Run
	memberID  string
	hasMember bool
}

// cancellationPlan is the complete, immutable fact set one cancellation command
// acts on. It is application-private because it combines domain Run topology,
// durable pending state, and process-local executor bindings; none of those
// outer representations belongs in the execution domain itself.
type cancellationPlan struct {
	root                 cancellationRun
	target               cancellationRun
	targetSubtree        []cancellationRun
	survivingTree        []cancellationRun
	treeState            rundomain.State
	executor             ExecutorRef
	pending              Pending
	hasPending           bool
	spawningItem         transcript.Item
	hasSpawningItem      bool
	targetInterruptItems []transcript.Item
	targetDrainedItems   []transcript.Item
	completePostorderIDs []string
}

// cancellationPlanSource is the coherent read model used to build one
// cancellation plan. It keeps repository facts and the process-local owner
// together only inside this use case; neither representation is promoted to a
// domain or executor contract.
type cancellationPlanSource struct {
	runs             []rundomain.Run
	pending          Pending
	hasPending       bool
	live             liveSegment
	hasLive          bool
	executor         ExecutorRef
	memberIDsByRunID map[string]string
}

// cancellationPlanFor resolves either a root or child address to one complete
// tree snapshot before any executor side effect. The single Tree read avoids
// racing a target lookup against a second aggregate lookup.
func (c *Coordinator) cancellationPlanFor(
	ctx context.Context,
	cmd CancelCommand,
) (cancellationPlan, liveSegment, bool, error) {
	source, err := c.readCancellationPlanSource(ctx, cmd)
	if err != nil {
		return cancellationPlan{}, liveSegment{}, false, err
	}

	var pending *Pending
	if source.hasPending {
		pending = &source.pending
	}
	plan, err := newCancellationPlan(
		cmd.RunID,
		source.runs,
		source.executor,
		source.memberIDsByRunID,
		pending,
	)
	if err != nil {
		return cancellationPlan{}, liveSegment{}, false, err
	}
	if plan.treeState == rundomain.Waiting && plan.target.run.Lineage().IsChild() {
		if err := c.loadWaitingCancellationItems(ctx, &plan); err != nil {
			return cancellationPlan{}, liveSegment{}, false, err
		}
	}
	return plan, source.live, source.hasLive, nil
}

func (c *Coordinator) readCancellationPlanSource(
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
	pending, pendingFound, err := c.interrupts.LookupOpenInterrupt(ctx, rootRunID)
	if err != nil {
		return cancellationPlanSource{}, err
	}
	live, liveFound := c.segments.lookup(rootRunID)
	executor, memberIDsByRunID, err := c.resolveCancellationOwner(
		ctx,
		cmd.RunID,
		root,
		pending,
		pendingFound,
		live,
		liveFound,
	)
	if err != nil {
		return cancellationPlanSource{}, err
	}
	return cancellationPlanSource{
		runs:             runs,
		pending:          pending,
		hasPending:       pendingFound,
		live:             live,
		hasLive:          liveFound,
		executor:         executor,
		memberIDsByRunID: memberIDsByRunID,
	}, nil
}

func (c *Coordinator) resolveCancellationOwner(
	ctx context.Context,
	targetRunID string,
	root rundomain.Run,
	pending Pending,
	hasPending bool,
	live liveSegment,
	hasLive bool,
) (ExecutorRef, map[string]string, error) {
	switch root.State() {
	case rundomain.Running:
		if hasPending {
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
		if err := validateCancellationLiveRoot(live, root); err != nil {
			return ExecutorRef{}, nil, c.classifyCancellationOwnerDrift(
				ctx, targetRunID, root.State(), err,
			)
		}
		return ExecutorRef{
			SessionID:  live.record.SessionID,
			ExecutorID: live.record.ExecutorID,
		}, live.owner.executorMemberSnapshot(), nil
	case rundomain.Waiting:
		if !hasPending {
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
		if err := pending.Validate(); err != nil {
			return ExecutorRef{}, nil, fmt.Errorf(
				"runs: cancellation tree %q pending set: %w",
				root.ID(),
				err,
			)
		}
		members := make(map[string]string, len(pending.Continuations))
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
			if err := validateCancellationLiveRoot(live, root); err != nil {
				return ExecutorRef{}, nil, c.classifyCancellationOwnerDrift(
					ctx, targetRunID, root.State(), err,
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
		}
		return ExecutorRef{
			SessionID:  pending.SessionID,
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

// classifyCancellationOwnerDrift distinguishes a durable lifecycle transition
// from a persistent ownership invariant violation. Tree, interrupt, and live
// owner facts come from different consistency domains, so Resume or terminal
// commit can linearize between those reads. That loser is a normal busy/finished
// outcome; only a contradiction that still exists at the refreshed Run state is
// an internal invariant fault.
func (c *Coordinator) classifyCancellationOwnerDrift(
	ctx context.Context,
	targetRunID string,
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
		return fmt.Errorf(
			"%w: Run %q moved from %s to %s while cancellation ownership was resolved: %v",
			ErrSessionBusy,
			targetRunID,
			sourceState,
			refreshed.State(),
			cause,
		)
	default:
		return cause
	}
}

func validateCancellationLiveRoot(live liveSegment, root rundomain.Run) error {
	switch {
	case live.record.ID != root.ID():
		return fmt.Errorf(
			"runs: cancellation root %q is owned by registry entry %q",
			root.ID(),
			live.record.ID,
		)
	case live.record.SessionID != root.SessionID():
		return fmt.Errorf(
			"runs: cancellation root %q belongs to session %q but its live owner belongs to %q",
			root.ID(),
			root.SessionID(),
			live.record.SessionID,
		)
	case live.record.ExecutorID == "":
		return fmt.Errorf("runs: cancellation root %q live owner has no executor ID", root.ID())
	case live.record.SegmentID != root.ActiveSegmentID():
		return fmt.Errorf(
			"runs: cancellation root %q durable segment %q differs from live owner %q",
			root.ID(),
			root.ActiveSegmentID(),
			live.record.SegmentID,
		)
	case !live.record.CreatedAt.Equal(root.CreatedAt()):
		return fmt.Errorf("runs: cancellation root %q creation time differs from live owner", root.ID())
	case live.record.ModelSelection != root.ModelSelection():
		return fmt.Errorf("runs: cancellation root %q model selection differs from live owner", root.ID())
	case !live.record.Capabilities.Equal(root.Capabilities()):
		return fmt.Errorf("runs: cancellation root %q run capabilities differ from live owner", root.ID())
	default:
		return nil
	}
}

func runByID(runs []rundomain.Run, runID string) (rundomain.Run, bool) {
	for _, run := range runs {
		if run.ID() == runID {
			return run, true
		}
	}
	return rundomain.Run{}, false
}
