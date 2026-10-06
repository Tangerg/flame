package runs

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// treeContinuation is the application-private execution hand-off shared by
// human-driven Resume and a host-settled continuation. It deliberately does not
// mean "an interrupt is open": Pending owns that external-input fact, while a
// tree whose final interrupt was canceled still has enough continuation state
// to open fresh Segments without inventing a fake human answer.
type treeContinuation struct {
	rootRunID        string
	executorID       string
	interrupts       []transcript.Interrupt
	approvalVerdicts map[string]approvalVerdict
	continuations    []Continuation
	// runs are the parked Runs the continuations hand off, by ID. A resumed
	// route reads its admission, accounting and creation from them; the
	// continuation carries none of those facts.
	runs map[string]run.Run
	// items are the Items the hand-off names; they own each drained Tool's
	// occurrence and invocation.
	items map[string]transcript.Item
	// answeredQuestions are the Question Items the resume claim committed with
	// their answers, in the order the hand-off named them.
	answeredQuestions []transcript.Item
}

func (t *treeContinuation) answeredQuestionsFor(runID string) []transcript.Item {
	var answered []transcript.Item
	for _, item := range t.answeredQuestions {
		if item.RunID() == runID {
			answered = append(answered, item)
		}
	}
	return answered
}

// sessionID is the parked root Run's Session.
func (t *treeContinuation) sessionID() string {
	return t.runs[t.rootRunID].SessionID()
}

func parkedRunsByID(parked []run.Run) map[string]run.Run {
	byID := make(map[string]run.Run, len(parked))
	for _, value := range parked {
		byID[value.ID()] = value
	}
	return byID
}

func parkedRoot(rootRunID string, parked []run.Run) (run.Run, bool) {
	for _, value := range parked {
		if value.ID() == rootRunID {
			return value, true
		}
	}
	return run.Run{}, false
}

func (t *treeContinuation) run(runID string) (run.Run, bool) {
	value, found := t.runs[runID]
	return value, found
}

func treeContinuationFromPending(
	pending Pending,
	parked []run.Run,
	itemsByID map[string]transcript.Item,
) (*treeContinuation, error) {
	interrupts, err := pending.ProjectInterrupts(itemsByID)
	if err != nil {
		return nil, err
	}
	continuation := &treeContinuation{
		rootRunID:     pending.RootRunID,
		executorID:    pending.ExecutorID,
		interrupts:    interrupts,
		continuations: slices.Clone(pending.Continuations),
		runs:          parkedRunsByID(parked),
		items:         maps.Clone(itemsByID),
	}
	if err := continuation.validate(); err != nil {
		return nil, err
	}
	return continuation, nil
}

// validate proves one constructed continuation. Whether a Segment has one at
// all is the caller's question, asked where a fresh Run and a resumed tree
// diverge; every path here holds a value newTreeContinuation returned.
func (t *treeContinuation) validate() error {
	if err := resourceid.ValidateRun(t.rootRunID); err != nil {
		return fmt.Errorf("runs: tree continuation root: %w", err)
	}
	if _, parked := t.runs[t.rootRunID]; !parked {
		return errors.New("runs: tree continuation root Run is not parked")
	}
	if err := runtimeidentity.ValidateExecutor(t.executorID); err != nil {
		return fmt.Errorf("runs: tree continuation: %w", err)
	}
	switch {
	case len(t.continuations) == 0:
		return errors.New("runs: tree continuation has no Runs")
	}

	runIDs := make(map[string]struct{}, len(t.continuations))
	memberOwners := make(map[string]string, len(t.continuations))
	members := make([]run.TreeMember, 0, len(t.continuations))
	for index, member := range t.continuations {
		if err := member.Validate(); err != nil {
			return fmt.Errorf("runs: tree continuation Run[%d]: %w", index, err)
		}
		if _, duplicate := runIDs[member.RunID]; duplicate {
			return fmt.Errorf("runs: tree continuation repeats Run %q", member.RunID)
		}
		if owner, duplicate := memberOwners[member.MemberID]; duplicate {
			return fmt.Errorf(
				"runs: tree continuation member %q belongs to Runs %q and %q",
				member.MemberID,
				owner,
				member.RunID,
			)
		}
		parked, found := t.runs[member.RunID]
		if !found {
			return fmt.Errorf("runs: tree continuation Run %q is not a parked Run", member.RunID)
		}
		runIDs[member.RunID] = struct{}{}
		memberOwners[member.MemberID] = member.RunID
		members = append(members, run.TreeMember{
			RunID:   member.RunID,
			Lineage: parked.Lineage(),
		})
	}
	tree, err := run.NewTree(t.rootRunID, members)
	if err != nil {
		return fmt.Errorf("runs: tree continuation topology: %w", err)
	}
	canonical := tree.Postorder()
	for index, member := range t.continuations {
		if member.RunID != canonical[index] {
			return fmt.Errorf(
				"runs: tree continuation Run[%d] is %q, canonical postorder requires %q",
				index,
				member.RunID,
				canonical[index],
			)
		}
	}
	for index, interrupt := range t.interrupts {
		if interrupt.ItemID == "" || interrupt.RunID == "" {
			return fmt.Errorf("runs: tree continuation interrupt[%d] has incomplete identity", index)
		}
		if _, exists := runIDs[interrupt.RunID]; !exists {
			return fmt.Errorf(
				"runs: tree continuation interrupt[%d] names removed Run %q",
				index,
				interrupt.RunID,
			)
		}
	}
	return nil
}

func (t *treeContinuation) root() (Continuation, bool) {
	for _, member := range t.continuations {
		if member.RunID == t.rootRunID {
			return member, true
		}
	}
	return Continuation{}, false
}

func (t *treeContinuation) forRun(runID string) (Continuation, bool) {
	for _, member := range t.continuations {
		if member.RunID == runID {
			return member, true
		}
	}
	return Continuation{}, false
}
