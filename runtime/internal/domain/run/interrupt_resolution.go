package run

import (
	"errors"
	"slices"

	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
)

// InterruptResolution is a closed, immutable answer. Question content and Tool approval
// never share fields; the settled transcript Item remains the durable owner.
type InterruptResolution struct {
	question *QuestionAnswer
	approval *approval.Resolution
}

type QuestionAnswer struct{ answers [][]string }

func NewQuestionAnswer(answers [][]string) (InterruptResolution, error) {
	if len(answers) == 0 {
		return InterruptResolution{}, errors.New("interrupt: question answer requires fields")
	}
	owned := make([][]string, len(answers))
	for index, values := range answers {
		owned[index] = append([]string(nil), values...)
	}
	return InterruptResolution{question: &QuestionAnswer{answers: owned}}, nil
}

func NewApprovalAnswer(answer approval.Resolution) (InterruptResolution, error) {
	if err := answer.Validate(); err != nil {
		return InterruptResolution{}, err
	}
	return InterruptResolution{approval: new(answer)}, nil
}

func (r InterruptResolution) Kind() InterruptKind {
	if r.question != nil {
		return InterruptQuestion
	}
	if r.approval != nil {
		return InterruptApproval
	}
	return ""
}

func (r InterruptResolution) Question() *QuestionAnswer {
	if r.question == nil {
		return nil
	}
	return new(*r.question)
}

func (r InterruptResolution) Approval() *approval.Resolution {
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
