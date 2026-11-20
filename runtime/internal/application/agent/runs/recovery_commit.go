package runs

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	corechat "github.com/Tangerg/scope/core/chat"
)

// NewRecoveryCommit constructs one complete, immutable boot-recovery write-set.
func NewRecoveryCommit(input RecoveryCommitInput) (RecoveryCommit, error) {
	state := cloneRecoveryCommitInput(input)
	commit := RecoveryCommit{state: state}
	if err := commit.Validate(); err != nil {
		return RecoveryCommit{}, err
	}
	return commit, nil
}

func cloneRecoveryCommitInput(state RecoveryCommitInput) RecoveryCommitInput {
	state.LostRuns = slices.Clone(state.LostRuns)
	state.ItemReplacements = slices.Clone(state.ItemReplacements)
	state.ConversationTransitions = cloneRecoveryConversationTransitions(state.ConversationTransitions)
	state.ModelInvocations = cloneRecoveryModels(state.ModelInvocations)
	state.IndependentEndpoints = slices.Clone(state.IndependentEndpoints)
	state.ToolInvocations = slices.Clone(state.ToolInvocations)
	state.DeleteInterrupts = slices.Clone(state.DeleteInterrupts)
	state.PreservedSessionIDs = slices.Clone(state.PreservedSessionIDs)
	state.DeleteCheckpointSessionIDs = slices.Clone(state.DeleteCheckpointSessionIDs)
	return state
}

func cloneRecoveryConversationTransitions(
	transitions []RecoveryConversationTransition,
) []RecoveryConversationTransition {
	owned := slices.Clone(transitions)
	for index := range owned {
		owned[index].Messages = cloneCommitMessages(owned[index].Messages)
	}
	return owned
}

// LostRuns returns the exact child-before-parent Run replacements.
func (r RecoveryCommit) LostRuns() []rundomain.Replacement {
	return slices.Clone(r.state.LostRuns)
}

// ItemReplacements returns the Tool Items closed by recovery.
func (r RecoveryCommit) ItemReplacements() []transcript.Replacement {
	return slices.Clone(r.state.ItemReplacements)
}

// ConversationTransitions returns isolated lost-tree context closures.
func (r RecoveryCommit) ConversationTransitions() []RecoveryConversationTransition {
	return cloneRecoveryConversationTransitions(r.state.ConversationTransitions)
}

// ModelInvocations returns abandoned logical attempts with their known physical
// evidence, if any. It never authorizes continuation or provider replay.
func (r RecoveryCommit) ModelInvocations() []ModelInvocationRecovery {
	return cloneRecoveryModels(r.state.ModelInvocations)
}

func (r RecoveryCommit) IndependentEndpoints() []modelref.EndpointObservation {
	return slices.Clone(r.state.IndependentEndpoints)
}

// ToolInvocations returns Tool attempts closed as incomplete.
func (r RecoveryCommit) ToolInvocations() []ToolInvocationRecovery {
	return slices.Clone(r.state.ToolInvocations)
}

// GoalRuns returns the lost Goal-owned root Runs, which the recovery
// transaction also applies to their Sessions' Goals.
func (r RecoveryCommit) GoalRuns() []rundomain.Run {
	var charged []rundomain.Run
	for _, lost := range r.state.LostRuns {
		if value := lost.State(); value.GoalIncarnationID() != "" {
			charged = append(charged, value)
		}
	}
	return charged
}

// DeleteInterrupts returns the lost root-owned Pending records to remove.
func (r RecoveryCommit) DeleteInterrupts() []identity.RunID {
	return slices.Clone(r.state.DeleteInterrupts)
}

// PreservedSessionIDs returns waiting Sessions whose checkpoints remain live.
func (r RecoveryCommit) PreservedSessionIDs() []identity.SessionID {
	return slices.Clone(r.state.PreservedSessionIDs)
}

// DeleteCheckpointSessionIDs returns lost Sessions whose checkpoints are obsolete.
func (r RecoveryCommit) DeleteCheckpointSessionIDs() []identity.SessionID {
	return slices.Clone(r.state.DeleteCheckpointSessionIDs)
}

