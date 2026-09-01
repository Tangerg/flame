package runs

import (
	"errors"
	"fmt"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
)

// ResumeClaimCommit is the answer linearization write-set. Its transaction
// consumes the exact waiting hand-off and deletes the old checkpoint before an
// executor may be restored or signaled. A crash after this commit therefore
// has no recoverable pre-answer snapshot and boot reconciliation must mark the
// still-nonterminal tree lost.
type ResumeClaimCommit struct {
	// CommitID identifies the complete answer-claim transaction. The checkpoint
	// returned by a successful claim remains a one-shot in-memory hand-off.
	CommitID  runtimeidentity.CommitID
	Expected  Pending
	Answers   []InterruptAnswer
	ClaimedAt time.Time
}

// ClaimedResume is the immutable result of a successful answer claim. The
// checkpoint is returned from the same transaction that made it nonrecoverable;
// callers may hold it only long enough to stage the continuation in this use
// case and never persist it again.
type ClaimedResume struct {
	Pending    Pending
	Answers    []InterruptAnswer
	Checkpoint ExecutorCheckpoint
}

// ToolApprovalResolution is the exact durable ToolCall fact accepted by one
// human answer. The persistence boundary resolves this identity inside the same
// transaction that consumes the Pending barrier.
type ToolApprovalResolution struct {
	Identity   transcript.ItemIdentity
	CallID     string
	Invocation transcript.ToolInvocation
	Decision   approval.Decision
}

func (t ToolApprovalResolution) Validate() error {
	if err := t.Identity.Validate(); err != nil {
		return err
	}
	if _, err := runtimeidentity.ParseEffect(t.CallID); err != nil {
		return fmt.Errorf("runs: approval Tool call: %w", err)
	}
	if err := t.Invocation.Validate(true); err != nil {
		return fmt.Errorf("runs: approval Tool invocation: %w", err)
	}
	if !t.Decision.Valid() {
		return fmt.Errorf("runs: invalid Tool approval decision %q", t.Decision)
	}
	return nil
}

func (r ResumeClaimCommit) Validate() error {
	if err := r.CommitID.Validate(); err != nil {
		return fmt.Errorf("runs: resume claim: %w", err)
	}
	if err := r.Expected.Validate(); err != nil {
		return fmt.Errorf("runs: resume claim Pending: %w", err)
	}
	if r.ClaimedAt.IsZero() {
		return errors.New("runs: resume claim time is required")
	}
	if len(r.Answers) != len(r.Expected.Bindings) {
		return fmt.Errorf(
			"runs: resume claim has %d answers for %d boundaries",
			len(r.Answers), len(r.Expected.Bindings),
		)
	}
	for index, answer := range r.Answers {
		binding := r.Expected.Bindings[index]
		if answer.InterruptItemID != binding.InterruptItemID || answer.MemberID != binding.MemberID ||
			answer.RequestID != binding.RequestID {
			return fmt.Errorf("runs: resume claim answer[%d] differs from its pending boundary", index)
		}
		if err := answer.validateResolution(r.Expected.Interrupts[index]); err != nil {
			return fmt.Errorf("runs: resume claim answer[%d]: %w", index, err)
		}
	}
	if _, err := r.QuestionReplacements(); err != nil {
		return fmt.Errorf("runs: resume claim question projections: %w", err)
	}
	if _, err := r.ToolApprovalResolutions(); err != nil {
		return fmt.Errorf("runs: resume claim Tool approval projections: %w", err)
	}
	return nil
}

// ToolApprovalResolutions derives the durable verdict for every accepted
// approval response from the exact Pending snapshot. It deliberately carries
// the original prompt invocation so the answer claim validates the exact
// reviewed boundary. Edited arguments become the approved execution input and
// may therefore replace the invocation on the terminal Tool Item; Item and
// provider call identities, rather than mutable arguments, preserve continuity.
func (r ResumeClaimCommit) ToolApprovalResolutions() ([]ToolApprovalResolution, error) {
	answersByItem := make(map[string]InterruptAnswer, len(r.Answers))
	bindingsByItem := make(map[string]InterruptBinding, len(r.Expected.Bindings))
	for _, answer := range r.Answers {
		answersByItem[answer.InterruptItemID] = answer
	}
	for _, binding := range r.Expected.Bindings {
		bindingsByItem[binding.InterruptItemID] = binding
	}
	resolutions := make([]ToolApprovalResolution, 0, len(r.Expected.Interrupts))
	for _, request := range r.Expected.Interrupts {
		if request.Kind != interrupt.Approval {
			continue
		}
		if request.Approval == nil {
			return nil, fmt.Errorf("approval item %q has no prompt", request.ItemID)
		}
		answer, ok := answersByItem[request.ItemID]
		if !ok {
			return nil, fmt.Errorf("approval item %q has no answer", request.ItemID)
		}
		binding, ok := bindingsByItem[request.ItemID]
		if !ok {
			return nil, fmt.Errorf("approval item %q has no continuation binding", request.ItemID)
		}
		resolution := ToolApprovalResolution{
			Identity: transcript.ItemIdentity{
				SessionID: r.Expected.SessionID, RunID: request.RunID,
				ItemID: request.ItemID, OccurredAt: request.ItemOccurredAt,
			},
			CallID:     binding.ToolCallID,
			Invocation: request.Approval.Tool,
			Decision:   approval.DecisionOf(answer.Resolution.Approved),
		}
		if err := resolution.Validate(); err != nil {
			return nil, fmt.Errorf("approval item %q: %w", request.ItemID, err)
		}
		resolutions = append(resolutions, resolution)
	}
	return resolutions, nil
}

// QuestionReplacements derives the transcript compare-and-swap write-set for
// every accepted Question answer. It is computed by the Application from the
// exact Pending snapshot and validated resolutions; the persistence port only
// executes these replacements in the same transaction as the claim.
func (r ResumeClaimCommit) QuestionReplacements() ([]ItemReplacement, error) {
	answersByItem := make(map[string]InterruptAnswer, len(r.Answers))
	for _, answer := range r.Answers {
		answersByItem[answer.InterruptItemID] = answer
	}
	replacements := make([]ItemReplacement, 0, len(r.Expected.Interrupts))
	for _, request := range r.Expected.Interrupts {
		if request.Kind != interrupt.Question {
			continue
		}
		if request.Question == nil {
			return nil, fmt.Errorf("question item %q has no prompt", request.ItemID)
		}
		answer, ok := answersByItem[request.ItemID]
		if !ok {
			return nil, fmt.Errorf("question item %q has no answer", request.ItemID)
		}
		expected, err := transcript.NewQuestion(transcript.ItemIdentity{
			SessionID:  r.Expected.SessionID,
			RunID:      request.RunID,
			ItemID:     request.ItemID,
			OccurredAt: request.ItemOccurredAt,
		}, *request.Question)
		if err != nil {
			return nil, fmt.Errorf("restore question item %q: %w", request.ItemID, err)
		}
		replacement, err := expected.AnswerQuestion(answer.Resolution.Answers)
		if err != nil {
			return nil, fmt.Errorf("answer question item %q: %w", request.ItemID, err)
		}
		replacements = append(replacements, ItemReplacement{
			Expected: expected, Replacement: replacement,
		})
	}
	return replacements, nil
}
