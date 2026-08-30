package agentexec

import (
	"testing"

	corechat "github.com/Tangerg/scope/core/chat"
)

func TestCompactionResultRequiresCanonicalObservableState(t *testing.T) {
	for _, test := range []struct {
		name    string
		summary string
		before  int
		after   int
	}{
		{name: "empty summary", before: 8, after: 3},
		{name: "whitespace summary", summary: "  ", before: 8, after: 3},
		{name: "non-canonical summary", summary: "summary\n", before: 8, after: 3},
		{name: "empty source", summary: "summary", before: 0, after: 1},
		{name: "empty result", summary: "summary", before: 8, after: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := NewCompactionResult(test.summary, test.before, test.after); err == nil {
				t.Fatal("NewCompactionResult accepted an invalid observable compaction")
			}
		})
	}

	result, err := NewCompactionResult("summary", 8, 3)
	if err != nil {
		t.Fatal(err)
	}
	before, after := result.MessageCounts()
	if !result.Compacted() || result.Summary() != "summary" || before != 8 || after != 3 {
		t.Fatalf("result = compacted:%t summary:%q counts:%d->%d", result.Compacted(), result.Summary(), before, after)
	}
}

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
