package runs

import (
	"errors"
	"fmt"
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
	if err := validateOpenInvocation(o.SessionID, o.RunID, o.SegmentID, o.CallID, o.StartedAt); err != nil {
		return err
	}
	if o.Endpoint != nil {
		return validateRecoveryEndpoint(o.Endpoint, o.SessionID, o.RunID, o.CallID, o.StartedAt, maxRecoveryEndpointTime(o.Endpoint))
	}
	return nil
}

// Validate checks one incomplete Tool attempt and its Transcript Item identity
// before it can influence boot recovery planning.
func (o OpenToolInvocation) Validate() error {
	if err := validateOpenInvocation(o.SessionID, o.RunID, o.SegmentID, o.CallID, o.StartedAt); err != nil {
		return err
	}
	if err := identity.ValidateItem(o.ItemID); err != nil {
		return err
	}
	return nil
}

func validateOpenInvocation(sessionID, runID, segmentID, callID string, startedAt time.Time) error {
	if err := identity.ValidateSession(sessionID); err != nil {
		return err
	}
	if err := identity.ValidateRun(runID); err != nil {
		return err
	}
	if err := identity.ValidateSegment(segmentID); err != nil {
		return err
	}
	if err := ValidateInvocation(callID); err != nil {
		return err
	}
	if startedAt.IsZero() || startedAt.Location() != time.UTC {
		return errors.New("open invocation start time is required in UTC")
	}
	return nil
}

func validateOpenModelInvocations(invocations []OpenModelInvocation) error {
	seen := make(map[string]struct{}, len(invocations))
	for index, invocation := range invocations {
		if err := invocation.Validate(); err != nil {
			return fmt.Errorf("runs: open model invocation[%d]: %w", index, err)
		}
		if _, duplicate := seen[invocation.CallID]; duplicate {
			return fmt.Errorf("runs: open model invocation catalog repeats call %q", invocation.CallID)
		}
		seen[invocation.CallID] = struct{}{}
	}
	return nil
}

func validateOpenToolInvocations(invocations []OpenToolInvocation) error {
	seenCalls := make(map[recoverySegmentResourceKey]struct{}, len(invocations))
	seenItems := make(map[recoverySegmentResourceKey]struct{}, len(invocations))
	for index, invocation := range invocations {
		if err := invocation.Validate(); err != nil {
			return fmt.Errorf("runs: open Tool invocation[%d]: %w", index, err)
		}
		call := recoverySegmentResourceKey{resourceID: invocation.CallID, segmentID: invocation.SegmentID}
		if _, duplicate := seenCalls[call]; duplicate {
			return fmt.Errorf(
				"runs: open Tool invocation catalog repeats call %q in Segment %q",
				invocation.CallID,
				invocation.SegmentID,
			)
		}
		seenCalls[call] = struct{}{}
		item := recoverySegmentResourceKey{resourceID: invocation.ItemID, segmentID: invocation.SegmentID}
		if _, duplicate := seenItems[item]; duplicate {
			return fmt.Errorf(
				"runs: open Tool invocation catalog repeats Item %q in Segment %q",
				invocation.ItemID,
				invocation.SegmentID,
			)
		}
		seenItems[item] = struct{}{}
	}
	return nil
}

func validateOpenInvocationOwners(
	trees map[string]recoveryRunTree,
	models []OpenModelInvocation,
	tools []OpenToolInvocation,
) error {
	activeRuns := make(map[string]recoveryInvocationOwner)
	for _, tree := range trees {
		for runID, active := range tree.runsByID {
			activeRuns[runID] = recoveryInvocationOwner{
				sessionID: active.SessionID(),
				segmentID: active.ActiveSegmentID(),
			}
		}
	}
	for index, invocation := range models {
		if invocation.Purpose == rundomain.ModelConversation && invocation.Endpoint != nil {
			for _, tree := range trees {
				if active, ok := tree.runsByID[invocation.RunID]; ok && (invocation.Endpoint.Invocation.Provider != active.ModelSelection().Provider() || invocation.Endpoint.Invocation.Model != active.ModelSelection().Model()) {
					return errors.New("runs: recovery endpoint differs from captured model selection")
				}
			}
		}
		if err := validateOpenInvocationOwner(
			"model", index, invocation.SessionID, invocation.RunID, invocation.SegmentID, activeRuns,
		); err != nil {
			return err
		}
	}
	for index, invocation := range tools {
		if err := validateOpenInvocationOwner(
			"Tool", index, invocation.SessionID, invocation.RunID, invocation.SegmentID, activeRuns,
		); err != nil {
			return err
		}
	}
	return nil
}

type recoveryInvocationOwner struct {
	sessionID string
	segmentID string
}

func validateOpenInvocationOwner(
	kind string,
	index int,
	sessionID string,
	runID string,
	segmentID string,
	activeRuns map[string]recoveryInvocationOwner,
) error {
	owner, active := activeRuns[runID]
	if !active {
		// An operational row may outlive a Run already made terminal by an
		// earlier atomic commit. Recovery still owns closing that orphan.
		return nil
	}
	if sessionID != owner.sessionID {
		return fmt.Errorf(
			"runs: open %s invocation[%d] Run %q belongs to Session %q, not %q",
			kind,
			index,
			runID,
			owner.sessionID,
			sessionID,
		)
	}
	if segmentID != owner.segmentID {
		return fmt.Errorf(
			"runs: open %s invocation[%d] belongs to Segment %q, want active Segment %q for Run %q",
			kind,
			index,
			segmentID,
			owner.segmentID,
			runID,
		)
	}
	return nil
}

func maxRecoveryEndpointTime(source *modelref.EndpointObservation) time.Time {
	if source.FinishedAt.IsZero() {
		return source.Invocation.StartedAt
	}
	return source.FinishedAt
}
