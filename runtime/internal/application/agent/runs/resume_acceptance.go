package runs

import (
	"fmt"
	"maps"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/resourceid"
)

// ResumeAcceptance is the answer write-set an opening transaction accepts
// together with its continuation. It binds one validated answer set to the exact
// waiting hand-off it consumes and derives the transcript compare-and-swap the
// acceptance settles. It carries no transaction identity or receipt: the opening
// it travels with owns both, so the answers, consumed open set, and Segment
// opening commit as one fact.
type ResumeAcceptance struct {
	// sessionID is the Session of the parked root Run read before acceptance.
	sessionID string
	expected  Pending
	// items are the Items the hand-off names, read before acceptance. They own
	// what each answered interrupt asked.
	items   map[string]transcript.Item
	answers []InterruptAnswer
	// rememberedRules are the standing rules derived from the answered approvals
	// and already validated against their frozen source authority. The acceptance
	// transaction persists them atomically with the answers it settles.
	rememberedRules []approval.Rule
}

// NewResumeAcceptance binds one validated answer set to the exact waiting
// hand-off that consumes it.
func NewResumeAcceptance(
	sessionID string,
	expected Pending,
	items map[string]transcript.Item,
	answers []InterruptAnswer,
) (ResumeAcceptance, error) {
	acceptance := ResumeAcceptance{
		sessionID: sessionID, expected: expected.Clone(), items: maps.Clone(items),
		answers: cloneInterruptAnswers(answers),
	}
	if err := acceptance.Validate(); err != nil {
		return ResumeAcceptance{}, err
	}
	return acceptance, nil
}

func cloneInterruptAnswers(answers []InterruptAnswer) []InterruptAnswer {
	owned := make([]InterruptAnswer, len(answers))
	for index, answer := range answers {
		owned[index] = answer
		owned[index].Resolution.Answers = transcript.CloneAnswers(answer.Resolution.Answers)
	}
	return owned
}

func (r ResumeAcceptance) Validate() error {
	if err := r.expected.Validate(); err != nil {
		return fmt.Errorf("runs: resume acceptance Pending: %w", err)
	}
	if err := resourceid.ValidateSession(r.sessionID); err != nil {
		return fmt.Errorf("runs: resume acceptance: %w", err)
	}
	interrupts, err := r.expected.ProjectInterrupts(r.items)
	if err != nil {
		return fmt.Errorf("runs: resume acceptance: %w", err)
	}
	if len(r.answers) != len(r.expected.Bindings) {
		return fmt.Errorf(
			"runs: resume acceptance has %d answers for %d boundaries",
			len(r.answers), len(r.expected.Bindings),
		)
	}
	for index, answer := range r.answers {
		binding := r.expected.Bindings[index]
		if answer.InterruptItemID != binding.InterruptItemID || answer.MemberID != binding.MemberID ||
			answer.RequestID != binding.RequestID {
			return fmt.Errorf("runs: resume acceptance answer[%d] differs from its pending boundary", index)
		}
		if err := answer.validateResolution(interrupts[index]); err != nil {
			return fmt.Errorf("runs: resume acceptance answer[%d]: %w", index, err)
		}
	}
	if _, err := r.ItemReplacements(); err != nil {
		return fmt.Errorf("runs: resume acceptance Item projections: %w", err)
	}
	return nil
}

// SessionID returns the Session of the accepted tree's root Run.
func (r ResumeAcceptance) SessionID() string { return r.sessionID }

// Pending returns an isolated snapshot of the exact waiting hand-off the acceptance consumes.
func (r ResumeAcceptance) Pending() Pending { return r.expected.Clone() }

// Answers returns the isolated executor-bound answer set in canonical Pending order.
func (r ResumeAcceptance) Answers() []InterruptAnswer { return cloneInterruptAnswers(r.answers) }

// WithRememberedRules binds the standing rules the acceptance persists together
// with its answers. Each rule is already derived from a confirmed invocation and
// validated against its frozen source authority, so this only records them on an
// isolated copy of the acceptance.
func (r ResumeAcceptance) WithRememberedRules(rules []approval.Rule) (ResumeAcceptance, error) {
	for index, rule := range rules {
		if err := rule.Validate(); err != nil {
			return ResumeAcceptance{}, fmt.Errorf("runs: resume acceptance rule[%d]: %w", index, err)
		}
	}
	r.rememberedRules = slices.Clone(rules)
	return r, nil
}

// RememberedRules returns an isolated copy of the standing rules the acceptance
// persists in its transaction.
func (r ResumeAcceptance) RememberedRules() []approval.Rule { return slices.Clone(r.rememberedRules) }

// ItemReplacements derives the transcript compare-and-swap write-set the
// acceptance settles: each Question Item read before acceptance carries its
// accepted answers, and each reviewed ToolCall carries its full resolution. The
// persistence port executes these replacements in the same transaction that
// consumes the open set and opens the continuation.
func (r ResumeAcceptance) ItemReplacements() ([]transcript.Replacement, error) {
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
				return expected.ResolveToolApproval(approval.Resolution{
					Decision:        approval.DecisionOf(answer.Resolution.Approved),
					EditedArguments: answer.Resolution.Arguments,
					Reason:          answer.Resolution.Reason,
					RememberScope:   answer.Resolution.RememberScope,
				})
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

// approvalCalls relates each reviewed Item to its executor call. The accepted
// verdict belongs to the committed Item, not to this routing projection.
func (r ResumeAcceptance) approvalCalls() map[string]string {
	calls := make(map[string]string)
	for index, open := range r.expected.Interrupts {
		if open.Kind() != interrupt.Approval {
			continue
		}
		calls[open.ItemID] = r.expected.Bindings[index].ToolCallID
	}
	return calls
}
