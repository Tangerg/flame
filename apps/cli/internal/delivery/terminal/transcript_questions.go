package terminal

import (
	"fmt"

	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
)

type trackedQuestion struct {
	runID string
	id    transcriptEntryID
	block *questionBlock
}

// revealAnsweredQuestion shows the answers a resumed Run re-completed for a
// pending question in place, so the transcript keeps its scroll, selection and
// search.
func (t *transcriptView) revealAnsweredQuestion(block conversation.Block) bool {
	key := conversation.BlockKey(block.RunID, block.ID)
	tracked, pending := t.pendingQuestions[key]
	if !pending || block.Question == nil || !block.Question.Answered() {
		return false
	}
	tracked.block.setQuestion(*block.Question)
	t.projectEntry(tracked.id)
	t.settleEntry(tracked.id)
	delete(t.pendingQuestions, key)
	t.refreshSearch()
	t.announceSelection()
	return true
}

func (t *transcriptView) finishPendingQuestions(runID string) {
	for key, question := range t.pendingQuestions {
		if runID != "" && question.runID != runID {
			continue
		}
		t.settleEntry(question.id)
		delete(t.pendingQuestions, key)
	}
}

// reconcilePendingQuestions closes presentation lifetimes after a cold read.
// An unanswered durable Question remains hidden and retained only when the same
// snapshot exposes it as an open interrupt. Canceled historical questions are
// still intentionally invisible, but must not pin the transcript forever.
func (t *transcriptView) reconcilePendingQuestions(interrupts []conversation.Interrupt) error {
	open := make(map[string]struct{}, len(interrupts))
	for _, interrupt := range interrupts {
		question, ok := interrupt.(conversation.Question)
		if !ok {
			continue
		}
		key := conversation.BlockKey(question.RunID, question.ItemID)
		if _, pending := t.pendingQuestions[key]; !pending {
			return fmt.Errorf("terminal transcript: open question block %s has no pending presentation", question.ItemID)
		}
		open[key] = struct{}{}
	}
	for key, question := range t.pendingQuestions {
		if _, remainsOpen := open[key]; remainsOpen {
			continue
		}
		t.settleEntry(question.id)
		delete(t.pendingQuestions, key)
	}
	return nil
}
