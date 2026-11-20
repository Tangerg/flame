package sessions

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	flameruns "github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/scope/core/chat"
)

// sortRunsParentFirst gives persistence a creation-safe tree order while
// preserving the archive's order among peers. Snapshot validation has already
// proved that every parent exists and the graph is acyclic.
func sortRunsParentFirst(runs []rundomain.Run) {
	byID := make(map[identity.RunID]rundomain.Run, len(runs))
	for _, run := range runs {
		byID[run.ID()] = run
	}
	depths := make(map[identity.RunID]int, len(runs))
	var depth func(rundomain.Run) int
	depth = func(run rundomain.Run) int {
		if known, ok := depths[run.ID()]; ok {
			return known
		}
		if run.Lineage().IsRoot() {
			depths[run.ID()] = 0
			return 0
		}
		value := depth(byID[run.Lineage().ParentRunID()]) + 1
		depths[run.ID()] = value
		return value
	}
	slices.SortStableFunc(runs, func(left, right rundomain.Run) int {
		return cmp.Compare(depth(left), depth(right))
	})
}

// TerminalPlan is the complete durable projection for ending a parked Run tree
// by cancellation or executor-state loss. Runs retains each exact waiting
// aggregate and its replacement in canonical postorder so every descendant
// terminalizes before its parent; the root is last. The Runs, interrupt Items,
// root-owned Pending, executor checkpoint, admission, and optional Goal charge
// all move in one transaction.
type TerminalPlan struct {
	runs  []rundomain.Replacement
	items []transcript.Replacement
	// Messages close model-context Tool calls that cannot receive their ordinary
	// result because the parked tree is being abandoned. They are appended in
	// the same transaction before the terminal Run watermark is committed.
	messages         []chat.Message
	checkpointRootID flameruns.MemberID
}

// NewTerminalPlan owns the complete durable projection for a parked Run
// termination. A Goal-owned root Run is applied to its Goal by the same
// transaction, read from the root Run itself.
func NewTerminalPlan(
	runs []rundomain.Replacement,
	items []transcript.Replacement,
	messages []chat.Message,
	checkpointRootID string,
) (TerminalPlan, error) {
	checkpointRoot, err := flameruns.ParseMember(checkpointRootID)
	if err != nil {
		return TerminalPlan{}, fmt.Errorf("sessions: terminal plan checkpoint root: %w", err)
	}
	terminal := TerminalPlan{
		runs:             slices.Clone(runs),
		items:            slices.Clone(items),
		messages:         cloneSnapshotMessages(messages),
		checkpointRootID: checkpointRoot,
	}
	if err := terminal.validate(); err != nil {
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

// Items returns the isolated transcript settlements committed with the Runs.
func (t TerminalPlan) Items() []transcript.Replacement { return slices.Clone(t.items) }

// Messages returns isolated model-context closures committed with the Runs.
func (t TerminalPlan) Messages() []chat.Message { return cloneSnapshotMessages(t.messages) }

// CheckpointRootID returns the canonical executor checkpoint root consumed by the plan.
func (t TerminalPlan) CheckpointRootID() string { return t.checkpointRootID.String() }

// validate proves that the parked-tree terminal write-set is complete,
// canonical, owner-bound, and carries exactly the Goal accounting fact implied
// by its root terminal Run.
func (t TerminalPlan) validate() error {
	root, ok := t.RootRun()
	if !ok {
		return errors.New("sessions: terminal plan must end with one root Run")
	}
	ownedRuns, err := t.validateRuns(root)
	if err != nil {
		return err
	}
	seenItems := make(map[identity.ItemID]struct{}, len(t.items))
	for index, replacement := range t.items {
		item := replacement.State()
		_, owned := ownedRuns[item.RunID()]
		if item.ID().IsZero() || item.SessionID() != root.SessionID() || !owned || item.Status() != transcript.ItemIncomplete {
			return fmt.Errorf("sessions: terminal plan Item[%d] is not an incomplete Item owned by its Run tree", index)
		}
		if _, duplicate := seenItems[item.ID()]; duplicate {
			return fmt.Errorf("sessions: terminal plan repeats Item %q", item.ID())
		}
		seenItems[item.ID()] = struct{}{}
	}
	for index, message := range t.messages {
		if err := message.Validate(); err != nil {
			return fmt.Errorf("sessions: terminal plan Message[%d]: %w", index, err)
		}
	}
	return nil
}

// validateRuns proves the plan ends one Run tree in canonical postorder, every
// Run with the root's terminal outcome, and returns the Runs it owns.
func (t TerminalPlan) validateRuns(root rundomain.Run) (map[identity.RunID]struct{}, error) {
	rootOutcome, rootTerminal := root.Outcome()
	members := make([]rundomain.TreeMember, 0, len(t.runs))
	ownedRuns := make(map[identity.RunID]struct{}, len(t.runs))
	actualOrder := make([]identity.RunID, 0, len(t.runs))
	for index, replacement := range t.runs {
		if err := validateTerminalRunReplacement(replacement); err != nil {
			return nil, fmt.Errorf("sessions: terminal plan Run[%d]: %w", index, err)
		}
		run := replacement.State()
		if run.SessionID() != root.SessionID() {
			return nil, fmt.Errorf("sessions: terminal plan Run %q belongs to Session %q, want %q", run.ID(), run.SessionID(), root.SessionID())
		}
		if outcome, terminal := run.Outcome(); !terminal || !rootTerminal || outcome != rootOutcome {
			return nil, fmt.Errorf("sessions: terminal plan Run %q has a different terminal outcome", run.ID())
		}
		if _, duplicate := ownedRuns[run.ID()]; duplicate {
			return nil, fmt.Errorf("sessions: terminal plan repeats Run %q", run.ID())
		}
		ownedRuns[run.ID()] = struct{}{}
		actualOrder = append(actualOrder, run.ID())
		members = append(members, rundomain.TreeMember{RunID: run.ID(), Lineage: run.Lineage()})
	}
	tree, err := rundomain.NewTree(root.ID(), members)
	if err != nil {
		return nil, fmt.Errorf("sessions: terminal plan Run tree: %w", err)
	}
	if !slices.Equal(actualOrder, tree.Postorder()) {
		return nil, errors.New("sessions: terminal plan Runs are not in canonical postorder")
	}
	return ownedRuns, nil
}

func validateTerminalRunReplacement(replacement rundomain.Replacement) error {
	if err := replacement.Validate(); err != nil {
		return err
	}
	state := replacement.State()
	outcome, _ := state.Outcome()
	switch outcome {
	case rundomain.OutcomeCanceled, rundomain.OutcomeLost:
		return nil
	default:
		return fmt.Errorf("terminal Run replacement has unsupported outcome %q", outcome)
	}
}
