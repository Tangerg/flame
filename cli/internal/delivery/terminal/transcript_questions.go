package terminal

import (
	"fmt"

	"github.com/Tangerg/flame/cli/internal/domain/conversation"
	"github.com/Tangerg/oolong/components/headless"
)

type trackedQuestion struct {
	runID string
	id    headless.BlockID
	block *questionBlock
}

// revealAnsweredQuestions shows the answers the Runtime committed for pending
// questions in place, so the transcript keeps its scroll, selection and search.
func (t *transcriptView) revealAnsweredQuestions(blocks []conversation.Block) {
	for _, block := range blocks {
		key := transcriptBlockKey(block.RunID, block.ID)
		tracked, pending := t.pendingQuestions[key]
		if !pending {
			continue
		}
		tracked.block.setQuestion(*block.Question)
		t.content.Changed(tracked.id)
		t.content.Finish(tracked.id)
		delete(t.pendingQuestions, key)
	}
	if len(blocks) > 0 {
		t.refreshSearch()
		t.announceSelection()
	}
}

func (t *transcriptView) finishPendingQuestions(runID string) {
	for key, question := range t.pendingQuestions {
		if runID != "" && question.runID != runID {
			continue
		}
		t.content.Finish(question.id)
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
		key := transcriptBlockKey(question.RunID, question.ItemID)
		if _, pending := t.pendingQuestions[key]; !pending {
			return fmt.Errorf("terminal transcript: open question block %s has no pending presentation", question.ItemID)
		}
		open[key] = struct{}{}
	}
	for key, question := range t.pendingQuestions {
		if _, remainsOpen := open[key]; remainsOpen {
			continue
		}
		t.content.Finish(question.id)
		delete(t.pendingQuestions, key)
	}
	return nil
}
