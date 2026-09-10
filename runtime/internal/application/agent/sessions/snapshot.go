package sessions

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
)

// ExportResult is the complete result of a session archive use case. The
// archive and its session view are derived while the same admission is held, so
// callers cannot pair one revision's archive with a later revision's view.
type ExportResult struct {
	Session  View
	Snapshot PortableSnapshot
	Items    []transcript.Item
}

// ExportSession reserves the session's single-writer slot and derives its
// portable archive and presentation from one coherent canonical state. Active
// and parked runs are rejected because their executor state is process-local
// and therefore cannot be represented by a portable session artifact.
func (c *Coordinator) ExportSession(ctx context.Context, sessionID string) (ExportResult, error) {
	admission, err := c.ClaimIdleSession(ctx, sessionID)
	if err != nil {
		return ExportResult{}, err
	}
	defer admission.Release()
	snapshot, err := c.snapshots.ReadSnapshot(ctx, sessionID)
	if err != nil {
		return ExportResult{}, err
	}
	if validateErr := snapshot.Session.ValidateFor(sessionID); validateErr != nil {
		return ExportResult{}, fmt.Errorf("sessions: export snapshot identity: %w", validateErr)
	}
	if validateErr := snapshot.Validate(); validateErr != nil {
		return ExportResult{}, validateErr
	}
	portable, err := snapshot.PortableSnapshot()
	if err != nil {
		return ExportResult{}, fmt.Errorf("sessions: prepare portable snapshot: %w", err)
	}
	view, err := c.view(snapshot.Session, ActivityIdle)
	if err != nil {
		return ExportResult{}, err
	}
	return ExportResult{Session: view, Snapshot: portable, Items: snapshot.Items}, nil
}

// Validate checks the complete Session and the snapshot's referential integrity
// before the coordinator hands it out.
func (s Snapshot) Validate() error {
	// The Session anchors every ownership comparison below, so an unbuilt one
	// would make an all-zero snapshot self-consistent.
	if s.Session.ID() == "" {
		return errors.New("sessions: snapshot carries no Session")
	}
	if _, err := conversation.New(s.Messages); err != nil {
		return fmt.Errorf("sessions: snapshot conversation: %w", err)
	}
	if err := plan.ValidateSteps(s.Plan); err != nil {
		return fmt.Errorf("sessions: snapshot Plan: %w", err)
	}
	runs, err := s.validateRuns()
	if err != nil {
		return err
	}
	items, err := s.validateItems(runs)
	if err != nil {
		return err
	}
	if err := validateSnapshotRunTree(s.Runs, items); err != nil {
		return err
	}
	return s.ValidateToolResults()
}

func (s Snapshot) validateRuns() (map[string]struct{}, error) {
	runs := make(map[string]struct{}, len(s.Runs))
	for _, run := range s.Runs {
		if run.ID() == "" || run.SessionID() != s.Session.ID() {
			return nil, fmt.Errorf("sessions: snapshot run %q belongs to session %q, want %q", run.ID(), run.SessionID(), s.Session.ID())
		}
		if _, exists := runs[run.ID()]; exists {
			return nil, fmt.Errorf("sessions: snapshot contains duplicate run %q", run.ID())
		}
		// A snapshot is a portable record of finished work, so only a terminal Run
		// belongs in one; whether its facts hold together is the Run's own rule.
		if !run.State().IsTerminal() {
			return nil, fmt.Errorf("sessions: snapshot run %q is %s, want terminal", run.ID(), run.State())
		}
		if run.MessageMark() > len(s.Messages) {
			return nil, fmt.Errorf("sessions: snapshot run %q has invalid message watermark %d", run.ID(), run.MessageMark())
		}
		runs[run.ID()] = struct{}{}
	}
	return runs, nil
}

