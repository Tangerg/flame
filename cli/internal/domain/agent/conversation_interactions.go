package agent

import "fmt"

type blockReplacement struct {
	at    int
	block Block
}

// RecordAcceptedInteractionAnswers folds the durable part of an acknowledged
// resume command before its continuation segment arrives. Question answers are
// persisted by the runtime at the resume linearization point, but are not
// repeated on the new segment stream; recording them here keeps the live
// projection equivalent to the next authoritative cold read.
//
// The complete waiting set is validated before any block is changed. Approval
// tool results remain stream-owned because their completed items do arrive on
// the continuation segment.
func (c *Conversation) RecordAcceptedInteractionAnswers(responses []InterruptAnswer) ([]Block, error) {
	if c.phase != ConversationWaiting {
		return nil, fmt.Errorf("%w: conversation is not waiting for interaction answers", ErrInvalidTransition)
	}
	if len(responses) != len(c.interactions) {
		return nil, fmt.Errorf(
			"%w: accepted response set has %d answers for %d interactions",
			ErrInvalidTransition, len(responses), len(c.interactions),
		)
	}
	byID, err := indexAcceptedAnswers(responses)
	if err != nil {
		return nil, err
	}
	replacements := make([]blockReplacement, 0, len(c.interactions))
	for _, interaction := range c.interactions {
		itemID := InteractionItemID(interaction)
		answer, exists := byID[itemID]
		if !exists {
			return nil, fmt.Errorf("%w: accepted response is missing item %s", ErrInvalidTransition, itemID)
		}
		if err := ValidateAnswer(interaction, answer); err != nil {
			return nil, fmt.Errorf("%w: accepted response for item %s: %v", ErrInvalidTransition, itemID, err)
		}
		replacement, replace, err := c.acceptedQuestionReplacement(interaction, answer)
		if err != nil {
			return nil, err
		}
		if replace {
			replacements = append(replacements, replacement)
		}
	}

	updated := make([]Block, len(replacements))
	for index, replacement := range replacements {
		c.blocks[replacement.at] = replacement.block.Clone()
		updated[index] = replacement.block.Clone()
	}
	return updated, nil
}

func indexAcceptedAnswers(responses []InterruptAnswer) (map[string]Answer, error) {
	byID := make(map[string]Answer, len(responses))
	for _, response := range responses {
		if _, duplicate := byID[response.ItemID]; duplicate {
			return nil, fmt.Errorf("%w: accepted response repeats item %s", ErrInvalidTransition, response.ItemID)
		}
		byID[response.ItemID] = response.Answer
	}
	return byID, nil
}

func (c *Conversation) acceptedQuestionReplacement(interaction Interaction, answer Answer) (blockReplacement, bool, error) {
	question, isQuestion := interaction.(Question)
	if !isQuestion {
		return blockReplacement{}, false, nil
	}
	itemID := question.ItemID
	response, ok := answer.(QuestionAnswer)
	if !ok {
		return blockReplacement{}, false, fmt.Errorf("%w: item %s does not carry a question answer", ErrInvalidTransition, itemID)
	}
	at, exists := c.index[blockIdentity(question.RunID, itemID)]
	if !exists {
		return blockReplacement{}, false, fmt.Errorf("%w: accepted question references unknown item %s", ErrInvalidTransition, itemID)
	}
	current := c.blocks[at]
	if err := validateInteractionItem(question, current); err != nil {
		return blockReplacement{}, false, fmt.Errorf("%w: accepted question item %s: %v", ErrInvalidTransition, itemID, err)
	}
	accepted, err := question.Accept(response)
	if err != nil {
		return blockReplacement{}, false, fmt.Errorf("%w: accept question item %s: %v", ErrInvalidTransition, itemID, err)
	}
	current.Question = &accepted
	return blockReplacement{at: at, block: current}, true, nil
}
