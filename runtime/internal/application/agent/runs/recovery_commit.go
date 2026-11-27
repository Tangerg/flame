package runs

import (
	"errors"
	"fmt"
	"iter"
	"slices"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	corechat "github.com/Tangerg/scope/core/chat"
)

type RecoveryCommit struct {
	trees                []recoveryTreeOutcome
	modelInvocations     []ModelInvocationRecovery
	independentEndpoints []modelref.EndpointObservation
	toolInvocations      []ToolInvocationRecovery
}

type recoveryTreeOutcome interface {
	sessionID() identity.SessionID
}

type preservedRecoveryTree struct {
	session identity.SessionID
}

func (t preservedRecoveryTree) sessionID() identity.SessionID { return t.session }

type LostTreeRecovery struct {
	runs    []rundomain.Replacement
	items   []transcript.Replacement
	closure recoveryConversationClosure
}

type recoveryConversationClosure struct {
	previousCount int
	messages      []corechat.Message
}

func (t LostTreeRecovery) sessionID() identity.SessionID { return t.Root().SessionID() }

// Replacements follow the validated postorder so dependent Runs retire before
// the root and cleanup uses the same root replacement that storage applies.
func (t LostTreeRecovery) Root() rundomain.Run {
	return t.runs[len(t.runs)-1].State()
}

func (t LostTreeRecovery) RunReplacements() iter.Seq[rundomain.Replacement] {
	return slices.Values(t.runs)
}

func (t LostTreeRecovery) ItemReplacements() iter.Seq[transcript.Replacement] {
	return slices.Values(t.items)
}

func (t LostTreeRecovery) ConversationClosure() (int, []corechat.Message) {
	return t.closure.previousCount, cloneCommitMessages(t.closure.messages)
}

func (r RecoveryCommit) LostTrees() iter.Seq[LostTreeRecovery] {
	return func(yield func(LostTreeRecovery) bool) {
		for _, tree := range r.trees {
			if lost, ok := tree.(LostTreeRecovery); ok && !yield(lost) {
				return
			}
		}
	}
}

func (r RecoveryCommit) RecoveredSessionIDs() iter.Seq[identity.SessionID] {
	return func(yield func(identity.SessionID) bool) {
		for _, tree := range r.trees {
			if !yield(tree.sessionID()) {
				return
			}
		}
	}
}

func (r RecoveryCommit) lostRunCount() int {
	count := 0
	for tree := range r.LostTrees() {
		count += len(tree.runs)
	}
	return count
}

func (r RecoveryCommit) ModelInvocations() []ModelInvocationRecovery {
	return cloneRecoveryModels(r.modelInvocations)
}

func (r RecoveryCommit) IndependentEndpoints() []modelref.EndpointObservation {
	return slices.Clone(r.independentEndpoints)
}

func (r RecoveryCommit) ToolInvocations() []ToolInvocationRecovery {
	return slices.Clone(r.toolInvocations)
}

func (r RecoveryCommit) validateInvocationSources() error {
	recoveredSessions := make(map[identity.SessionID]struct{}, len(r.trees))
	for sessionID := range r.RecoveredSessionIDs() {
		recoveredSessions[sessionID] = struct{}{}
	}
	lostRuns := make(map[identity.RunID]rundomain.Replacement)
	replacedItems := make(map[identity.ItemID]transcript.Replacement)
	for tree := range r.LostTrees() {
		for run := range tree.RunReplacements() {
			lostRuns[run.Expected().ID()] = run
		}
		for item := range tree.ItemReplacements() {
			replacedItems[item.Expected().ID()] = item
		}
	}
	if err := validateRecoveryModelInvocations(r.modelInvocations, lostRuns, recoveredSessions); err != nil {
		return err
	}
	if err := validateIndependentEndpoints(r.independentEndpoints, r.modelInvocations, lostRuns); err != nil {
		return err
	}
	return validateRecoveryToolInvocations(r.toolInvocations, lostRuns, recoveredSessions, replacedItems)
}

func validateIndependentEndpoints(observations []modelref.EndpointObservation, logical []ModelInvocationRecovery, lostByID map[identity.RunID]rundomain.Replacement) error {
	seen := make(map[string]struct{}, len(observations))
	for _, observation := range observations {
		if err := observation.Validate(); err != nil {
			return err
		}
		source := observation.Snapshot()
		owner, found := lostByID[source.RunID]
		if !found || owner.State().SessionID() != source.Invocation.SessionID || source.SegmentID.IsZero() || source.FinishedAt.After(owner.State().UpdatedAt()) {
			return errors.New("runs: independent source has no lost Run owner")
		}
		if _, duplicate := seen[source.Invocation.ID]; duplicate {
			return errors.New("runs: independent recovery source repeats")
		}
		seen[source.Invocation.ID] = struct{}{}
		if slices.ContainsFunc(logical, func(attempt ModelInvocationRecovery) bool { return attempt.CallID == source.Invocation.Caller.ID }) {
			return errors.New("runs: independent source duplicates logical recovery")
		}
	}
	return nil
}

