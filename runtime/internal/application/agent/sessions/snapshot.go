package sessions

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
)

func (s Snapshot) Validate() error {
	if err := plan.ValidateSteps(s.Plan); err != nil {
		return fmt.Errorf("sessions: snapshot Plan: %w", err)
	}
	return s.validateHistory()
}

func (s HistorySnapshot) validateHistory() error {
	// The Session anchors every ownership comparison below, so an unbuilt one
	// would make an all-zero snapshot self-consistent.
	if s.Session.ID().IsZero() {
		return errors.New("sessions: snapshot carries no Session")
	}
	if err := conversation.ValidateMessages(s.Messages); err != nil {
		return fmt.Errorf("sessions: snapshot conversation: %w", err)
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

func (s HistorySnapshot) validateRuns() (map[identity.RunID]struct{}, error) {
	runs := make(map[identity.RunID]struct{}, len(s.Runs))
	for _, run := range s.Runs {
		if run.ID().IsZero() || run.SessionID() != s.Session.ID() {
			return nil, fmt.Errorf("sessions: snapshot run %q belongs to session %q, want %q", run.ID(), run.SessionID(), s.Session.ID())
		}
		if _, exists := runs[run.ID()]; exists {
			return nil, fmt.Errorf("sessions: snapshot contains duplicate run %q", run.ID())
		}
		if !run.State().IsTerminal() {
			return nil, fmt.Errorf("sessions: snapshot run %q is %s, want terminal", run.ID(), run.State())
		}
		if count, known := run.MessageMark().Count(); known && count > len(s.Messages) {
			return nil, fmt.Errorf("sessions: snapshot run %q has invalid message watermark %s", run.ID(), run.MessageMark())
		}
		runs[run.ID()] = struct{}{}
	}
	return runs, nil
}

func (s HistorySnapshot) validateItems(runs map[identity.RunID]struct{}) (map[identity.ItemID]transcript.Item, error) {
	items := make(map[identity.ItemID]transcript.Item, len(s.Items))
	for _, item := range s.Items {
		if item.ID().IsZero() || item.SessionID() != s.Session.ID() {
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
		if call, _ := item.ToolCall(); call.Failure != nil && item.Status() != transcript.ItemIncomplete {
			return nil, fmt.Errorf("sessions: snapshot item %q has an invalid tool failure", item.ID())
		}
	}
	return items, nil
}

func validateSnapshotRunTree(runs []run.Run, items map[identity.ItemID]transcript.Item) error {
	index := &snapshotRunTree{runByID: make(map[identity.RunID]run.Run, len(runs))}
	for _, value := range runs {
		index.runByID[value.ID()] = value
	}
	membersByRoot := make(map[identity.RunID][]run.TreeMember, len(runs))
	for _, value := range runs {
		lineage := value.Lineage()
		rootRunID := value.ID()
		if lineage.IsChild() {
			if err := index.validateChildSpawn(value, items); err != nil {
				return err
			}
			rootRunID = lineage.RootRunID()
		}
		membersByRoot[rootRunID] = append(
			membersByRoot[rootRunID],
			run.TreeMember{RunID: value.ID(), Lineage: lineage},
		)
	}
	for _, rootRunID := range slices.SortedFunc(maps.Keys(membersByRoot), identity.RunID.Compare) {
		if _, err := run.NewTree(rootRunID, membersByRoot[rootRunID]); err != nil {
			return fmt.Errorf("sessions: snapshot run tree: %w", err)
		}
	}
	return nil
}

type snapshotRunTree struct {
	runByID map[identity.RunID]run.Run
}

func (s *snapshotRunTree) validateChildSpawn(
	child run.Run,
	items map[identity.ItemID]transcript.Item,
) error {
	lineage := child.Lineage()
	parent, parentFound := s.runByID[lineage.ParentRunID()]
	if !parentFound {
		return fmt.Errorf("sessions: snapshot child run %q references unknown parent %q", child.ID(), lineage.ParentRunID())
	}
	root, rootFound := s.runByID[lineage.RootRunID()]
	if !rootFound {
		return fmt.Errorf("sessions: snapshot child run %q references unknown root %q", child.ID(), lineage.RootRunID())
	}
	if !root.Capabilities().ChildRuns {
		return fmt.Errorf(
			"sessions: snapshot child run %q belongs to root %q whose run capabilities disallow child runs",
			child.ID(),
			root.ID(),
		)
	}
	item, found := items[lineage.SpawnedByItemID()]
	if !found {
		return fmt.Errorf("sessions: snapshot run %q references unknown spawning item %q", child.ID(), lineage.SpawnedByItemID())
	}
	if err := item.ValidateSpawns(child.SessionID(), parent.ID()); err != nil {
		return fmt.Errorf("sessions: snapshot child run %q: %w", child.ID(), err)
	}
	return nil
}