// Validate proves that a boot-recovery write-set is self-contained and
// owner-bound before its transaction begins.
func (r RecoveryCommit) Validate() error {
	state := r.state
	lost, err := indexLostRuns(state.LostRuns)
	if err != nil {
		return err
	}
	lostSessionIDs := r.lostRootSessionIDs()
	for index := 1; index < len(lostSessionIDs); index++ {
		if lostSessionIDs[index-1] == lostSessionIDs[index] {
			return fmt.Errorf("runs: recovery commit loses two root Runs in Session %q", lostSessionIDs[index])
		}
	}
	if err := validateRecoveryConversationTransitions(state.ConversationTransitions, lost.rootIDs, lost.members, lost.byID); err != nil {
		return err
	}
	if err := validateCanonicalSessionIdentities("preserved Session", state.PreservedSessionIDs); err != nil {
		return err
	}
	recoveredSessions, err := recoveredSessionSet(r.RecoveredSessionIDs())
	if err != nil {
		return err
	}
	if err := validateRecoveryModelInvocations(state.ModelInvocations, lost.byID, recoveredSessions); err != nil {
		return err
	}
	if err := validateIndependentEndpoints(state.IndependentEndpoints, state.ModelInvocations, lost.byID); err != nil {
		return err
	}
	replacedItems, err := indexItemReplacements(state.ItemReplacements, lost.byID)
	if err != nil {
		return err
	}
	if err := validateRecoveryToolInvocations(state.ToolInvocations, lost.byID, recoveredSessions, replacedItems); err != nil {
		return err
	}
	if err := validateRecoveryInterruptDeletions(state.DeleteInterrupts, lost.byID); err != nil {
		return err
	}
	return validateRecoveryCheckpointDeletions(state.DeleteCheckpointSessionIDs, lostSessionIDs)
}

// lostRunTrees indexes lost Runs by identity and by the tree each belongs to.
type lostRunTrees struct {
	byID    map[identity.RunID]rundomain.Replacement
	members map[identity.RunID][]rundomain.TreeMember
	rootIDs []identity.RunID
}

// indexLostRuns admits each lost Run once and proves the set arrives in
// canonical tree postorder, so every child settles before its parent.
func indexLostRuns(lostRuns []rundomain.Replacement) (lostRunTrees, error) {
	trees := lostRunTrees{
		byID:    make(map[identity.RunID]rundomain.Replacement, len(lostRuns)),
		members: make(map[identity.RunID][]rundomain.TreeMember),
	}
	order := make([]identity.RunID, 0, len(lostRuns))
	for index, recovery := range lostRuns {
		if err := validateLostRunReplacement(recovery); err != nil {
			return lostRunTrees{}, fmt.Errorf("runs: recovery commit lost Run[%d]: %w", index, err)
		}
		run := recovery.State()
		if _, duplicate := trees.byID[run.ID()]; duplicate {
			return lostRunTrees{}, fmt.Errorf("runs: recovery commit repeats lost Run %q", run.ID())
		}
		trees.byID[run.ID()] = recovery
		rootID := run.Lineage().TreeRootID(run.ID())
		trees.members[rootID] = append(trees.members[rootID], rundomain.TreeMember{RunID: run.ID(), Lineage: run.Lineage()})
		order = append(order, run.ID())
	}
	trees.rootIDs = slices.SortedFunc(maps.Keys(trees.members), identity.RunID.Compare)
	expected := make([]identity.RunID, 0, len(lostRuns))
	for _, rootID := range trees.rootIDs {
		tree, err := rundomain.NewTree(rootID, trees.members[rootID])
		if err != nil {
			return lostRunTrees{}, fmt.Errorf("runs: recovery commit tree %q: %w", rootID, err)
		}
		expected = append(expected, tree.Postorder()...)
	}
	if !slices.Equal(order, expected) {
		return lostRunTrees{}, errors.New("runs: recovery commit lost Runs are not in canonical tree/postorder")
	}
	return trees, nil
}

