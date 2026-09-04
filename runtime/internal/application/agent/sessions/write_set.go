package sessions

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/scope/core/chat"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// runsInParentFirstOrder gives persistence a creation-safe tree order while
// preserving the archive's order among peers. Snapshot validation has already
// proved that every parent exists and the graph is acyclic.
func runsInParentFirstOrder(runs []rundomain.Run) []rundomain.Run {
	ordered := append([]rundomain.Run(nil), runs...)
	byID := make(map[string]rundomain.Run, len(runs))
	for _, run := range runs {
		byID[run.ID()] = run
	}
	depths := make(map[string]int, len(runs))
	var depth func(rundomain.Run) int
	depth = func(run rundomain.Run) int {
		if known, ok := depths[run.ID()]; ok {
			return known
		}
		if run.Lineage().IsRoot() {
			depths[run.ID()] = 0
			return 0
		}
		value := depth(byID[run.Lineage().ParentRunID]) + 1
		depths[run.ID()] = value
		return value
	}
	slices.SortStableFunc(ordered, func(left, right rundomain.Run) int {
		return cmp.Compare(depth(left), depth(right))
	})
	return ordered
}

// TerminalPlan is the complete durable projection for ending a parked Run tree
// by cancellation or executor-state loss. Runs retains each exact waiting
// aggregate and its replacement in canonical postorder so every descendant
// terminalizes before its parent; the root is last. The Runs, interrupt Items,
// root-owned Pending, executor checkpoint, admission, and optional Goal charge
// all move in one transaction.
type TerminalPlan struct {
	runs  []rundomain.Replacement
	items []transcript.Item
	// Messages close model-context Tool calls that cannot receive their ordinary
	// result because the parked tree is being abandoned. They are appended in
	// the same transaction before the terminal Run watermark is committed.
	messages         []chat.Message
	checkpointRootID runtimeidentity.MemberID
	// ResumeClaimed requires persistence to consume the resuming interrupt owned
	// by a failed Resume attempt. Ordinary parked termination consumes an open
	// interrupt instead.
	resumeClaimed bool
	// GoalRun is present exactly when the root Run was admitted by an autonomous Goal.
	// Keeping it in the same write-set makes every terminal path—not only the
	// normal reducer path—charge the incarnation atomically with the Run transition.
	goalRun *goal.RunRecord
}

// NewTerminalPlan owns the complete durable projection for an ordinary parked
// Run termination. Goal accounting is derived from the root Run so callers
// cannot supply a second version of the same terminal fact.
func NewTerminalPlan(
	runs []rundomain.Replacement,
	items []transcript.Item,
	messages []chat.Message,
	checkpointRootID string,
) (TerminalPlan, error) {
	return newTerminalPlan(runs, items, messages, checkpointRootID, false)
}

// NewClaimedResumeTerminalPlan owns the complete durable projection for a
// Resume claim whose executor state proved unrestorable.
func NewClaimedResumeTerminalPlan(
	runs []rundomain.Replacement,
	items []transcript.Item,
	messages []chat.Message,
	checkpointRootID string,
) (TerminalPlan, error) {
	return newTerminalPlan(runs, items, messages, checkpointRootID, true)
}

func newTerminalPlan(
	runs []rundomain.Replacement,
	items []transcript.Item,
	messages []chat.Message,
	checkpointRootID string,
	resumeClaimed bool,
) (TerminalPlan, error) {
	checkpointRoot, err := runtimeidentity.ParseMember(checkpointRootID)
	if err != nil {
		return TerminalPlan{}, fmt.Errorf("sessions: terminal plan checkpoint root: %w", err)
	}
	terminal := TerminalPlan{
		runs:             slices.Clone(runs),
		items:            slices.Clone(items),
		messages:         cloneSnapshotMessages(messages),
		checkpointRootID: checkpointRoot,
		resumeClaimed:    resumeClaimed,
	}
	if root, ok := terminal.RootRun(); ok && root.GoalIncarnationID() != "" {
		record, err := terminalGoalRun(root)
		if err != nil {
			return TerminalPlan{}, err
		}
		terminal.goalRun = &record
	}
	if err := terminal.Validate(); err != nil {
		return TerminalPlan{}, err
	}
	return terminal, nil
}

