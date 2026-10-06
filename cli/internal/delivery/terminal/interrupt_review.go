package terminal

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
)

// interruptReview owns the terminal-side draft of a runtime interrupt
// batch. Runtime interrupts remain immutable; only the answers and cursor
// change until the user commits the complete batch once.
type interruptReview struct {
	items             []conversation.Interrupt
	answers           []conversation.Answer
	current           int
	submissionFailure string
}

func newInterruptReview(items []conversation.Interrupt) (*interruptReview, error) {
	if err := conversation.ValidateInterrupts(items); err != nil {
		return nil, err
	}
	cloned := conversation.CloneInterrupts(items)
	return &interruptReview{items: cloned, answers: make([]conversation.Answer, len(cloned))}, nil
}

func restoreInterruptReview(items []conversation.Interrupt, responses []conversation.InterruptAnswer) (*interruptReview, error) {
	review, err := newInterruptReview(items)
	if err != nil {
		return nil, err
	}
	if len(responses) != len(items) {
		return nil, errors.New("interrupt response count does not match review")
	}
	for index, response := range responses {
		if response.ItemID != conversation.InterruptItemID(items[index]) {
			return nil, fmt.Errorf("interrupt response %d targets another item", index+1)
		}
		if err := review.Record(response.Answer); err != nil {
			return nil, fmt.Errorf("restore interrupt response %d: %w", index+1, err)
		}
		if !review.Advance() && index+1 < len(responses) {
			return nil, fmt.Errorf("restore interrupt response %d did not advance", index+1)
		}
	}
	return review, nil
}

func (i *interruptReview) Current() (conversation.Interrupt, bool) {
	if i.current < 0 || i.current >= len(i.items) {
		return nil, false
	}
	return conversation.CloneInterrupt(i.items[i.current]), true
}

func (i *interruptReview) CurrentAnswer() conversation.Answer {
	if i.current < 0 || i.current >= len(i.answers) {
		return nil
	}
	return conversation.CloneAnswer(i.answers[i.current])
}

func (i *interruptReview) Record(answer conversation.Answer) error {
	item, ok := i.Current()
	if !ok {
		return errors.New("interrupt review has no current item")
	}
	if err := conversation.ValidateAnswer(item, answer); err != nil {
		return err
	}
	i.answers[i.current] = conversation.CloneAnswer(answer)
	return nil
}

func (i *interruptReview) Advance() bool {
	if i.current >= len(i.items) || i.answers[i.current] == nil {
		return false
	}
	i.current++
	return i.current < len(i.items)
}

func (i *interruptReview) Back() bool {
	if i.current <= 0 {
		return false
	}
	if i.current >= len(i.items) {
		i.current = len(i.items) - 1
	} else {
		i.current--
	}
	return true
}

func (i *interruptReview) completed() bool {
	return i != nil && len(i.items) > 0 && i.current == len(i.items)
}

func (i *interruptReview) ReportSubmissionFailure(err error) {
	if err == nil {
		return
	}
	i.submissionFailure = err.Error()
}

func (i *interruptReview) SubmissionFailure() string {
	return i.submissionFailure
}

func (i *interruptReview) Reviewing() bool {
	return i != nil && len(i.items) > 1 && i.completed()
}

func (i *interruptReview) Position() (current, total int) {
	return min(i.current+1, len(i.items)), len(i.items)
}

func (i *interruptReview) Responses() ([]conversation.InterruptAnswer, error) {
	if len(i.items) == 0 {
		return nil, errors.New("interrupt review is empty")
	}
	responses := make([]conversation.InterruptAnswer, len(i.items))
	for index, item := range i.items {
		if i.answers[index] == nil {
			return nil, fmt.Errorf("interrupt %d has no answer", index+1)
		}
		responses[index] = conversation.InterruptAnswer{
			ItemID: conversation.InterruptItemID(item),
			Answer: conversation.CloneAnswer(i.answers[index]),
		}
	}
	return responses, nil
}

func (i *interruptReview) Items() []conversation.Interrupt {
	return conversation.CloneInterrupts(i.items)
}

func (i *interruptReview) Answers() []conversation.Answer {
	answers := slices.Clone(i.answers)
	for index := range answers {
		answers[index] = conversation.CloneAnswer(answers[index])
	}
	return answers
}