func (s Snapshot) validateItems(runs map[string]struct{}) (map[string]transcript.Item, error) {
	items := make(map[string]transcript.Item, len(s.Items))
	for _, item := range s.Items {
		if item.ID() == "" || item.SessionID() != s.Session.ID() {
			return nil, fmt.Errorf("sessions: snapshot item %q belongs to session %q, want %q", item.ID(), item.SessionID(), s.Session.ID())
		}
		if _, exists := items[item.ID()]; exists {
			return nil, fmt.Errorf("sessions: snapshot contains duplicate item %q", item.ID())
		}
		items[item.ID()] = item
		if _, found := runs[item.RunID()]; !found {
			return nil, fmt.Errorf("sessions: snapshot item %q references unknown run %q", item.ID(), item.RunID())
		}
		switch item.Status() {
		case transcript.ItemCompleted, transcript.ItemIncomplete:
		case transcript.ItemRunning:
			return nil, fmt.Errorf("sessions: snapshot terminal run item %q is still running", item.ID())
		default:
			return nil, fmt.Errorf("sessions: snapshot item %q has unknown status %q", item.ID(), item.Status())
		}
		if _, failed := item.Failure(); failed && (item.Kind() != transcript.ToolCall || item.Status() != transcript.ItemIncomplete) {
			return nil, fmt.Errorf("sessions: snapshot item %q has an invalid tool failure", item.ID())
		}
	}
	return items, nil
}

// validateSnapshotRunTree proves every Run in the archive belongs to one
// complete topology. Reachability, cycles, duplicates and the root each child
// names are run.Tree's rules; a snapshot only adds what its Items say about the
// call that spawned each child.
func validateSnapshotRunTree(runs []run.Run, items map[string]transcript.Item) error {
	index := &snapshotRunTree{runByID: make(map[string]run.Run, len(runs))}
	for _, value := range runs {
		index.runByID[value.ID()] = value
	}
	membersByRoot := make(map[string][]run.TreeMember, len(runs))
	for _, value := range runs {
		lineage := value.Lineage()
		rootRunID := value.ID()
		if lineage.IsChild() {
			if err := index.validateChildSpawn(value, items); err != nil {
				return err
			}
			rootRunID = lineage.RootRunID
		}
		membersByRoot[rootRunID] = append(
			membersByRoot[rootRunID],
			run.TreeMember{RunID: value.ID(), Lineage: lineage},
		)
	}
	for _, rootRunID := range slices.Sorted(maps.Keys(membersByRoot)) {
		if _, err := run.NewTree(rootRunID, membersByRoot[rootRunID]); err != nil {
			return fmt.Errorf("sessions: snapshot run tree: %w", err)
		}
	}
	return nil
}

type snapshotRunTree struct {
	runByID map[string]run.Run
}

// validateChildSpawn proves the Item a child names is the Tool call its parent
// actually made. run.Tree settles the identities; only the transcript can say
// whether the call that produced this child exists in it.
func (s *snapshotRunTree) validateChildSpawn(
	child run.Run,
	items map[string]transcript.Item,
) error {
	lineage := child.Lineage()
	parent, parentFound := s.runByID[lineage.ParentRunID]
	if !parentFound {
		return fmt.Errorf("sessions: snapshot child run %q references unknown parent %q", child.ID(), lineage.ParentRunID)
	}
	root, rootFound := s.runByID[lineage.RootRunID]
	if !rootFound {
		return fmt.Errorf("sessions: snapshot child run %q references unknown root %q", child.ID(), lineage.RootRunID)
	}
	if !root.Capabilities().ChildRuns {
		return fmt.Errorf(
			"sessions: snapshot child run %q belongs to root %q whose run capabilities disallow child runs",
			child.ID(),
			root.ID(),
		)
	}
	item, found := items[lineage.SpawnedByItemID]
	if !found {
		return fmt.Errorf("sessions: snapshot run %q references unknown spawning item %q", child.ID(), lineage.SpawnedByItemID)
	}
	if item.Kind() != transcript.ToolCall {
		return fmt.Errorf("sessions: snapshot run %q spawning item %q is not a tool call", child.ID(), lineage.SpawnedByItemID)
	}
	if item.RunID() != parent.ID() {
		return fmt.Errorf(
			"sessions: snapshot child run %q spawning item %q belongs to run %q, want parent %q",
			child.ID(),
			item.ID(),
			item.RunID(),
			parent.ID(),
		)
	}
	return nil
}
