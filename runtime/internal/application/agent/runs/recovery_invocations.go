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

func (o OpenToolInvocation) Validate() error {
	if err := o.InvocationAttempt.Validate(); err != nil {
		return err
	}
	return o.ItemID.Validate()
}

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

type InvocationAttempt struct {
	SessionID identity.SessionID
	RunID     identity.RunID
	SegmentID identity.SegmentID
	CallID    string
	StartedAt time.Time
}

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

func compareInvocationAttempts(a, other InvocationAttempt) int {
	return cmp.Or(
		a.SessionID.Compare(other.SessionID),
		a.RunID.Compare(other.RunID),
		strings.Compare(a.SegmentID.String(), other.SegmentID.String()),
		strings.Compare(a.CallID, other.CallID),
	)
}

type OpenModelInvocation struct {
	InvocationAttempt
	Purpose  rundomain.ModelCallPurpose
	Endpoint *modelref.EndpointObservation
}

type OpenToolInvocation struct {
	InvocationAttempt
	ItemID identity.ItemID
}

type ModelInvocationRecovery struct {
	OpenModelInvocation
	FinishedAt time.Time
}

type ToolInvocationRecovery struct {
	OpenToolInvocation
	FinishedAt time.Time
}

func (t ToolInvocationRecovery) Transition() ToolInvocationCommit {
	return ToolInvocationCommit{
		CallID: t.CallID, ItemID: t.ItemID, SegmentID: t.SegmentID,
		State: ToolInvocationIncomplete, StartedAt: t.StartedAt, FinishedAt: t.FinishedAt,
	}
}
