package runs

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
)

type WaitingSubtreeCancellationRequest struct {
	continuation   ExecutionContinuation
	targetMemberID string
	reason         string
}

func (r WaitingSubtreeCancellationRequest) executorRef() ExecutorRef {
	return ExecutorRef{SessionID: r.continuation.SessionID, ExecutorID: r.continuation.ExecutorID}
}

func NewWaitingSubtreeCancellationRequest(
	continuation ExecutionContinuation,
	targetMemberID string,
	reason string,
) (WaitingSubtreeCancellationRequest, error) {
	request := WaitingSubtreeCancellationRequest{
		continuation:   continuation.Clone(),
		targetMemberID: targetMemberID,
		reason:         reason,
	}
	if err := request.validate(); err != nil {
		return WaitingSubtreeCancellationRequest{}, err
	}
	return request, nil
}

func (w WaitingSubtreeCancellationRequest) Continuation() ExecutionContinuation {
	return w.continuation.Clone()
}

func (w WaitingSubtreeCancellationRequest) TargetMemberID() string { return w.targetMemberID }
func (w WaitingSubtreeCancellationRequest) Reason() string         { return w.reason }

func NewExecutionContinuation(draft ExecutionContinuation) (ExecutionContinuation, error) {
	continuation := draft.Clone()
	if err := continuation.Validate(); err != nil {
		return ExecutionContinuation{}, err
	}
	return continuation, nil
}

func (w ExecutionContinuation) Clone() ExecutionContinuation {
	w.Members = append([]ExecutionMember(nil), w.Members...)
	for index := range w.Members {
		w.Members[index].DrainedTools = slices.Clone(w.Members[index].DrainedTools)
	}
	w.Checkpoint = w.Checkpoint.Clone()
	w.Capabilities = w.Capabilities.Clone()
	return w
}

func (w WaitingSubtreeCancellationRequest) validate() error {
	if err := w.continuation.Validate(); err != nil {
		return fmt.Errorf("runs: waiting subtree continuation: %w", err)
	}
	if err := ValidateMember(w.targetMemberID); err != nil {
		return fmt.Errorf("runs: waiting subtree target: %w", err)
	}
	if strings.TrimSpace(w.reason) == "" || w.reason != strings.TrimSpace(w.reason) {
		return errors.New("runs: waiting subtree reason is required without surrounding whitespace")
	}
	targetFound := false
	for _, member := range w.continuation.Members {
		if member.MemberID != w.targetMemberID {
			continue
		}
		if member.ParentRunID.IsZero() {
			return errors.New("runs: waiting subtree target is the root member")
		}
		targetFound = true
		break
	}
	if !targetFound {
		return errors.New("runs: waiting subtree target is absent from the continuation")
	}
	return nil
}

func waitingContinuationFromPending(
	pending Pending,
	checkpoint ExecutorCheckpoint,
	parked []run.Run,
	sess session.Session,
) (ExecutionContinuation, error) {
	if err := pending.Validate(); err != nil {
		return ExecutionContinuation{}, err
	}
	members, err := waitingMembersFromPending(pending, parked)
	if err != nil {
		return ExecutionContinuation{}, err
	}
	root, found := parkedRunsByID(parked)[pending.RootRunID]
	if !found {
		return ExecutionContinuation{}, fmt.Errorf("runs: waiting root Run %q is not parked", pending.RootRunID)
	}
	return NewExecutionContinuation(ExecutionContinuation{
		SessionID: root.SessionID(), ExecutorID: pending.ExecutorID,
		RootRunID: pending.RootRunID, Members: members, Checkpoint: checkpoint.Clone(),
		Capabilities:      root.Capabilities(),
		GoalIncarnationID: root.GoalIncarnationID(),
		Workspace:         sess.Workspace().Path(),
		Project:           sess.Project(),
		Isolated:          sess.Isolated(),
	})
}

func waitingMembersFromPending(pending Pending, parked []run.Run) ([]ExecutionMember, error) {
	runs := parkedRunsByID(parked)
	members := make([]ExecutionMember, len(pending.Continuations))
	for index, continuation := range pending.Continuations {
		value, found := runs[continuation.RunID]
		if !found {
			return nil, fmt.Errorf("runs: waiting member Run %q is not parked", continuation.RunID)
		}
		members[index] = ExecutionMember{
			RunID: continuation.RunID, MemberID: continuation.MemberID,
			ParentRunID:     value.Lineage().ParentRunID(),
			SpawnedByItemID: value.Lineage().SpawnedByItemID(),
			ModelSelection:  value.ModelSelection(),
			DrainedTools:    continuation.DrainedTools,
		}
	}
	return members, nil
}

func (w ExecutionMember) Validate() error {
	if err := w.RunID.Validate(); err != nil {
		return fmt.Errorf("runs: waiting member: %w", err)
	}
	if err := ValidateMember(w.MemberID); err != nil {
		return fmt.Errorf("runs: waiting member: %w", err)
	}
	if (w.ParentRunID.IsZero()) != (w.SpawnedByItemID.IsZero()) {
		return errors.New("runs: waiting member child lineage is incomplete")
	}
	if !w.ParentRunID.IsZero() {
		if err := w.ParentRunID.Validate(); err != nil {
			return fmt.Errorf("runs: waiting member parent: %w", err)
		}
		if err := w.SpawnedByItemID.Validate(); err != nil {
			return fmt.Errorf("runs: waiting member spawned-by: %w", err)
		}
	}
	if w.ParentRunID == w.RunID {
		return errors.New("runs: waiting member refers to itself as parent")
	}
	if err := w.ModelSelection.ValidateExact(); err != nil {
		return fmt.Errorf("runs: waiting member: %w", err)
	}
	if err := validateDrainedTools(w.DrainedTools); err != nil {
		return fmt.Errorf("runs: waiting member tools: %w", err)
	}
	return nil
}

