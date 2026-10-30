package transcript

import (
	"testing"
	"time"

	questioncontract "github.com/Tangerg/flame/packages/go-sdk/question"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

func TestTransitionsBindTheirReplacementToTheExpectedItem(t *testing.T) {
	identity := ItemIdentity{SessionID: "ses_1", RunID: "run_1", ItemID: "item_1", OccurredAt: time.Unix(1, 0).UTC()}
	prompt, err := NewQuestion(identity, Question{Fields: []QuestionField{{Prompt: "Answer", Kind: questioncontract.Text}}})
	if err != nil {
		t.Fatal(err)
	}
	answered, err := prompt.AnswerQuestion([][]string{{"accepted"}})
	if err != nil {
		t.Fatal(err)
	}
	if !answered.Expected().Equal(prompt) || answered.State().Equal(prompt) {
		t.Fatalf("answer replacement = %+v", answered)
	}

	started, err := NewToolCall(identity, ToolInvocation{Name: "shell"}, tool.SafetyClassSafe)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := started.ResolveToolApproval(approval.Resolution{Decision: approval.Allow})
	if err != nil {
		t.Fatal(err)
	}
	completed, err := approved.State().CompleteToolCall(ToolInvocation{Name: "shell"}, identity.OccurredAt, identity.OccurredAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !completed.Expected().Equal(approved.State()) || completed.State().Status() != ItemCompleted {
		t.Fatalf("settlement replacement = %+v", completed)
	}
	call, _ := completed.State().ToolCall()
	if call.Approval == nil || call.Approval.Decision != approval.Allow {
		t.Fatalf("settlement lost the approval verdict: %+v", call)
	}
}
