package runs

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
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

// InvocationAttempt locates one journaled attempt inside the Run Segment that
// started it. The journal row's Session is its Run's Session by schema.
type InvocationAttempt struct {
	SessionID identity.SessionID
	RunID     identity.RunID
	SegmentID identity.SegmentID
	CallID    string
	StartedAt time.Time
}

// Validate checks the attempt identity before it can influence recovery.
func (a InvocationAttempt) Validate() error {
	if err := a.SessionID.Validate(); err != nil {
		return err
	}
	if err := a.RunID.Validate(); err != nil {
		return err
	}
	if err := a.SegmentID.Validate(); err != nil {
		return err
	}
	if err := ValidateInvocation(a.CallID); err != nil {
		return err
	}
	if a.StartedAt.IsZero() || a.StartedAt.Location() != time.UTC {
		return errors.New("open invocation start time is required in UTC")
	}
	return nil
}

func (a InvocationAttempt) compare(other InvocationAttempt) int {
	return cmp.Or(
		a.SessionID.Compare(other.SessionID),
		a.RunID.Compare(other.RunID),
		strings.Compare(a.SegmentID.String(), other.SegmentID.String()),
		strings.Compare(a.CallID, other.CallID),
	)
}

// OpenModelInvocation retains a committed logical start. Its physical endpoint
// is optional because endpoint admission has its own durable boundary.
type OpenModelInvocation struct {
	InvocationAttempt
	Purpose  rundomain.ModelCallPurpose
	Endpoint *modelref.EndpointObservation
}

// OpenToolInvocation is an operational Tool attempt without a durable
// terminal observation. ItemID binds the attempt back to its canonical
// Transcript lifecycle without copying arguments or results into the journal.
type OpenToolInvocation struct {
	InvocationAttempt
	ItemID identity.ItemID
}

// ModelInvocationRecovery closes a boot-abandoned outer operation. A known
// physical response remains evidence under failure; absence remains unknown.
// The state is implied by the recovery operation rather than stored as an
// application enum in the persistence port.
type ModelInvocationRecovery struct {
	OpenModelInvocation
	FinishedAt time.Time
}

// ToolInvocationRecovery marks one boot-abandoned Tool attempt incomplete.
type ToolInvocationRecovery struct {
	OpenToolInvocation
	FinishedAt time.Time
}

// Transition is the journal transition that closes this attempt.
func (t ToolInvocationRecovery) Transition() ToolInvocationCommit {
	return ToolInvocationCommit{
		CallID: t.CallID, ItemID: t.ItemID, SegmentID: t.SegmentID,
		State: ToolInvocationIncomplete, StartedAt: t.StartedAt, FinishedAt: t.FinishedAt,
	}
}