// RootRun returns the root terminal projection. A valid plan always has one.
func (t TerminalPlan) RootRun() (rundomain.Run, bool) {
	if len(t.runs) == 0 {
		return rundomain.Run{}, false
	}
	root := t.runs[len(t.runs)-1].State()
	return root, root.Lineage().IsRoot()
}

// Runs returns an isolated canonical postorder of exact Run replacements.
func (t TerminalPlan) Runs() []rundomain.Replacement { return slices.Clone(t.runs) }

// Items returns the isolated incomplete transcript projections committed with the Runs.
func (t TerminalPlan) Items() []transcript.Item { return slices.Clone(t.items) }

// Messages returns isolated model-context closures committed with the Runs.
func (t TerminalPlan) Messages() []chat.Message { return cloneSnapshotMessages(t.messages) }

// CheckpointRootID returns the canonical executor checkpoint root consumed by the plan.
func (t TerminalPlan) CheckpointRootID() string { return t.checkpointRootID.String() }

// ConsumesClaimedResume reports whether the plan consumes a claimed Resume hand-off.
func (t TerminalPlan) ConsumesClaimedResume() bool { return t.resumeClaimed }

// GoalRun returns the root-derived Goal accounting record, when the Run was Goal-owned.
func (t TerminalPlan) GoalRun() *goal.RunRecord {
	if t.goalRun == nil {
		return nil
	}
	record := *t.goalRun
	return &record
}

// Validate proves that the parked-tree terminal write-set is complete,
// canonical, owner-bound, and carries exactly the Goal accounting fact implied
// by its root terminal Run.
func (t TerminalPlan) Validate() error {
	root, ok := t.RootRun()
	if !ok {
		return errors.New("sessions: terminal plan must end with one root Run")
	}
	members := make([]rundomain.TreeMember, 0, len(t.runs))
	ownedRuns := make(map[string]struct{}, len(t.runs))
	actualOrder := make([]string, 0, len(t.runs))
	for index, replacement := range t.runs {
		if err := validateTerminalRunReplacement(replacement); err != nil {
			return fmt.Errorf("sessions: terminal plan Run[%d]: %w", index, err)
		}
		run := replacement.State()
		if run.SessionID() != root.SessionID() {
			return fmt.Errorf("sessions: terminal plan Run %q belongs to Session %q, want %q", run.ID(), run.SessionID(), root.SessionID())
		}
		outcome, terminal := run.Outcome()
		rootOutcome, rootTerminal := root.Outcome()
		if !terminal || !rootTerminal || outcome != rootOutcome {
			return fmt.Errorf("sessions: terminal plan Run %q has a different terminal outcome", run.ID())
		}
		if _, duplicate := ownedRuns[run.ID()]; duplicate {
			return fmt.Errorf("sessions: terminal plan repeats Run %q", run.ID())
		}
		ownedRuns[run.ID()] = struct{}{}
		actualOrder = append(actualOrder, run.ID())
		members = append(members, rundomain.TreeMember{RunID: run.ID(), Lineage: run.Lineage()})
	}
	rootOutcome, _ := root.Outcome()
	if t.resumeClaimed && rootOutcome != rundomain.OutcomeLost {
		return errors.New("sessions: claimed Resume terminal plan must recover a lost Run")
	}
	tree, err := rundomain.NewTree(root.ID(), members)
	if err != nil {
		return fmt.Errorf("sessions: terminal plan Run tree: %w", err)
	}
	if !slices.Equal(actualOrder, tree.Postorder()) {
		return errors.New("sessions: terminal plan Runs are not in canonical postorder")
	}
	if err := t.checkpointRootID.Validate(); err != nil {
		return fmt.Errorf("sessions: terminal plan checkpoint root: %w", err)
	}
	seenItems := make(map[string]struct{}, len(t.items))
	for index, item := range t.items {
		_, owned := ownedRuns[item.RunID()]
		if item.ID() == "" || item.SessionID() != root.SessionID() || !owned || item.Status() != transcript.ItemIncomplete {
			return fmt.Errorf("sessions: terminal plan Item[%d] is not an incomplete Item owned by its Run tree", index)
		}
		if _, duplicate := seenItems[item.ID()]; duplicate {
			return fmt.Errorf("sessions: terminal plan repeats Item %q", item.ID())
		}
		seenItems[item.ID()] = struct{}{}
		if err := item.Validate(); err != nil {
			return fmt.Errorf("sessions: terminal plan Item %q: %w", item.ID(), err)
		}
	}
	for index, message := range t.messages {
		if err := message.Validate(); err != nil {
			return fmt.Errorf("sessions: terminal plan Message[%d]: %w", index, err)
		}
	}
	return validateTerminalGoalRun(root, t.goalRun)
}

