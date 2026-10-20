package interrupt

import (
	"errors"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
)

// Resolution is a closed, immutable answer. Question content and Tool approval
// never share fields; the settled transcript Item remains the durable owner.
type Resolution struct {
	question *QuestionAnswer
	approval *approval.Resolution
}

type QuestionAnswer struct{ answers [][]string }

func NewQuestionAnswer(answers [][]string) (Resolution, error) {
	if len(answers) == 0 {
		return Resolution{}, errors.New("interrupt: question answer requires fields")
	}
	owned := make([][]string, len(answers))
	for index, values := range answers {
		owned[index] = append([]string(nil), values...)
	}
	return Resolution{question: &QuestionAnswer{answers: owned}}, nil
}

func NewApprovalAnswer(answer approval.Resolution) (Resolution, error) {
	if err := answer.Validate(); err != nil {
		return Resolution{}, err
	}
	return Resolution{approval: new(answer)}, nil
}

func (r Resolution) Kind() Kind {
	if r.question != nil {
		return Question
	}
	if r.approval != nil {
		return Approval
	}
	return ""
}

func (r Resolution) Question() *QuestionAnswer { return r.question }

func (r Resolution) Approval() *approval.Resolution {
	if r.approval == nil {
		return nil
	}
	return new(*r.approval)
}

func (q QuestionAnswer) Answers() [][]string {
	answers := make([][]string, len(q.answers))
	for index, values := range q.answers {
		answers[index] = slices.Clone(values)
	}
	return answers
}
