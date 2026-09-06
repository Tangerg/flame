package runs

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// WaitingSubtreeCancellationRequest carries the complete durable waiting tree
// and the addressed child cancellation into the execution port. Its implementation
// may claim the live tree or restore this exact checkpoint, but cannot reread
// Application persistence.
type WaitingSubtreeCancellationRequest struct {
	continuation   WaitingContinuation
	targetMemberID string
	reason         string
}

// NewWaitingSubtreeCancellationRequest captures one validated cancellation
// request without retaining the caller's continuation slices.
func NewWaitingSubtreeCancellationRequest(
	continuation WaitingContinuation,
	targetMemberID string,
	reason string,
) (WaitingSubtreeCancellationRequest, error) {
	request := WaitingSubtreeCancellationRequest{
		continuation:   continuation.Clone(),
		targetMemberID: targetMemberID,
		reason:         reason,
	}
	if err := request.Validate(); err != nil {
		return WaitingSubtreeCancellationRequest{}, err
	}
	return request, nil
}

// Continuation returns an ownership-independent waiting tree.
func (w WaitingSubtreeCancellationRequest) Continuation() WaitingContinuation {
	return w.continuation.Clone()
}

func (w WaitingSubtreeCancellationRequest) TargetMemberID() string { return w.targetMemberID }
func (w WaitingSubtreeCancellationRequest) Reason() string         { return w.reason }

// NewWaitingContinuation freezes and validates one complete waiting-tree draft.
func NewWaitingContinuation(draft WaitingContinuation) (WaitingContinuation, error) {
	continuation := draft.Clone()
	if err := continuation.Validate(); err != nil {
		return WaitingContinuation{}, err
	}
	return continuation, nil
}

// Clone returns an ownership-independent waiting-tree value.
func (w WaitingContinuation) Clone() WaitingContinuation {
	w.Members = append([]WaitingMember(nil), w.Members...)
	for index := range w.Members {
		w.Members[index].DrainedTools = slices.Clone(w.Members[index].DrainedTools)
	}
	w.Checkpoint = w.Checkpoint.Clone()
	w.Capabilities = w.Capabilities.Clone()
	return w
}

