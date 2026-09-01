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
	"github.com/Tangerg/flame/runtime/internal/domain/run/toolresult"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/exactint"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// RollbackPlan is the atomic durable command for truncating a session back to a
// run boundary. A parked run among DropRunIDs needs no terminalization: dropping
// its record is also how it releases the session's admission slot.
type RollbackPlan struct {
	SessionID         string
	KeepMessageMark   int
	DropRunIDs        []string
	CheckpointRootIDs []string
	// PlanReplacement is the application-decided replacement for the Plan the
	// boundary held. Applying it is a NEW state commit
	// (Replace, never delete-and-rewrite): the live revision has to move forward or a
	// client holding a higher one discards the rolled-back list as stale.
	PlanReplacement *PlanReplacement
}

type ForkPlan struct {
	ParentID string
	// Child is the complete Domain-derived child Session. Persistence verifies
	// the parent still exists and inserts this exact aggregate.
	Child    session.Session
	Messages []chat.Message
	// Runs, Items, and ToolResults are the identity-remapped durable projection of
	// the copied conversation. They make the child's visible transcript agree with
	// the chat history used as model context.
	Runs        []rundomain.Run
	Items       []transcript.Item
	ToolResults []toolresult.Blob
	// PlanReplacement is the initial child Plan decided from the parent's fork
	// boundary. nil means the boundary held no value worth publishing.
	PlanReplacement *PlanReplacement
}

// Replacement is an immutable Application decision to insert a restored
// Session at revision one or replace the target's current revision exactly.
type Replacement struct {
	expectedRevision uint64
	state            session.Session
}

// InitialReplacement prepares an initial restored Session write.
func InitialReplacement(state session.Session) (Replacement, error) {
	replacement := Replacement{state: state}
	if err := replacement.Validate(); err != nil {
		return Replacement{}, err
	}
	return replacement, nil
}

// NextReplacement prepares an exact replacement of current.
func NextReplacement(current, state session.Session) (Replacement, error) {
	replacement := Replacement{expectedRevision: current.Revision(), state: state}
	if err := replacement.Validate(); err != nil {
		return Replacement{}, err
	}
	return replacement, nil
}

// ExpectedRevision returns zero for an initial insert or the target revision
// an exact replacement was based on.
func (s Replacement) ExpectedRevision() uint64 { return s.expectedRevision }

// State returns the complete already-decided Session aggregate.
func (s Replacement) State() session.Session { return s.state }

// Validate proves that s is either one initial aggregate or one monotonic
// replacement of an existing aggregate.
func (s Replacement) Validate() error {
	if err := s.state.Validate(); err != nil {
		return fmt.Errorf("sessions: invalid Session replacement: %w", err)
	}
	if s.expectedRevision == 0 {
		firstRevision := exactint.First().Value()
		if s.state.Revision() != firstRevision {
			return fmt.Errorf("sessions: initial Session revision is %d, want %d", s.state.Revision(), firstRevision)
		}
		return nil
	}
	if err := exactint.Follows(s.expectedRevision, s.state.Revision()); err != nil {
		return fmt.Errorf(
			"sessions: Session replacement revision %d does not follow expected revision %d",
			s.state.Revision(), s.expectedRevision,
		)
	}
	return nil
}

// RestorePlan is the atomic durable command for replacing a session aggregate.
// It is intentionally distinct from Snapshot, the export read model: the
// explicit command makes the persistence boundary's destructive operation
// visible instead of silently accepting every snapshot-shaped value.
type RestorePlan struct {
	Session     Replacement
	Messages    []chat.Message
	Items       []transcript.Item
	Runs        []rundomain.Run
	ToolResults []toolresult.Blob
	// PlanReplacement is the already-decided restored Plan transition. It updates
	// the live row in place so the target session's revision space never restarts.
	PlanReplacement *PlanReplacement
}

func restorePlan(
	snapshot Snapshot,
	sessionReplacement Replacement,
	planReplacement *PlanReplacement,
) RestorePlan {
	return RestorePlan{
		Session: sessionReplacement, Messages: snapshot.Messages, Items: snapshot.Items,
		Runs: runsInParentFirstOrder(snapshot.Runs), ToolResults: snapshot.ToolResults,
		PlanReplacement: planReplacement,
	}
}

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

// DeletePlan removes exactly one addressed conversation. User-created forks are
// independent conversations and delegated work is represented by child Runs,
// not hidden Session rows.
type DeletePlan struct {
	SessionID string
}

// TerminalPlan is the complete durable projection for ending a parked Run tree
// by cancellation or executor-state loss. Runs is canonical postorder so every
// descendant terminalizes before its parent; the root is last. The Runs,
// interrupt Items, root-owned Pending, executor checkpoint, admission, and
// optional Goal charge all move in one transaction.
type TerminalPlan struct {
	Runs  []rundomain.Run
	Items []transcript.Item
	// Messages close model-context Tool calls that cannot receive their ordinary
	// result because the parked tree is being abandoned. They are appended in
	// the same transaction before the terminal Run watermark is committed.
	Messages         []chat.Message
	CheckpointRootID string
	// ResumeClaimed requires persistence to consume the resuming interrupt owned
	// by a failed Resume attempt. Ordinary parked termination consumes an open
	// interrupt instead.
	ResumeClaimed bool
	// GoalRun is present exactly when the root Run was admitted by an autonomous Goal.
	// Keeping it in the same write-set makes every terminal path—not only the
	// normal reducer path—charge the incarnation atomically with the Run transition.
	GoalRun *goal.RunRecord
}

// RootRun returns the root terminal projection. A valid plan always has one.
func (t TerminalPlan) RootRun() (rundomain.Run, bool) {
	if len(t.Runs) == 0 {
		return rundomain.Run{}, false
	}
	root := t.Runs[len(t.Runs)-1]
	return root, root.Lineage().IsRoot()
}

// Validate proves that the parked-tree terminal write-set is complete,
// canonical, owner-bound, and carries exactly the Goal accounting fact implied
// by its root terminal Run.
func (t TerminalPlan) Validate() error {
	root, ok := t.RootRun()
	if !ok {
		return errors.New("sessions: terminal plan must end with one root Run")
	}
	members := make([]rundomain.TreeMember, 0, len(t.Runs))
	ownedRuns := make(map[string]struct{}, len(t.Runs))
	actualOrder := make([]string, 0, len(t.Runs))
	for index, run := range t.Runs {
		if err := run.Validate(); err != nil {
			return fmt.Errorf("sessions: terminal plan Run[%d]: %w", index, err)
		}
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
	if t.ResumeClaimed && rootOutcome != rundomain.OutcomeLost {
		return errors.New("sessions: claimed Resume terminal plan must recover a lost Run")
	}
	tree, err := rundomain.NewTree(root.ID(), members)
	if err != nil {
		return fmt.Errorf("sessions: terminal plan Run tree: %w", err)
	}
	if !slices.Equal(actualOrder, tree.Postorder()) {
		return errors.New("sessions: terminal plan Runs are not in canonical postorder")
	}
	if _, err := runtimeidentity.ParseMember(t.CheckpointRootID); err != nil {
		return fmt.Errorf("sessions: terminal plan checkpoint root: %w", err)
	}
	seenItems := make(map[string]struct{}, len(t.Items))
	for index, item := range t.Items {
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
	for index, message := range t.Messages {
		if err := message.Validate(); err != nil {
			return fmt.Errorf("sessions: terminal plan Message[%d]: %w", index, err)
		}
	}
	return validateTerminalGoalRun(root, t.GoalRun)
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
