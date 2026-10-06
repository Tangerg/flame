package conversation

import "fmt"

// InstallAnsweredQuestions replaces each waiting Question block with the
// answered Item the Runtime committed when it accepted a resume. Answers are
// committed before the continuation opens and are not repeated on its stream, so
// the Runtime's read of the Session is their only source; the terminal never
// derives them from the command it sent.
func (c *Conversation) InstallAnsweredQuestions(committed []Block) ([]Block, error) {
	if c.Phase() != Waiting {
		return nil, fmt.Errorf("%w: conversation is not waiting for interrupt answers", ErrInvalidTransition)
	}
	byIdentity := make(map[string]Block, len(committed))
	for _, block := range committed {
		byIdentity[blockIdentity(block.RunID, block.ID)] = block
	}
	type replacement struct {
		at    int
		block Block
	}
	replacements := make([]replacement, 0, len(c.interrupts))
	for _, interrupt := range c.interrupts {
		question, isQuestion := interrupt.(Question)
		if !isQuestion {
			continue
		}
		key := blockIdentity(question.RunID, question.ItemID)
		at, exists := c.index[key]
		if !exists {
			return nil, fmt.Errorf("%w: waiting question %s has no transcript block", ErrInvalidTransition, question.ItemID)
		}
		block, found := byIdentity[key]
		if !found || block.Kind != BlockQuestion || block.Question == nil || !block.Question.Answered() {
			return nil, fmt.Errorf("%w: runtime has not committed an answer for question %s", ErrInvalidTransition, question.ItemID)
		}
		replacements = append(replacements, replacement{at: at, block: block.Clone()})
	}
	installed := make([]Block, len(replacements))
	for index, replacement := range replacements {
		c.blocks[replacement.at] = replacement.block
		installed[index] = replacement.block.Clone()
	}
	return installed, nil
}
