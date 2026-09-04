package runs

import (
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// Validate checks one incomplete provider attempt before it can influence boot
// recovery planning.
func (o OpenModelInvocation) Validate() error {
	return validateOpenInvocation(o.SessionID, o.RunID, o.SegmentID, o.CallID, o.StartedAt)
}

// Validate checks one incomplete Tool attempt and its Transcript Item identity
// before it can influence boot recovery planning.
func (o OpenToolInvocation) Validate() error {
	if err := validateOpenInvocation(o.SessionID, o.RunID, o.SegmentID, o.CallID, o.StartedAt); err != nil {
		return err
	}
	if _, err := resourceid.ParseItem(o.ItemID); err != nil {
		return err
	}
	return nil
}

func validateOpenInvocation(sessionID, runID, segmentID, callID string, startedAt time.Time) error {
	if _, err := resourceid.ParseSession(sessionID); err != nil {
		return err
	}
	if _, err := resourceid.ParseRun(runID); err != nil {
		return err
	}
	if _, err := resourceid.ParseSegment(segmentID); err != nil {
		return err
	}
	if _, err := runtimeidentity.ParseEffect(callID); err != nil {
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
