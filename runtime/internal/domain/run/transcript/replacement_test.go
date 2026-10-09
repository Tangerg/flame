package transcript

import (
	"errors"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
)

func TestReplaceRequiresOneValidItemIdentity(t *testing.T) {
	at := time.Unix(1, 0).UTC()
	expected, err := NewQuestion(ItemIdentity{
		SessionID: "ses_1", RunID: "run_1", ItemID: "item_1", OccurredAt: at,
	}, Question{Fields: []QuestionField{{Prompt: "Answer", Kind: QuestionText}}})
	if err != nil {
		t.Fatal(err)
	}
	state, err := expected.AnswerQuestion([][]string{{"accepted"}})
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := Replace(expected, func(Item) (Item, error) { return state, nil })
	if err != nil {
		t.Fatal(err)
	}
	if replacement.Expected().ID() != expected.ID() || replacement.State().ID() != state.ID() {
		t.Fatalf("replacement = %+v", replacement)
	}

	foreign, err := NewUserMessage(ItemIdentity{
		SessionID: "ses_1", RunID: "run_2", ItemID: "item_1", OccurredAt: at,
	}, []ContentBlock{{Kind: TextContent, Text: "after"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Replace(expected, func(Item) (Item, error) { return foreign, nil }); !errors.Is(err, ErrIdentityConflict) {
		t.Fatalf("identity error = %v, want ErrIdentityConflict", err)
	}
	if _, err := Replace(Item{}, func(Item) (Item, error) { return state, nil }); err == nil {
		t.Fatal("Replace accepted an invalid expected Item")
	}
}

func TestReplacementRejectsReconstructedTranscriptRewrites(t *testing.T) {
	identity := ItemIdentity{SessionID: "ses_1", RunID: "run_1", ItemID: "item_1", OccurredAt: time.Unix(1, 0).UTC()}
	message, err := NewUserMessage(identity, []ContentBlock{{Kind: TextContent, Text: "original"}})
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := NewQuestion(identity, Question{Fields: []QuestionField{{Prompt: "Answer", Kind: QuestionText}}})
	if err != nil {
		t.Fatal(err)
	}
	answered, err := prompt.AnswerQuestion([][]string{{"accepted"}})
	if err != nil {
		t.Fatal(err)
	}
	started, err := NewToolCall(identity, ToolInvocation{Name: "shell"}, tool.SafetyClassSafe)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := started.ResolveToolApproval(approval.Allow)
	if err != nil {
		t.Fatal(err)
	}
	completed, err := approved.CompleteToolCall(ToolInvocation{Name: "shell"}, identity.OccurredAt, identity.OccurredAt.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name     string
		expected Item
		mutate   func(*ItemSnapshot)
	}{
		{"message content", message, func(snapshot *ItemSnapshot) { snapshot.Content[0].Text = "changed" }},
		{"message kind", message, func(snapshot *ItemSnapshot) { snapshot.Kind = AgentMessage; snapshot.MessagePhase = MessageFinalAnswer }},
		{"occurrence", message, func(snapshot *ItemSnapshot) { snapshot.Identity.OccurredAt = identity.OccurredAt.Add(time.Second) }},
		{"question prompt", prompt, func(snapshot *ItemSnapshot) {
			snapshot.Question.Fields[0].Prompt = "Changed"
			snapshot.Question.Answers = [][]string{{"accepted"}}
		}},
		{"question answer", answered, func(snapshot *ItemSnapshot) { snapshot.Question.Answers = [][]string{{"changed"}} }},
		{"approval verdict", approved, func(snapshot *ItemSnapshot) { snapshot.ApprovalDecision = approval.Deny }},
		{"tool safety", started, func(snapshot *ItemSnapshot) { snapshot.SafetyClass = tool.SafetyClassExec }},
		{"tool resurrection", completed, func(snapshot *ItemSnapshot) {
			snapshot.Status = ItemRunning
			snapshot.FinishedAt = time.Time{}
			snapshot.ExecutionDuration = nil
		}},
		{"tool completion", completed, func(snapshot *ItemSnapshot) { snapshot.FinishedAt = snapshot.FinishedAt.Add(time.Second) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := test.expected.Snapshot()
			test.mutate(&snapshot)
			state, err := RestoreItem(snapshot)
			if err != nil {
				t.Fatalf("reconstructed fixture: %v", err)
			}
			if _, err := Replace(test.expected, func(Item) (Item, error) { return state, nil }); !errors.Is(err, ErrIdentityConflict) {
				t.Fatalf("replacement error = %v, want ErrIdentityConflict", err)
			}
		})
	}
}

func TestReplacementAcceptsToolApprovalAndSettlement(t *testing.T) {
	identity := ItemIdentity{SessionID: "ses_1", RunID: "run_1", ItemID: "item_1", OccurredAt: time.Unix(1, 0).UTC()}
	started, err := NewToolCall(identity, ToolInvocation{Name: "shell"}, tool.SafetyClassSafe)
	if err != nil {
		t.Fatal(err)
	}
	change, err := Replace(started, func(item Item) (Item, error) {
		approved, err := item.ResolveToolApproval(approval.Allow)
		if err != nil {
			return Item{}, err
		}
		return approved.CompleteToolCall(ToolInvocation{Name: "shell"}, identity.OccurredAt, identity.OccurredAt.Add(time.Second))
	})
	if err != nil {
		t.Fatal(err)
	}
	if change.State().Status() != ItemCompleted || change.State().ApprovalDecision() != approval.Allow {
		t.Fatalf("replacement = %+v", change.State().Snapshot())
	}
}