func validateRecoveryModelInvocations(
	invocations []ModelInvocationRecovery,
	lostByID map[identity.RunID]rundomain.Replacement,
	recoveredSessions map[identity.SessionID]struct{},
) error {
	seen := make(map[string]struct{}, len(invocations))
	for index, invocation := range invocations {
		if err := invocation.Purpose.Validate(); err != nil {
			return err
		}
		if err := validateRecoveryInvocation(invocation.InvocationAttempt, invocation.FinishedAt, lostByID, recoveredSessions); err != nil {
			return fmt.Errorf("runs: recovery commit model invocation[%d]: %w", index, err)
		}
		observedAt, err := invocation.endpointObservedAt()
		if err != nil {
			return fmt.Errorf("runs: recovery commit model invocation[%d]: %w", index, err)
		}
		if invocation.FinishedAt.Before(observedAt) {
			return fmt.Errorf("runs: recovery commit model invocation[%d] closes before its endpoint observation", index)
		}
		if _, duplicate := seen[invocation.CallID]; duplicate {
			return fmt.Errorf("runs: recovery commit repeats model invocation %q", invocation.CallID)
		}
		seen[invocation.CallID] = struct{}{}
		if index > 0 && compareModelInvocationRecoveries(invocations[index-1], invocation) >= 0 {
			return errors.New("runs: recovery commit model invocations are not in canonical order")
		}
	}
	return nil
}

type recoverySegmentResourceKey struct {
	resourceID string
	segmentID  identity.SegmentID
}

func validateRecoveryToolInvocations(
	invocations []ToolInvocationRecovery,
	lostByID map[identity.RunID]rundomain.Replacement,
	recoveredSessions map[identity.SessionID]struct{},
	replacedItems map[identity.ItemID]transcript.Replacement,
) error {
	seen := make(map[recoverySegmentResourceKey]struct{}, len(invocations))
	seenItems := make(map[recoverySegmentResourceKey]struct{}, len(invocations))
	for index, invocation := range invocations {
		if err := validateRecoveryInvocation(invocation.InvocationAttempt, invocation.FinishedAt, lostByID, recoveredSessions); err != nil {
			return fmt.Errorf("runs: recovery commit Tool invocation[%d]: %w", index, err)
		}
		if err := invocation.ItemID.Validate(); err != nil {
			return fmt.Errorf("runs: recovery commit Tool invocation[%d]: %w", index, err)
		}
		if _, lost := lostByID[invocation.RunID]; lost {
			replacement, present := replacedItems[invocation.ItemID]
			if !present || replacement.Expected().RunID() != invocation.RunID {
				return fmt.Errorf(
					"runs: recovery commit Tool invocation[%d] %q has no matching lost-Run Item replacement",
					index,
					invocation.CallID,
				)
			}
		}
		key := recoverySegmentResourceKey{resourceID: invocation.CallID, segmentID: invocation.SegmentID}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf(
				"runs: recovery commit repeats Tool invocation %q in Segment %q",
				invocation.CallID,
				invocation.SegmentID,
			)
		}
		seen[key] = struct{}{}
		itemKey := recoverySegmentResourceKey{resourceID: invocation.ItemID.String(), segmentID: invocation.SegmentID}
		if _, duplicate := seenItems[itemKey]; duplicate {
			return fmt.Errorf(
				"runs: recovery commit repeats Tool invocation Item %q in Segment %q",
				invocation.ItemID,
				invocation.SegmentID,
			)
		}
		seenItems[itemKey] = struct{}{}
		if index > 0 && compareToolInvocationRecoveries(invocations[index-1], invocation) >= 0 {
			return errors.New("runs: recovery commit Tool invocations are not in canonical order")
		}
	}
	return nil
}

func validateRecoveryInvocation(
	attempt InvocationAttempt,
	finishedAt time.Time,
	lostByID map[identity.RunID]rundomain.Replacement,
	recoveredSessions map[identity.SessionID]struct{},
) error {
	if err := attempt.Validate(); err != nil {
		return err
	}
	if _, recovered := recoveredSessions[attempt.SessionID]; !recovered {
		return fmt.Errorf("invocation Session %q is outside this recovery ownership", attempt.SessionID)
	}
	if finishedAt.Before(attempt.StartedAt) {
		return errors.New("invocation finish time precedes start time")
	}
	if recovery, found := lostByID[attempt.RunID]; found {
		expected := recovery.Expected()
		lost := recovery.State()
		if lost.SessionID() != attempt.SessionID || !lost.FinishedAt().Equal(finishedAt) {
			return fmt.Errorf("invocation differs from its recovered lost Run %q", attempt.RunID)
		}
		if !expected.Executes(attempt.SegmentID) {
			return fmt.Errorf(
				"invocation belongs to Segment %q, not recovered active Segment %q for Run %q",
				attempt.SegmentID,
				expected.ActiveSegmentID(),
				attempt.RunID,
			)
		}
	}
	return nil
}

func cloneRecoveryModels(values []ModelInvocationRecovery) []ModelInvocationRecovery {
	cloned := slices.Clone(values)
	for index, value := range cloned {
		if value.Endpoint != nil {
			source := *value.Endpoint
			cloned[index].Endpoint = &source
		}
	}
	return cloned
}