func terminalGoalRun(root rundomain.Run) (goal.RunRecord, error) {
	outcome, _ := root.Outcome()
	record := goal.RunRecord{
		SessionID:     root.SessionID(),
		IncarnationID: root.GoalIncarnationID(),
		RunID:         root.ID(),
		Outcome:       outcome,
		Steps:         root.Metrics().Steps(),
		CompletedAt:   root.FinishedAt(),
	}
	cost, err := goalRunCost(root.Metrics())
	if err != nil {
		return goal.RunRecord{}, fmt.Errorf("sessions: terminal Goal Run cost: %w", err)
	}
	record.Cost = cost
	return record, nil
}

func validateTerminalRunReplacement(replacement rundomain.Replacement) error {
	if err := replacement.Validate(); err != nil {
		return err
	}
	expected := replacement.Expected()
	state := replacement.State()
	outcome, terminal := state.Outcome()
	if !terminal {
		return errors.New("terminal Run replacement has no outcome")
	}
	current, err := expected.AdvanceProgress(
		state.Metrics(),
		state.ContextTokens(),
		state.FinishedAt(),
	)
	if err != nil {
		return fmt.Errorf("terminal Run replacement progress: %w", err)
	}
	var derived rundomain.Run
	switch outcome {
	case rundomain.OutcomeCanceled:
		derived, err = current.CancelWaiting(state.Detail(), state.FinishedAt(), state.MessageMark())
	case rundomain.OutcomeLost:
		failure, failed := state.Failure()
		if !failed {
			return errors.New("lost Run replacement has no failure")
		}
		derived, err = current.RecoverLost(failure, state.FinishedAt(), state.MessageMark())
	default:
		return fmt.Errorf("terminal Run replacement has unsupported outcome %s", outcome)
	}
	if err != nil {
		return fmt.Errorf("terminal Run replacement transition: %w", err)
	}
	if !derived.Equal(state) {
		return fmt.Errorf("terminal Run replacement rewrites facts outside Run %q transition", expected.ID())
	}
	return nil
}

func validateTerminalGoalRun(run rundomain.Run, record *goal.RunRecord) error {
	if run.GoalIncarnationID() == "" {
		if record != nil {
			return fmt.Errorf("sessions: terminal plan non-Goal Run %q carries a Goal Run", run.ID())
		}
		return nil
	}
	if record == nil {
		return fmt.Errorf("sessions: terminal plan Goal-owned Run %q has no Goal Run", run.ID())
	}
	if err := record.Validate(); err != nil {
		return fmt.Errorf("sessions: terminal plan Goal Run: %w", err)
	}
	cost, err := goalRunCost(run.Metrics())
	if err != nil {
		return fmt.Errorf("sessions: terminal plan Goal Run cost: %w", err)
	}
	outcome, terminal := run.Outcome()
	if !terminal || record.SessionID != run.SessionID() || record.IncarnationID != run.GoalIncarnationID() ||
		record.RunID != run.ID() || record.Outcome != outcome || !record.Cost.Equal(cost) ||
		record.Steps != run.Metrics().Steps() || !record.CompletedAt.Equal(run.FinishedAt()) {
		return fmt.Errorf("sessions: terminal plan Goal Run differs from Run %q", run.ID())
	}
	return nil
}

func goalRunCost(metrics rundomain.Metrics) (accounting.Cost, error) {
	usage, reported := metrics.Usage()
	if !reported {
		return accounting.Cost{}, nil
	}
	return accounting.CostFromOptional(usage.Total.CostUSD)
}
