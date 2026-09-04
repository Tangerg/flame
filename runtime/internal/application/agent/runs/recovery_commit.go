package runs

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/conversation"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	corechat "github.com/Tangerg/scope/core/chat"
)

// Validate proves that a boot-recovery write-set is self-contained and
// owner-bound before its transaction begins.
func (r RecoveryCommit) Validate() error {
	lostByID := make(map[string]rundomain.Run, len(r.LostRuns))
	treeMembers := make(map[string][]rundomain.TreeMember)
	actualOrder := make([]string, 0, len(r.LostRuns))
	for index, run := range r.LostRuns {
		if err := run.Validate(); err != nil {
			return fmt.Errorf("runs: recovery commit lost Run[%d]: %w", index, err)
		}
		outcome, terminal := run.Outcome()
		failure, failed := run.Failure()
		if !terminal || outcome != rundomain.OutcomeLost || !failed || failure.Kind != rundomain.FailureLost {
			return fmt.Errorf("runs: recovery commit Run %q is not a run-lost terminal", run.ID())
		}
		if _, duplicate := lostByID[run.ID()]; duplicate {
			return fmt.Errorf("runs: recovery commit repeats lost Run %q", run.ID())
		}
		lostByID[run.ID()] = run
		rootID := run.Lineage().TreeRootID(run.ID())
		treeMembers[rootID] = append(treeMembers[rootID], rundomain.TreeMember{
			RunID:   run.ID(),
			Lineage: run.Lineage(),
		})
		actualOrder = append(actualOrder, run.ID())
	}
	rootIDs := make([]string, 0, len(treeMembers))
	for rootID := range treeMembers {
		rootIDs = append(rootIDs, rootID)
	}
	slices.Sort(rootIDs)
	lostSessionIDs, err := recoveryLostSessionIDs(rootIDs, lostByID)
	if err != nil {
		return err
	}
	expectedOrder := make([]string, 0, len(r.LostRuns))
	for _, rootID := range rootIDs {
		members := treeMembers[rootID]
		tree, err := rundomain.NewTree(rootID, members)
		if err != nil {
			return fmt.Errorf("runs: recovery commit tree %q: %w", rootID, err)
		}
		expectedOrder = append(expectedOrder, tree.Postorder()...)
	}
	if !slices.Equal(actualOrder, expectedOrder) {
		return errors.New("runs: recovery commit lost Runs are not in canonical tree/postorder")
	}
	if err := validateRecoveryConversationTransitions(
		r.ConversationTransitions,
		rootIDs,
		treeMembers,
		lostByID,
	); err != nil {
		return err
	}
	recoveredSessionIDs, err := recoverySessionIDs(r.PreservedSessionIDs, lostSessionIDs)
	if err != nil {
		return err
	}
	recoveredSessions := make(map[string]struct{}, len(recoveredSessionIDs))
	for _, sessionID := range recoveredSessionIDs {
		recoveredSessions[sessionID] = struct{}{}
	}
	if err := validateRecoveryModelInvocations(r.ModelInvocations, lostByID, recoveredSessions); err != nil {
		return err
	}
	if err := validateRecoveryToolInvocations(r.ToolInvocations, lostByID, recoveredSessions); err != nil {
		return err
	}

	replacedItems := make(map[string]struct{}, len(r.ItemReplacements))
	for index, replacement := range r.ItemReplacements {
		owner, found := lostByID[replacement.Expected.RunID()]
		if !found || replacement.Expected.SessionID() != owner.SessionID() {
			return fmt.Errorf(
				"runs: recovery commit Item %q is not owned by a lost Run",
				replacement.Expected.ID(),
			)
		}
		if err := validateRecoveryItemReplacement(replacement, owner.FinishedAt()); err != nil {
			return fmt.Errorf("runs: recovery commit Item replacement[%d]: %w", index, err)
		}
		if _, duplicate := replacedItems[replacement.Expected.ID()]; duplicate {
			return fmt.Errorf("runs: recovery commit repeats Item replacement %q", replacement.Expected.ID())
		}
		replacedItems[replacement.Expected.ID()] = struct{}{}
	}
	if err := validateRecoveryGoalRuns(r.GoalRuns, lostByID); err != nil {
		return err
	}
	if err := validateRecoveryInterruptDeletions(r.DeleteInterrupts, lostByID); err != nil {
		return err
	}
	if err := validateRecoveryCheckpointDeletions(
		r.DeleteCheckpointSessionIDs,
		lostSessionIDs,
	); err != nil {
		return err
	}
	return nil
}

