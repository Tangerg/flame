package terminal

import (
	"errors"
	"fmt"
	"slices"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
)

// interactionReview owns the terminal-side draft of a runtime interaction
// batch. Runtime interactions remain immutable; only the answers and cursor
// change until the user commits the complete batch once.
type interactionReview struct {
	items             []conversation.Interaction
	answers           []conversation.Answer
	current           int
	submissionFailure string
}

func newInteractionReview(items []conversation.Interaction) (*interactionReview, error) {
	if err := conversation.ValidateInteractions(items); err != nil {
		return nil, err
	}
	cloned := conversation.CloneInteractions(items)
	return &interactionReview{items: cloned, answers: make([]conversation.Answer, len(cloned))}, nil
}

func restoreInteractionReview(items []conversation.Interaction, responses []conversation.InterruptAnswer) (*interactionReview, error) {
	review, err := newInteractionReview(items)
	if err != nil {
		return nil, err
	}
	if len(responses) != len(items) {
		return nil, errors.New("interaction response count does not match review")
	}
	for index, response := range responses {
		if response.ItemID != conversation.InteractionItemID(items[index]) {
			return nil, fmt.Errorf("interaction response %d targets another item", index+1)
		}
		if err := review.Record(response.Answer); err != nil {
			return nil, fmt.Errorf("restore interaction response %d: %w", index+1, err)
		}
		if !review.Advance() && index+1 < len(responses) {
			return nil, fmt.Errorf("restore interaction response %d did not advance", index+1)
		}
	}
	return review, nil
}

func (i *interactionReview) Current() (conversation.Interaction, bool) {
	if i.current < 0 || i.current >= len(i.items) {
		return nil, false
	}
	return conversation.CloneInteraction(i.items[i.current]), true
}

func (i *interactionReview) CurrentAnswer() conversation.Answer {
	if i.current < 0 || i.current >= len(i.answers) {
		return nil
	}
	return conversation.CloneAnswer(i.answers[i.current])
}

func (i *interactionReview) Record(answer conversation.Answer) error {
	item, ok := i.Current()
	if !ok {
		return errors.New("interaction review has no current item")
	}
	if err := conversation.ValidateAnswer(item, answer); err != nil {
		return err
	}
	i.answers[i.current] = conversation.CloneAnswer(answer)
	return nil
}

func (i *interactionReview) Advance() bool {
	if i.current >= len(i.items) || i.answers[i.current] == nil {
		return false
	}
	i.current++
	return i.current < len(i.items)
}

func (i *interactionReview) Back() bool {
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

func (i *interactionReview) completed() bool {
	return i != nil && len(i.items) > 0 && i.current == len(i.items)
}

func (i *interactionReview) ReportSubmissionFailure(err error) {
	if err == nil {
		return
	}
	i.submissionFailure = err.Error()
}

func (i *interactionReview) SubmissionFailure() string {
	return i.submissionFailure
}

func (i *interactionReview) Reviewing() bool {
	return i != nil && len(i.items) > 1 && i.completed()
}

func (i *interactionReview) Position() (current, total int) {
	return min(i.current+1, len(i.items)), len(i.items)
}

func (i *interactionReview) Responses() ([]conversation.InterruptAnswer, error) {
	if len(i.items) == 0 {
		return nil, errors.New("interaction review is empty")
	}
	responses := make([]conversation.InterruptAnswer, len(i.items))
	for index, item := range i.items {
		if i.answers[index] == nil {
			return nil, fmt.Errorf("interaction %d has no answer", index+1)
		}
		responses[index] = conversation.InterruptAnswer{
			ItemID: conversation.InteractionItemID(item),
			Answer: conversation.CloneAnswer(i.answers[index]),
		}
	}
	return responses, nil
}

func (i *interactionReview) Items() []conversation.Interaction {
	return conversation.CloneInteractions(i.items)
}

func (i *interactionReview) Answers() []conversation.Answer {
	answers := slices.Clone(i.answers)
	for index := range answers {
		answers[index] = conversation.CloneAnswer(answers[index])
	}
	return answers
}
