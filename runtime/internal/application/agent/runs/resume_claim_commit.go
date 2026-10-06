package runs

import (
	"fmt"
	"github.com/Tangerg/flame/runtime/internal/domain/resourceid"
	"maps"

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
	commitID runtimeidentity.CommitID
	// sessionID is the Session of the parked root Run read before the claim.
	sessionID string
	expected  Pending
	// items are the Items the hand-off names, read before the claim. They own
	// what each answered interrupt asked.
	items   map[string]transcript.Item
	answers []InterruptAnswer
}

// NewResumeClaimCommit binds one validated answer set to the exact waiting
// hand-off and stable transaction identity that consumes it.
func NewResumeClaimCommit(
	commitID runtimeidentity.CommitID,
	sessionID string,
	expected Pending,
	items map[string]transcript.Item,
	answers []InterruptAnswer,
) (ResumeClaimCommit, error) {
	claim := ResumeClaimCommit{
		commitID: commitID, sessionID: sessionID, expected: expected.Clone(), items: maps.Clone(items),
		answers: cloneInterruptAnswers(answers),
	}
	if err := claim.Validate(); err != nil {
		return ResumeClaimCommit{}, err
	}
	return claim, nil
}

func cloneInterruptAnswers(answers []InterruptAnswer) []InterruptAnswer {
	owned := make([]InterruptAnswer, len(answers))
	for index, answer := range answers {
		owned[index] = answer
		owned[index].Resolution.Answers = transcript.CloneAnswers(answer.Resolution.Answers)
	}
	return owned
}

// approvalVerdict is the accepted decision on one reviewed ToolCall and the
// executor call that resumes it.
type approvalVerdict struct {
	callID   string
	decision approval.Decision
}

func (r ResumeClaimCommit) Validate() error {
	if err := r.commitID.Validate(); err != nil {
		return fmt.Errorf("runs: resume claim: %w", err)
	}
	if err := r.expected.Validate(); err != nil {
		return fmt.Errorf("runs: resume claim Pending: %w", err)
	}
	if err := resourceid.ValidateSession(r.sessionID); err != nil {
		return fmt.Errorf("runs: resume claim: %w", err)
	}
	interrupts, err := r.expected.ProjectInterrupts(r.items)
	if err != nil {
		return fmt.Errorf("runs: resume claim: %w", err)
	}
	if len(r.answers) != len(r.expected.Bindings) {
		return fmt.Errorf(
			"runs: resume claim has %d answers for %d boundaries",
			len(r.answers), len(r.expected.Bindings),
		)
	}
	for index, answer := range r.answers {
		binding := r.expected.Bindings[index]
		if answer.InterruptItemID != binding.InterruptItemID || answer.MemberID != binding.MemberID ||
			answer.RequestID != binding.RequestID {
			return fmt.Errorf("runs: resume claim answer[%d] differs from its pending boundary", index)
		}
		if err := answer.validateResolution(interrupts[index]); err != nil {
			return fmt.Errorf("runs: resume claim answer[%d]: %w", index, err)
		}
	}
	if _, err := r.ItemReplacements(); err != nil {
		return fmt.Errorf("runs: resume claim Item projections: %w", err)
	}
	return nil
}

// SessionID returns the Session of the claimed tree's root Run.
func (r ResumeClaimCommit) SessionID() string { return r.sessionID }

// CommitID returns the stable answer-claim transaction identity.
func (r ResumeClaimCommit) CommitID() runtimeidentity.CommitID { return r.commitID }

// Pending returns an isolated snapshot of the exact waiting hand-off consumed by the claim.
func (r ResumeClaimCommit) Pending() Pending { return r.expected.Clone() }

// Answers returns the isolated executor-bound answer set in canonical Pending order.
func (r ResumeClaimCommit) Answers() []InterruptAnswer { return cloneInterruptAnswers(r.answers) }

// ItemReplacements derives the transcript compare-and-swap write-set the claim
// settles: each Question Item read before the claim carries its accepted
// answers, and each reviewed ToolCall carries its verdict. The persistence port
// only executes these replacements in the same transaction as the claim.
func (r ResumeClaimCommit) ItemReplacements() ([]transcript.Replacement, error) {
	answersByItem := make(map[string]InterruptAnswer, len(r.answers))
	for _, answer := range r.answers {
		answersByItem[answer.InterruptItemID] = answer
	}
	replacements := make([]transcript.Replacement, 0, len(r.expected.Interrupts))
	for _, open := range r.expected.Interrupts {
		answer, ok := answersByItem[open.ItemID]
		if !ok {
			return nil, fmt.Errorf("interrupt item %q has no answer", open.ItemID)
		}
		expected, found := r.items[open.ItemID]
		if !found {
			return nil, fmt.Errorf("interrupt item %q was not read", open.ItemID)
		}
		itemReplacement, err := transcript.Replace(expected, func(expected transcript.Item) (transcript.Item, error) {
			switch open.Kind() {
			case interrupt.Question:
				return expected.AnswerQuestion(answer.Resolution.Answers)
			case interrupt.Approval:
				return expected.ResolveToolApproval(approval.DecisionOf(answer.Resolution.Approved))
			default:
				return transcript.Item{}, fmt.Errorf("unknown interrupt kind %q", open.Kind())
			}
		})
		if err != nil {
			return nil, fmt.Errorf("settle interrupt item %q: %w", open.ItemID, err)
		}
		replacements = append(replacements, itemReplacement)
	}
	return replacements, nil
}

// approvalVerdicts names the verdict on every reviewed ToolCall by its Item.
func (r ResumeClaimCommit) approvalVerdicts() map[string]approvalVerdict {
	answersByItem := make(map[string]InterruptAnswer, len(r.answers))
	for _, answer := range r.answers {
		answersByItem[answer.InterruptItemID] = answer
	}
	verdicts := make(map[string]approvalVerdict)
	for index, open := range r.expected.Interrupts {
		if open.Kind() != interrupt.Approval {
			continue
		}
		verdicts[open.ItemID] = approvalVerdict{
			callID:   r.expected.Bindings[index].ToolCallID,
			decision: approval.DecisionOf(answersByItem[open.ItemID].Resolution.Approved),
		}
	}
	return verdicts
}