func recoveredSessionSet(sessionIDs []identity.SessionID) (map[identity.SessionID]struct{}, error) {
	recovered := make(map[identity.SessionID]struct{}, len(sessionIDs))
	for index, sessionID := range sessionIDs {
		if index > 0 && sessionIDs[index-1] == sessionID {
			return nil, fmt.Errorf("runs: recovery commit both loses and preserves Session %q", sessionID)
		}
		recovered[sessionID] = struct{}{}
	}
	return recovered, nil
}

// validateIndependentEndpoints admits physical model evidence that belongs to
// a lost Run but to no logical attempt being recovered.
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

func indexItemReplacements(replacements []transcript.Replacement, lostByID map[identity.RunID]rundomain.Replacement) (map[identity.ItemID]transcript.Replacement, error) {
	replaced := make(map[identity.ItemID]transcript.Replacement, len(replacements))
	for index, replacement := range replacements {
		expected := replacement.Expected()
		owner, found := lostByID[expected.RunID()]
		if !found || expected.SessionID() != owner.State().SessionID() {
			return nil, fmt.Errorf("runs: recovery commit Item %q is not owned by a lost Run", expected.ID())
		}
		if err := validateAbandonedToolReplacement(replacement, owner.State().FinishedAt()); err != nil {
			return nil, fmt.Errorf("runs: recovery commit Item replacement[%d]: %w", index, err)
		}
		if _, duplicate := replaced[expected.ID()]; duplicate {
			return nil, fmt.Errorf("runs: recovery commit repeats Item replacement %q", expected.ID())
		}
		replaced[expected.ID()] = replacement
	}
	return replaced, nil
}

