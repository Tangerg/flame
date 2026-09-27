package render

import (
	"fmt"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
)

// runScope protects a renderer from mixing unrelated streams while allowing a
// root stream to carry events from its negotiated child-run tree.
type runScope struct {
	rootID  string
	members map[string]conversation.RunLineage
}

func (r *runScope) bind(run conversation.Run) error {
	if err := run.Validate(); err != nil {
		return err
	}
	if !run.Lineage.IsRoot() {
		return fmt.Errorf("run %s is not a root", run.ID)
	}
	if r.rootID != "" && r.rootID != run.ID {
		return fmt.Errorf("run %s does not match %s", run.ID, r.rootID)
	}
	r.ensureMembers()
	r.rootID = run.ID
	r.members[run.ID] = run.Lineage
	return nil
}

func (r *runScope) accept(envelope conversation.RunEvent) error {
	if started, opening := envelope.Event.(conversation.SegmentStarted); opening {
		return r.acceptSegmentStarted(envelope.RunID, started.Run)
	}
	if _, exists := r.members[envelope.RunID]; !exists {
		return fmt.Errorf("event references unknown run %s", envelope.RunID)
	}
	return validateRunEventOwnership(envelope)
}

func (r *runScope) acceptSegmentStarted(envelopeRunID string, run conversation.Run) error {
	if run.ID != envelopeRunID {
		return fmt.Errorf("segment start run %s does not match envelope %s", run.ID, envelopeRunID)
	}
	if run.Lineage.IsRoot() {
		return r.bind(run)
	}
	if r.rootID == "" || run.Lineage.RootRunID() != r.rootID {
		return fmt.Errorf("child run %s does not belong to root %s", run.ID, r.rootID)
	}
	r.ensureMembers()
	if _, exists := r.members[run.Lineage.ParentRunID()]; !exists {
		return fmt.Errorf("child run %s has unknown parent %s", run.ID, run.Lineage.ParentRunID())
	}
	if lineage, exists := r.members[run.ID]; exists && lineage != run.Lineage {
		return fmt.Errorf("child run %s changed lineage", run.ID)
	}
	r.members[run.ID] = run.Lineage
	return nil
}

func validateRunEventOwnership(envelope conversation.RunEvent) error {
	switch event := envelope.Event.(type) {
	case conversation.BlockStarted:
		if event.Block.RunID != envelope.RunID {
			return fmt.Errorf("block %s belongs to run %s, not %s", event.Block.ID, event.Block.RunID, envelope.RunID)
		}
	case conversation.BlockCompleted:
		if event.Block.RunID != envelope.RunID {
			return fmt.Errorf("block %s belongs to run %s, not %s", event.Block.ID, event.Block.RunID, envelope.RunID)
		}
	case conversation.RunInterrupted:
		for _, interaction := range event.Interactions {
			if conversation.InteractionRunID(interaction) != envelope.RunID {
				return fmt.Errorf("interrupt for run %s carries an interaction from run %s", envelope.RunID, conversation.InteractionRunID(interaction))
			}
		}
	}
	return nil
}

func (r *runScope) restore(snapshot conversation.SessionSnapshot, rootID string) error {
	run, exists := snapshot.RunByID(rootID)
	if !exists {
		return fmt.Errorf("run %s is absent from the snapshot", rootID)
	}
	if err := r.bind(run); err != nil {
		return err
	}
	for _, member := range snapshot.Runs {
		if member.Lineage.RootRunID() == rootID {
			r.members[member.ID] = member.Lineage
		}
	}
	return nil
}

func (r *runScope) contains(runID string) bool {
	_, exists := r.members[runID]
	return exists
}

func (r *runScope) isRoot(runID string) bool { return runID != "" && runID == r.rootID }

func (r *runScope) isChild(runID string) bool {
	lineage, exists := r.members[runID]
	return exists && !lineage.IsRoot()
}

func (r *runScope) ensureMembers() {
	if r.members == nil {
		r.members = make(map[string]conversation.RunLineage)
	}
}
