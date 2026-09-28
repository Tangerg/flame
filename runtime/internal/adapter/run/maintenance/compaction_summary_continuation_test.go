package maintenance

import (
	"testing"

	"github.com/Tangerg/scope/core/chat"
)

func TestSummaryContinuationRemainsFoldable(t *testing.T) {
	messages := []chat.Message{
		chat.NewUserMessage(chat.NewTextPart("continue until finished")),
		chat.NewAssistantMessage(chat.NewTextPart("work before the first summary")),
	}
	for iteration := range 3 {
		cutoff := summaryCutoffWithProtectedTail(messages, 0)
		if cutoff != len(messages) {
			t.Fatalf("summary %d cutoff = %d, want all %d foldable messages", iteration, cutoff, len(messages))
		}
		messages = []chat.Message{
			chat.NewSystemMessage(compactionModelPrefix + "retained user intent and progress"),
			chat.NewAssistantMessage(chat.NewTextPart("more work without another user turn")),
		}
	}
}

func TestSummaryContinuationPreservesOwnedTail(t *testing.T) {
	messages := []chat.Message{
		chat.NewSystemMessage(compactionModelPrefix + "earlier turn"),
		chat.NewAssistantMessage(chat.NewTextPart("foldable continuation")),
		chat.NewAssistantMessage(chat.NewTextPart("protected continuation")),
	}
	for _, test := range []struct {
		tail int
		want int
	}{{0, 3}, {1, 2}, {3, 0}, {-1, 0}, {4, 0}} {
		if got := summaryCutoffWithProtectedTail(messages, test.tail); got != test.want {
			t.Fatalf("tail %d: cutoff = %d, want %d", test.tail, got, test.want)
		}
	}
}
