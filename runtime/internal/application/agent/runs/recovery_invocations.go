package runs

import (
	"errors"
	"fmt"
	"maps"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
)

// Validate checks one incomplete provider attempt before it can influence boot
// recovery planning.
func (o OpenModelInvocation) Validate() error {
	if err := o.Purpose.Validate(); err != nil {
		return err
	}
	if err := o.InvocationAttempt.Validate(); err != nil {
		return err
	}
	_, err := o.endpointObservedAt()
	return err
}

func (o OpenModelInvocation) endpointObservedAt() (time.Time, error) {
	source := o.Endpoint
	if source == nil {
		return time.Time{}, nil
	}
	if err := source.Validate(); err != nil {
		return time.Time{}, err
	}
	record := source.Snapshot()
	if record.Invocation.SessionID != o.SessionID || record.RunID != o.RunID || record.SegmentID != o.SegmentID ||
		record.Invocation.Caller.ID != o.CallID || record.Invocation.StartedAt.Before(o.StartedAt) {
		return time.Time{}, errors.New("runs: recovery endpoint differs from its original call")
	}
	if source.Open() {
		return record.Invocation.StartedAt, nil
	}
	return record.FinishedAt, nil
}

// Validate checks one incomplete Tool attempt and its Transcript Item identity
// before it can influence boot recovery planning.
func (o OpenToolInvocation) Validate() error {
	if err := o.InvocationAttempt.Validate(); err != nil {
		return err
	}
	return o.ItemID.Validate()
}

// validateOpenInvocationOwners proves each open attempt still belongs to its
// active Run's current Segment; the journal writer admits only that Segment.
func validateOpenInvocationOwners(
	trees map[identity.RunID]recoveryRunTree,
	models []OpenModelInvocation,
	tools []OpenToolInvocation,
) error {
	activeRuns := make(map[identity.RunID]rundomain.Run)
	for _, tree := range trees {
		maps.Copy(activeRuns, tree.runsByID)
	}
	for index, invocation := range models {
		active, found := activeRuns[invocation.RunID]
		if found && invocation.Purpose == rundomain.ModelConversation && invocation.Endpoint != nil {
			called := invocation.Endpoint.Snapshot().Invocation
			if called.Provider != active.ModelSelection().Provider() || called.Model != active.ModelSelection().Model() {
				return errors.New("runs: recovery endpoint differs from captured model selection")
			}
		}
		if err := validateOpenInvocationSegment("model", index, invocation.InvocationAttempt, activeRuns); err != nil {
			return err
		}
	}
	for index, invocation := range tools {
		if err := validateOpenInvocationSegment("Tool", index, invocation.InvocationAttempt, activeRuns); err != nil {
			return err
		}
	}
	return nil
}

func validateOpenInvocationSegment(kind string, index int, attempt InvocationAttempt, activeRuns map[identity.RunID]rundomain.Run) error {
	owner, active := activeRuns[attempt.RunID]
	if !active {
		// An operational row may outlive a Run already made terminal by an
		// earlier atomic commit. Recovery still owns closing that orphan.
		return nil
	}
	if attempt.SegmentID != owner.ActiveSegmentID() {
		return fmt.Errorf(
			"runs: open %s invocation[%d] belongs to Segment %q, want active Segment %q for Run %q",
			kind,
			index,
			attempt.SegmentID,
			owner.ActiveSegmentID(),
			attempt.RunID,
		)
	}
	return nil
}