func validateRecoveryModelInvocations(
	invocations []ModelInvocationRecovery,
	lostByID map[string]rundomain.Run,
	recoveredSessions map[string]struct{},
) error {
	seen := make(map[string]struct{}, len(invocations))
	for index, invocation := range invocations {
		if err := validateRecoveryInvocation(
			invocation.SessionID,
			invocation.RunID,
			invocation.SegmentID,
			invocation.CallID,
			invocation.StartedAt,
			invocation.FinishedAt,
			lostByID,
			recoveredSessions,
		); err != nil {
			return fmt.Errorf("runs: recovery commit model invocation[%d]: %w", index, err)
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
	segmentID  string
}

type recoveryInterruptOwnerKey struct {
	sessionID string
	rootRunID string
}

func validateRecoveryToolInvocations(
	invocations []ToolInvocationRecovery,
	lostByID map[string]rundomain.Run,
	recoveredSessions map[string]struct{},
) error {
	seen := make(map[recoverySegmentResourceKey]struct{}, len(invocations))
	seenItems := make(map[recoverySegmentResourceKey]struct{}, len(invocations))
	for index, invocation := range invocations {
		if err := validateRecoveryInvocation(
			invocation.SessionID,
			invocation.RunID,
			invocation.SegmentID,
			invocation.CallID,
			invocation.StartedAt,
			invocation.FinishedAt,
			lostByID,
			recoveredSessions,
		); err != nil {
			return fmt.Errorf("runs: recovery commit Tool invocation[%d]: %w", index, err)
		}
		if _, err := resourceid.ParseItem(invocation.ItemID); err != nil {
			return fmt.Errorf("runs: recovery commit Tool invocation[%d]: %w", index, err)
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
		itemKey := recoverySegmentResourceKey{resourceID: invocation.ItemID, segmentID: invocation.SegmentID}
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
	sessionID, runID, segmentID, callID string,
	startedAt, finishedAt time.Time,
	lostByID map[string]rundomain.Run,
	recoveredSessions map[string]struct{},
) error {
	if _, err := resourceid.ParseSession(sessionID); err != nil {
		return err
	}
	if _, recovered := recoveredSessions[sessionID]; !recovered {
		return fmt.Errorf("invocation Session %q is outside this recovery ownership", sessionID)
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
	if startedAt.IsZero() || finishedAt.IsZero() {
		return errors.New("invocation start and finish times are required")
	}
	if finishedAt.Before(startedAt) {
		return errors.New("invocation finish time precedes start time")
	}
	if lost, found := lostByID[runID]; found {
		if lost.SessionID() != sessionID || !lost.FinishedAt().Equal(finishedAt) {
			return fmt.Errorf("invocation differs from its recovered lost Run %q", runID)
		}
	}
	return nil
}

func validateRecoveryConversationTransitions(
	transitions []RecoveryConversationTransition,
	rootIDs []string,
	treeMembers map[string][]rundomain.TreeMember,
	lostByID map[string]rundomain.Run,
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
	rootID string,
	transition RecoveryConversationTransition,
	members []rundomain.TreeMember,
	lostByID map[string]rundomain.Run,
) error {
	root := lostByID[rootID]
	if transition.RootRunID != rootID ||
		transition.SessionID != root.SessionID() ||
		transition.ExpectedCount < 0 {
		return fmt.Errorf(
			"runs: recovery commit conversation transition[%d] differs from lost root Run %q",
			index,
			rootID,
		)
	}
	if _, err := resourceid.ParseSession(transition.SessionID); err != nil {
		return fmt.Errorf("runs: recovery commit conversation transition[%d]: %w", index, err)
	}
	if err := validateRecoveryClosureMessages(rootID, transition.Messages); err != nil {
		return err
	}
	messageMark := transition.ExpectedCount + len(transition.Messages)
	for _, member := range members {
		if lostByID[member.RunID].MessageMark() != messageMark {
			return fmt.Errorf(
				"runs: recovery commit lost Run %q message mark differs from its conversation transition",
				member.RunID,
			)
		}
	}
	return nil
}

func validateRecoveryClosureMessages(rootID string, messages []corechat.Message) error {
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
		if err := conversation.ValidateMessageIdentities(message); err != nil {
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
			if result == nil || !result.IsError {
				return fmt.Errorf(
					"runs: recovery commit conversation transition for root Run %q has an invalid Tool result",
					rootID,
				)
			}
			text, textual := result.Output.Text()
			if !textual || text != recoveryLostToolResult {
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

func validateRecoveryItemReplacement(replacement ItemReplacement, finishedAt time.Time) error {
	expected := replacement.Expected
	actual := replacement.Replacement
	if err := expected.Validate(); err != nil {
		return fmt.Errorf("expected Item: %w", err)
	}
	if err := actual.Validate(); err != nil {
		return fmt.Errorf("replacement Item: %w", err)
	}
	if expected.ID() == "" || expected.SessionID() == "" || expected.RunID() == "" {
		return errors.New("expected Item identity is incomplete")
	}
	if expected.Status() != transcript.ItemRunning || actual.Status() != transcript.ItemIncomplete {
		return errors.New("replacement must move a Running Item to Incomplete")
	}
	failure := tool.Failure{
		Kind:   tool.FailureExecution,
		Detail: "tool call interrupted because the run was lost on restart",
	}
	want, err := expected.AbandonToolCall(&failure, finishedAt)
	if err != nil {
		return fmt.Errorf("expected recovery transition: %w", err)
	}
	if !reflect.DeepEqual(actual.Snapshot(), want.Snapshot()) {
		return fmt.Errorf("replacement rewrites facts other than recovery status for Item %q", expected.ID())
	}
	return nil
}

func validateRecoveryGoalRuns(records []goal.RunRecord, lostByID map[string]rundomain.Run) error {
	expected := make(map[string]rundomain.Run)
	for _, run := range lostByID {
		if run.Lineage().IsRoot() && run.GoalIncarnationID() != "" {
			expected[run.ID()] = run
		}
	}
	seen := make(map[string]struct{}, len(records))
	for index, record := range records {
		if err := record.Validate(); err != nil {
			return fmt.Errorf("runs: recovery commit Goal Run[%d]: %w", index, err)
		}
		if _, duplicate := seen[record.RunID]; duplicate {
			return fmt.Errorf("runs: recovery commit repeats Goal Run for Run %q", record.RunID)
		}
		seen[record.RunID] = struct{}{}
		run, found := expected[record.RunID]
		outcome, terminal := run.Outcome()
		if !found || !terminal {
			return fmt.Errorf("runs: recovery commit Goal Run names unowned Run %q", record.RunID)
		}
		cost, err := costFromRunMetrics(run.Metrics())
		if err != nil {
			return fmt.Errorf("runs: recovery commit Goal Run %q cost: %w", run.ID(), err)
		}
		if record.SessionID != run.SessionID() || record.IncarnationID != run.GoalIncarnationID() ||
			record.Outcome != outcome || !record.Cost.Equal(cost) ||
			record.Steps != run.Metrics().Steps() || !record.CompletedAt.Equal(run.FinishedAt()) {
			return fmt.Errorf("runs: recovery commit Goal Run differs from lost Run %q", run.ID())
		}
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("runs: recovery commit has %d Goal Runs, want %d", len(seen), len(expected))
	}
	return nil
}

func validateRecoveryInterruptDeletions(
	values []InterruptOwner,
	lostByID map[string]rundomain.Run,
) error {
	expected := make(map[string]rundomain.Run)
	for _, lost := range lostByID {
		if lost.Lineage().IsRoot() {
			expected[lost.ID()] = lost
		}
	}
	seen := make(map[recoveryInterruptOwnerKey]struct{}, len(values))
	for index, value := range values {
		if _, err := resourceid.ParseSession(value.SessionID); err != nil {
			return fmt.Errorf("runs: recovery commit interrupt deletion[%d]: %w", index, err)
		}
		if _, err := resourceid.ParseRun(value.RootRunID); err != nil {
			return fmt.Errorf("runs: recovery commit interrupt deletion[%d]: %w", index, err)
		}
		key := recoveryInterruptOwnerKey{sessionID: value.SessionID, rootRunID: value.RootRunID}
		if _, duplicate := seen[key]; duplicate {
			return fmt.Errorf("runs: recovery commit repeats interrupt deletion %q/%q", value.SessionID, value.RootRunID)
		}
		seen[key] = struct{}{}
		owner, found := expected[value.RootRunID]
		if !found || owner.SessionID() != value.SessionID {
			return fmt.Errorf(
				"runs: recovery commit interrupt deletion %q/%q is not owned by a lost root Run",
				value.SessionID,
				value.RootRunID,
			)
		}
		if index > 0 {
			previous := values[index-1]
			if previous.SessionID > value.SessionID ||
				(previous.SessionID == value.SessionID && previous.RootRunID >= value.RootRunID) {
				return errors.New("runs: recovery commit interrupt deletions are not in canonical order")
			}
		}
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("runs: recovery commit has %d interrupt deletions, want %d lost roots", len(seen), len(expected))
	}
	return nil
}

func validateRecoveryCheckpointDeletions(
	values []string,
	expected []string,
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

func validateCanonicalSessionIdentities(name string, values []string) error {
	for index, value := range values {
		if _, err := resourceid.ParseSession(value); err != nil {
			return fmt.Errorf("runs: recovery commit %s[%d]: %w", name, index, err)
		}
		if index > 0 && values[index-1] >= value {
			return fmt.Errorf("runs: recovery commit %ss are not unique canonical order", name)
		}
	}
	return nil
}

func recoveryLostSessionIDs(
	rootIDs []string,
	lostByID map[string]rundomain.Run,
) ([]string, error) {
	values := make([]string, len(rootIDs))
	seen := make(map[string]string, len(rootIDs))
	for index, rootID := range rootIDs {
		sessionID := lostByID[rootID].SessionID()
		if otherRoot, duplicate := seen[sessionID]; duplicate {
			return nil, fmt.Errorf(
				"runs: recovery commit lost roots %q and %q share Session %q",
				otherRoot,
				rootID,
				sessionID,
			)
		}
		seen[sessionID] = rootID
		values[index] = sessionID
	}
	slices.Sort(values)
	return values, nil
}

func recoverySessionIDs(preserved, lost []string) ([]string, error) {
	if err := validateCanonicalSessionIdentities("preserved Session", preserved); err != nil {
		return nil, err
	}
	values := append(slices.Clone(lost), preserved...)
	slices.Sort(values)
	for index := 1; index < len(values); index++ {
		if values[index-1] == values[index] {
			return nil, fmt.Errorf("runs: recovery commit both loses and preserves Session %q", values[index])
		}
	}
	return values, nil
}

// RecoveredSessionIDs derives the exact Sessions whose abandoned callback
// ledgers may be retired. Validate must succeed before this projection is used.
func (r RecoveryCommit) RecoveredSessionIDs() []string {
	values := slices.Clone(r.PreservedSessionIDs)
	for _, lost := range r.LostRuns {
		if lost.Lineage().IsRoot() {
			values = append(values, lost.SessionID())
		}
	}
	slices.Sort(values)
	return slices.Compact(values)
}