func validateLostRunReplacement(recovery rundomain.Replacement) error {
	if err := recovery.Validate(); err != nil {
		return err
	}
	if _, lost := recovery.State().Loss(); !lost {
		return errors.New("runs: recovered Run is not lost")
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

func validateRecoveryConversationTransitions(
	transitions []RecoveryConversationTransition,
	rootIDs []identity.RunID,
	treeMembers map[identity.RunID][]rundomain.TreeMember,
	lostByID map[identity.RunID]rundomain.Replacement,
) error {
	if len(transitions) != len(rootIDs) {
		return fmt.Errorf(
			"runs: recovery commit has %d conversation transitions, want %d lost roots",
			len(transitions),
			len(rootIDs),
		)
	}
	for index, rootID := range rootIDs {
		if err := validateRecoveryConversationTransition(
			index, rootID, transitions[index], treeMembers[rootID], lostByID,
		); err != nil {
			return err
		}
	}
	return nil
}

func validateRecoveryConversationTransition(
	index int,
	rootID identity.RunID,
	transition RecoveryConversationTransition,
	members []rundomain.TreeMember,
	lostByID map[identity.RunID]rundomain.Replacement,
) error {
	root := lostByID[rootID].State()
	if transition.RootRunID != rootID ||
		transition.SessionID != root.SessionID() ||
		transition.ExpectedCount < 0 {
		return fmt.Errorf(
			"runs: recovery commit conversation transition[%d] differs from lost root Run %q",
			index,
			rootID,
		)
	}
	if err := transition.SessionID.Validate(); err != nil {
		return fmt.Errorf("runs: recovery commit conversation transition[%d]: %w", index, err)
	}
	if err := validateRecoveryClosureMessages(rootID, transition.Messages); err != nil {
		return err
	}
	messageMark := transition.ExpectedCount + len(transition.Messages)
	for _, member := range members {
		if lostByID[member.RunID].State().MessageMark() != member.Lineage.MessageMark(messageMark) {
			return fmt.Errorf(
				"runs: recovery commit lost Run %q message mark differs from its conversation transition",
				member.RunID,
			)
		}
	}
	return nil
}

func validateRecoveryClosureMessages(rootID identity.RunID, messages []corechat.Message) error {
	if len(messages) > 1 {
		return fmt.Errorf(
			"runs: recovery commit conversation transition for root Run %q has more than one closure message",
			rootID,
		)
	}
	seenToolCalls := make(map[string]struct{})
	for messageIndex, message := range messages {
		if err := message.Validate(); err != nil {
			return fmt.Errorf(
				"runs: recovery commit conversation transition for root Run %q message[%d]: %w",
				rootID,
				messageIndex,
				err,
			)
		}
		if message.Role != corechat.RoleTool {
			return fmt.Errorf(
				"runs: recovery commit conversation transition for root Run %q is not a Tool message",
				rootID,
			)
		}
		for _, part := range message.Parts {
			result := part.ToolResult
			if result == nil {
				return fmt.Errorf(
					"runs: recovery commit conversation transition for root Run %q has an invalid Tool result",
					rootID,
				)
			}
			if _, duplicate := seenToolCalls[result.ID]; duplicate {
				return fmt.Errorf(
					"runs: recovery commit conversation transition for root Run %q repeats ToolCall %q",
					rootID,
					result.ID,
				)
			}
			seenToolCalls[result.ID] = struct{}{}
		}
	}
	return nil
}

func validateRecoveryInterruptDeletions(
	values []identity.RunID,
	lostByID map[identity.RunID]rundomain.Replacement,
) error {
	expected := make(map[identity.RunID]struct{})
	for _, recovery := range lostByID {
		if lost := recovery.State(); lost.Lineage().IsRoot() {
			expected[lost.ID()] = struct{}{}
		}
	}
	for index, rootRunID := range values {
		if err := rootRunID.Validate(); err != nil {
			return fmt.Errorf("runs: recovery commit interrupt deletion[%d]: %w", index, err)
		}
		if _, found := expected[rootRunID]; !found {
			return fmt.Errorf("runs: recovery commit interrupt deletion %q is not owned by a lost root Run", rootRunID)
		}
		if index > 0 && values[index-1].Compare(rootRunID) >= 0 {
			return errors.New("runs: recovery commit interrupt deletions are not in canonical order")
		}
	}
	if len(values) != len(expected) {
		return fmt.Errorf("runs: recovery commit has %d interrupt deletions, want %d lost roots", len(values), len(expected))
	}
	return nil
}

func validateRecoveryCheckpointDeletions(
	values []identity.SessionID,
	expected []identity.SessionID,
) error {
	if err := validateCanonicalSessionIdentities("checkpoint deletion Session", values); err != nil {
		return err
	}
	if !slices.Equal(values, expected) {
		return fmt.Errorf(
			"runs: recovery commit checkpoint deletion Sessions %v differ from lost tree Sessions %v",
			values,
			expected,
		)
	}
	return nil
}

func validateCanonicalSessionIdentities(name string, values []identity.SessionID) error {
	for index, value := range values {
		if err := value.Validate(); err != nil {
			return fmt.Errorf("runs: recovery commit %s[%d]: %w", name, index, err)
		}
		if index > 0 && values[index-1].Compare(value) >= 0 {
			return fmt.Errorf("runs: recovery commit %ss are not unique canonical order", name)
		}
	}
	return nil
}

func (r RecoveryCommit) lostRootSessionIDs() []identity.SessionID {
	values := make([]identity.SessionID, 0, len(r.state.LostRuns))
	for _, recovery := range r.state.LostRuns {
		if lost := recovery.State(); lost.Lineage().IsRoot() {
			values = append(values, lost.SessionID())
		}
	}
	slices.SortFunc(values, identity.SessionID.Compare)
	return values
}

// RecoveredSessionIDs is the one derivation of which Sessions this recovery
// owns, and so of which abandoned callback ledgers may be retired: every
// preserved waiting Session plus the Session of every lost root. Validate
// proves the result carries no repeat, so no reader has to defend against one.
func (r RecoveryCommit) RecoveredSessionIDs() []identity.SessionID {
	values := append(r.lostRootSessionIDs(), r.state.PreservedSessionIDs...)
	slices.SortFunc(values, identity.SessionID.Compare)
	return values
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