func (w ExecutionContinuation) Validate() error {
	if err := validateExecutionContinuationEnvelope(w); err != nil {
		return err
	}
	topology, err := buildExecutionContinuationTopology(w)
	if err != nil {
		return err
	}
	if err := validateExecutionContinuationOrder(w.Members, topology.tree.Postorder()); err != nil {
		return err
	}
	if len(w.Members) > 1 && !w.Capabilities.ChildRuns {
		return errors.New("runs: waiting continuation has child members without child-Run capability")
	}
	return w.Checkpoint.ValidateOwnership(topology.rootMemberID, w.SessionID)
}

func validateExecutionContinuationEnvelope(continuation ExecutionContinuation) error {
	if err := continuation.SessionID.Validate(); err != nil {
		return fmt.Errorf("runs: waiting continuation: %w", err)
	}
	if err := ValidateExecutor(continuation.ExecutorID); err != nil {
		return fmt.Errorf("runs: waiting continuation: %w", err)
	}
	if err := continuation.RootRunID.Validate(); err != nil {
		return fmt.Errorf("runs: waiting continuation: %w", err)
	}
	if len(continuation.Members) == 0 {
		return errors.New("runs: waiting continuation has no surviving members")
	}
	if err := continuation.Capabilities.Validate(); err != nil {
		return fmt.Errorf("runs: waiting continuation capabilities: %w", err)
	}
	if strings.TrimSpace(continuation.Workspace) == "" || continuation.Workspace != strings.TrimSpace(continuation.Workspace) {
		return errors.New("runs: waiting continuation workspace is required without surrounding whitespace")
	}
	return nil
}

type waitingContinuationTopology struct {
	rootMemberID string
	tree         run.Tree
}

func buildExecutionContinuationTopology(
	continuation ExecutionContinuation,
) (waitingContinuationTopology, error) {
	seenRunIDs := make(map[identity.RunID]struct{}, len(continuation.Members))
	seenMemberIDs := make(map[string]struct{}, len(continuation.Members))
	treeMembers := make([]run.TreeMember, 0, len(continuation.Members))
	rootMemberID := ""
	for index, member := range continuation.Members {
		if err := member.Validate(); err != nil {
			return waitingContinuationTopology{}, fmt.Errorf("runs: waiting continuation member[%d]: %w", index, err)
		}
		if _, duplicate := seenRunIDs[member.RunID]; duplicate {
			return waitingContinuationTopology{}, fmt.Errorf("runs: waiting continuation repeats Run %q", member.RunID)
		}
		if _, duplicate := seenMemberIDs[member.MemberID]; duplicate {
			return waitingContinuationTopology{}, fmt.Errorf("runs: waiting continuation repeats member %q", member.MemberID)
		}
		seenRunIDs[member.RunID] = struct{}{}
		seenMemberIDs[member.MemberID] = struct{}{}
		lineage := run.Lineage{}
		if member.RunID != continuation.RootRunID {
			if member.ParentRunID.IsZero() {
				return waitingContinuationTopology{}, fmt.Errorf("runs: waiting child Run %q has no parent", member.RunID)
			}
			child, err := run.NewLineage(member.SpawnedByItemID, member.ParentRunID, continuation.RootRunID)
			if err != nil {
				return waitingContinuationTopology{}, fmt.Errorf("runs: waiting child Run %q: %w", member.RunID, err)
			}
			lineage = child
		} else {
			if !member.ParentRunID.IsZero() || rootMemberID != "" {
				return waitingContinuationTopology{}, errors.New("runs: waiting continuation has an invalid root member")
			}
			rootMemberID = member.MemberID
		}
		treeMembers = append(treeMembers, run.TreeMember{RunID: member.RunID, Lineage: lineage})
	}
	if rootMemberID == "" {
		return waitingContinuationTopology{}, errors.New("runs: waiting continuation has no root member")
	}
	tree, err := run.NewTree(continuation.RootRunID, treeMembers)
	if err != nil {
		return waitingContinuationTopology{}, fmt.Errorf("runs: waiting continuation product tree: %w", err)
	}
	return waitingContinuationTopology{rootMemberID: rootMemberID, tree: tree}, nil
}

func validateExecutionContinuationOrder(members []ExecutionMember, canonicalRunIDs []identity.RunID) error {
	for index, member := range members {
		if member.RunID != canonicalRunIDs[index] {
			return fmt.Errorf(
				"runs: waiting continuation member[%d] is Run %q, canonical postorder requires %q",
				index, member.RunID, canonicalRunIDs[index],
			)
		}
	}
	return nil
}

func (w ExecutionContinuation) RootModelSelection() flamemodel.Selection {
	for _, member := range w.Members {
		if member.RunID == w.RootRunID {
			return member.ModelSelection
		}
	}
	return flamemodel.Selection{}
}
