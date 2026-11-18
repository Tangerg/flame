package runs

import (
	"errors"
	"fmt"
	"maps"
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
	return validateRecoveryEndpoint(o.Endpoint, o.InvocationAttempt, maxRecoveryEndpointTime(o.Endpoint))
}

// Validate checks one incomplete Tool attempt and its Transcript Item identity
// before it can influence boot recovery planning.
func (o OpenToolInvocation) Validate() error {
	if err := o.InvocationAttempt.Validate(); err != nil {
		return err
	}
	return identity.ValidateItem(o.ItemID)
}

// validateOpenInvocationOwners proves each open attempt still belongs to its
// active Run's current Segment; the journal writer admits only that Segment.
func validateOpenInvocationOwners(
	trees map[string]recoveryRunTree,
	models []OpenModelInvocation,
	tools []OpenToolInvocation,
) error {
	activeRuns := make(map[string]rundomain.Run)
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

func validateOpenInvocationSegment(kind string, index int, attempt InvocationAttempt, activeRuns map[string]rundomain.Run) error {
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

func maxRecoveryEndpointTime(source *modelref.EndpointObservation) time.Time {
	record := source.Snapshot()
	if source.Open() {
		return record.Invocation.StartedAt
	}
	return record.FinishedAt
}
