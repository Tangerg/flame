package render

import (
	"fmt"

	"github.com/Tangerg/flame/runtime/protocol"

	"github.com/Tangerg/flame/cli/internal/domain/agent"
)

// runScope protects a renderer from mixing unrelated streams while allowing a
// root stream to carry events from its negotiated child-run tree.
type runScope struct {
	rootID  string
	members map[string]string
}

func (r *runScope) bindRoot(runID string) error {
	if err := protocol.ValidateRunID(runID); err != nil {
		return err
	}
	if r.rootID != "" && r.rootID != runID {
		return fmt.Errorf("run %s does not match %s", runID, r.rootID)
	}
	r.ensureMembers()
	r.rootID = runID
	r.members[runID] = ""
	return nil
}

func (r *runScope) accept(envelope agent.RunEvent) error {
	if started, opening := envelope.Event.(agent.SegmentStarted); opening {
		return r.acceptSegmentStarted(envelope.RunID, started.Run)
	}
	if _, exists := r.members[envelope.RunID]; !exists {
		return fmt.Errorf("event references unknown run %s", envelope.RunID)
	}
	return validateRunEventOwnership(envelope)
}

func (r *runScope) acceptSegmentStarted(envelopeRunID string, run protocol.RunRef) error {
	if run.ID != envelopeRunID {
		return fmt.Errorf("segment start run %s does not match envelope %s", run.ID, envelopeRunID)
	}
	if run.ParentRunID == "" {
		return r.bindRoot(run.ID)
	}
	if r.rootID == "" || run.RootRunID != r.rootID {
		return fmt.Errorf("child run %s does not belong to root %s", run.ID, r.rootID)
	}
	r.ensureMembers()
	if _, exists := r.members[run.ParentRunID]; !exists {
		return fmt.Errorf("child run %s has unknown parent %s", run.ID, run.ParentRunID)
	}
	if lineage, exists := r.members[run.ID]; exists && lineage != run.ParentRunID {
		return fmt.Errorf("child run %s changed lineage", run.ID)
	}
	r.members[run.ID] = run.ParentRunID
	return nil
}

func validateRunEventOwnership(envelope agent.RunEvent) error {
	switch event := envelope.Event.(type) {
	case agent.BlockStarted:
		if event.Block.RunID != envelope.RunID {
			return fmt.Errorf("block %s belongs to run %s, not %s", event.Block.ID, event.Block.RunID, envelope.RunID)
		}
	case agent.BlockCompleted:
		if event.Block.RunID != envelope.RunID {
			return fmt.Errorf("block %s belongs to run %s, not %s", event.Block.ID, event.Block.RunID, envelope.RunID)
		}
	case agent.RunInterrupted:
		for _, interaction := range event.Interactions {
			if agent.InteractionRunID(interaction) != envelope.RunID {
				return fmt.Errorf("interrupt for run %s carries an interaction from run %s", envelope.RunID, agent.InteractionRunID(interaction))
			}
		}
	}
	return nil
}

func (r *runScope) restore(snapshot agent.SessionSnapshot, rootID string) error {
	run, exists := snapshot.RunByID(rootID)
	if !exists {
		return fmt.Errorf("run %s is absent from the snapshot", rootID)
	}
	if run.ParentRunID != "" {
		return fmt.Errorf("run %s is not a root", run.ID)
	}
	if err := r.bindRoot(run.ID); err != nil {
		return err
	}
	for _, member := range snapshot.Runs {
		if member.RootRunID == rootID {
			r.members[member.ID] = member.ParentRunID
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
	return exists && lineage != ""
}

func (r *runScope) ensureMembers() {
	if r.members == nil {
		r.members = make(map[string]string)
	}
}