// Validate verifies the Application-owned waiting subtree command without
// interpreting the executor checkpoint payload.
func (w WaitingSubtreeCancellationRequest) Validate() error {
	if err := w.continuation.Validate(); err != nil {
		return fmt.Errorf("runs: waiting subtree continuation: %w", err)
	}
	if _, err := runtimeidentity.ParseMember(w.targetMemberID); err != nil {
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
		if member.ParentRunID == "" {
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
) (WaitingContinuation, error) {
	if err := pending.Validate(); err != nil {
		return WaitingContinuation{}, err
	}
	return NewWaitingContinuation(WaitingContinuation{
		SessionID: pending.SessionID, ExecutorID: pending.ExecutorID,
		RootRunID: pending.RootRunID, Members: waitingMembersFromPending(pending), Checkpoint: checkpoint.Clone(),
		Capabilities:             pending.Capabilities,
		ChildRunAdmissionEnabled: pending.Capabilities.ChildRuns,
	})
}

func waitingMembersFromPending(pending Pending) []WaitingMember {
	members := make([]WaitingMember, len(pending.Continuations))
	for index, continuation := range pending.Continuations {
		members[index] = WaitingMember{
			RunID: continuation.RunID, MemberID: continuation.MemberID,
			ParentRunID:     continuation.Lineage.ParentRunID,
			SpawnedByItemID: continuation.Lineage.SpawnedByItemID,
			ModelSelection:  continuation.ModelSelection, Metrics: continuation.Metrics,
			DrainedTools: continuation.DrainedTools,
		}
	}
	return members
}

// Validate verifies one surviving product member without interpreting executor
// topology or checkpoint payload.
func (w WaitingMember) Validate() error {
	if _, err := resourceid.ParseRun(w.RunID); err != nil {
		return fmt.Errorf("runs: waiting member: %w", err)
	}
	if _, err := runtimeidentity.ParseMember(w.MemberID); err != nil {
		return fmt.Errorf("runs: waiting member: %w", err)
	}
	if (w.ParentRunID == "") != (w.SpawnedByItemID == "") {
		return errors.New("runs: waiting member child lineage is incomplete")
	}
	if w.ParentRunID != "" {
		if _, err := resourceid.ParseRun(w.ParentRunID); err != nil {
			return fmt.Errorf("runs: waiting member parent: %w", err)
		}
		if _, err := resourceid.ParseItem(w.SpawnedByItemID); err != nil {
			return fmt.Errorf("runs: waiting member spawned-by: %w", err)
		}
	}
	if w.ParentRunID == w.RunID {
		return errors.New("runs: waiting member refers to itself as parent")
	}
	if err := w.ModelSelection.ValidateExact(); err != nil {
		return fmt.Errorf("runs: waiting member: %w", err)
	}
	if err := w.Metrics.Validate(); err != nil {
		return fmt.Errorf("runs: waiting member metrics: %w", err)
	}
	if err := validateDrainedTools(w.DrainedTools); err != nil {
		return fmt.Errorf("runs: waiting member tools: %w", err)
	}
	return nil
}

// Validate verifies the complete Application side of one executor continuation.
// The opaque checkpoint payload remains the executor implementation's responsibility.
func (w WaitingContinuation) Validate() error {
	if err := validateWaitingContinuationEnvelope(w); err != nil {
		return err
	}
	topology, err := buildWaitingContinuationTopology(w)
	if err != nil {
		return err
	}
	if err := validateWaitingContinuationOrder(w.Members, topology.tree.Postorder()); err != nil {
		return err
	}
	if len(w.Members) > 1 && !w.Capabilities.ChildRuns {
		return errors.New("runs: waiting continuation has child members without child-Run capability")
	}
	return w.Checkpoint.ValidateOwnership(topology.rootMemberID, w.SessionID)
}

func validateWaitingContinuationEnvelope(continuation WaitingContinuation) error {
	if _, err := resourceid.ParseSession(continuation.SessionID); err != nil {
		return fmt.Errorf("runs: waiting continuation: %w", err)
	}
	if _, err := runtimeidentity.ParseExecutor(continuation.ExecutorID); err != nil {
		return fmt.Errorf("runs: waiting continuation: %w", err)
	}
	if _, err := resourceid.ParseRun(continuation.RootRunID); err != nil {
		return fmt.Errorf("runs: waiting continuation: %w", err)
	}
	if len(continuation.Members) == 0 {
		return errors.New("runs: waiting continuation has no surviving members")
	}
	if err := continuation.Capabilities.Validate(); err != nil {
		return fmt.Errorf("runs: waiting continuation capabilities: %w", err)
	}
	if continuation.ChildRunAdmissionEnabled != continuation.Capabilities.ChildRuns {
		return errors.New("runs: waiting continuation child admission differs from frozen capabilities")
	}
	return nil
}

type waitingContinuationTopology struct {
	rootMemberID string
	tree         run.Tree
}

func buildWaitingContinuationTopology(
	continuation WaitingContinuation,
) (waitingContinuationTopology, error) {
	seenRunIDs := make(map[string]struct{}, len(continuation.Members))
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
			if member.ParentRunID == "" {
				return waitingContinuationTopology{}, fmt.Errorf("runs: waiting child Run %q has no parent", member.RunID)
			}
			lineage = run.Lineage{
				SpawnedByItemID: member.SpawnedByItemID,
				ParentRunID:     member.ParentRunID, RootRunID: continuation.RootRunID,
			}
		} else {
			if member.ParentRunID != "" || rootMemberID != "" {
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

func validateWaitingContinuationOrder(members []WaitingMember, canonicalRunIDs []string) error {
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
