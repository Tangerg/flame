package agentexec

import (
	"testing"

	corechat "github.com/Tangerg/scope/core/chat"
)

func TestModelContextCompactionResultDerivesSummarizedFromSummary(t *testing.T) {
	messages := []corechat.Message{corechat.NewUserMessage(corechat.NewTextPart("continue"))}
	if _, err := NewModelContextCompactionResult(messages, false, "summary", 1, 1); err == nil {
		t.Fatal("unchanged model context accepted a summary")
	}
	if _, err := NewModelContextCompactionResult(messages, true, "summary\n", 1, 1); err == nil {
		t.Fatal("model context accepted a non-canonical summary")
	}
	result, err := NewModelContextCompactionResult(messages, true, "summary", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Summarized() || result.Summary() != "summary" {
		t.Fatalf("result = summarized:%t summary:%q", result.Summarized(), result.Summary())
	}
}
