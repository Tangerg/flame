package transcript_test

import (
	"strings"
	"testing"
	"time"

	flameidentity "github.com/Tangerg/flame/packages/go-sdk/identity"
	questioncontract "github.com/Tangerg/flame/packages/go-sdk/question"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func TestItemIdentityRejectsNonCanonicalOrUnboundedResourceIdentity(t *testing.T) {
	valid := itemIdentity()
	for name, mutate := range map[string]func(*transcript.ItemIdentity){
		"session control": func(identity *transcript.ItemIdentity) { identity.SessionID = "session\n1" },
		"run padding":     func(identity *transcript.ItemIdentity) { identity.RunID = " run-1" },
		"item oversized": func(identity *transcript.ItemIdentity) {
			identity.ItemID = strings.Repeat("i", flameidentity.MaximumResourceCharacters+1)
		},
	} {
		t.Run(name, func(t *testing.T) {
			identity := valid
			mutate(&identity)
			if err := identity.Validate(); err == nil {
				t.Fatalf("invalid identity was accepted: %+v", identity)
			}
		})
	}
}

func TestToolSettlementPreservesTheCapturedInstallation(t *testing.T) {
	source, err := tool.ParseRef("mcp:installation:eeb329cd-c7ce-40c9-bd90-6821fef06d30:web:web_search")
	if err != nil {
		t.Fatal(err)
	}
	other, err := tool.ParseRef("mcp:installation:8ad9abf5-3a7d-4d0b-bef9-6ef92c20e746:web:web_search")
	if err != nil {
		t.Fatal(err)
	}
	invocation := transcript.ToolInvocation{Name: source.ModelName(), Source: source}
	running, err := transcript.NewToolCall(itemIdentity(), invocation, tool.SafetyClassNetwork)
	if err != nil {
		t.Fatal(err)
	}
	finish := running.OccurredAt().Add(time.Second)
	invocation.Source = other
	if _, err := running.CompleteToolCall(invocation, running.OccurredAt(), finish); err == nil {
		t.Fatal("a different source completed the same unqualified name")
	}
	invocation.Source = tool.Ref{}
	resolved, err := running.CompleteToolCall(invocation, running.OccurredAt(), finish)
	if err != nil || toolCall(t, resolved.State()).Invocation.Source != source {
		t.Fatalf("settlement lost the captured installation: %v", err)
	}
}

func TestToolApprovalDecisionIsImmutableAndSurvivesSettlement(t *testing.T) {
	running, err := transcript.NewToolCall(
		itemIdentity(), transcript.ToolInvocation{Name: "shell"}, tool.SafetyClassExec,
	)
	if err != nil {
		t.Fatalf("NewToolCall: %v", err)
	}
	resolution, err := running.ResolveToolApproval(approval.Resolution{Decision: approval.Allow})
	if err != nil {
		t.Fatalf("ResolveToolApproval: %v", err)
	}
	resolved := resolution.State()
	if approvalDecision(running) != "" || approvalDecision(resolved) != approval.Allow {
		t.Fatalf("approval decisions = original %q, resolved %q", approvalDecision(running), approvalDecision(resolved))
	}
	if _, resolveToolApprovalErr := resolved.ResolveToolApproval(approval.Resolution{Decision: approval.Deny}); resolveToolApprovalErr == nil {
		t.Fatal("ResolveToolApproval accepted a second decision")
	}
	result, err := tool.NewResult("ok")
	if err != nil {
		t.Fatal(err)
	}
	completed, err := resolved.CompleteToolCall(
		transcript.ToolInvocation{Name: "shell", Result: &result},
		resolved.OccurredAt(),
		resolved.OccurredAt().Add(time.Second),
	)
	if err != nil {
		t.Fatalf("CompleteToolCall: %v", err)
	}
	if approvalDecision(completed.State()) != approval.Allow {
		t.Fatalf("completed approval decision = %q, want %q", approvalDecision(completed.State()), approval.Allow)
	}
	if restored, err := transcript.RestoreItem(completed.State().Snapshot()); err != nil {
		t.Fatalf("RestoreItem: %v", err)
	} else if approvalDecision(restored) != approval.Allow {
		t.Fatalf("restored approval decision = %q, want %q", approvalDecision(restored), approval.Allow)
	}
}

func approvalDecision(item transcript.Item) approval.Decision {
	call, _ := item.ToolCall()
	if call.Approval == nil {
		return ""
	}
	return call.Approval.Decision
}

func toolCall(t *testing.T, item transcript.Item) transcript.ToolCallBody {
	t.Helper()
	call, ok := item.ToolCall()
	if !ok {
		t.Fatalf("Item %q is %s, not a ToolCall", item.ID(), item.Kind())
	}
	return call
}

func itemIdentity() transcript.ItemIdentity {
	location := time.FixedZone("fixture", 8*60*60)
	return transcript.ItemIdentity{
		SessionID:  "session-1",
		RunID:      "run-1",
		ItemID:     "item-1",
		OccurredAt: time.Date(2026, 8, 10, 12, 0, 0, 0, location),
	}
}

func TestItemConstructorsCloseEveryVariant(t *testing.T) {
	identity := itemIdentity()
	constructors := []struct {
		name string
		kind transcript.ItemKind
		new  func() (transcript.Item, error)
	}{
		{
			name: "user message", kind: transcript.UserMessage,
			new: func() (transcript.Item, error) {
				return transcript.NewUserMessage(identity, []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "hello"}})
			},
		},
		{
			name: "agent message", kind: transcript.AgentMessage,
			new: func() (transcript.Item, error) {
				return transcript.NewAgentMessage(identity, transcript.MessageFinalAnswer, []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "hello"}})
			},
		},
		{
			name: "reasoning", kind: transcript.Reasoning,
			new: func() (transcript.Item, error) {
				return transcript.NewReasoning(identity, "consider the evidence", true)
			},
		},
		{
			name: "question", kind: transcript.QuestionItem,
			new: func() (transcript.Item, error) {
				return transcript.NewQuestion(identity, transcript.Question{Fields: []transcript.QuestionField{{
					Prompt: "Continue?", Kind: questioncontract.Text,
				}}})
			},
		},
		{
			name: "compaction", kind: transcript.Compaction,
			new: func() (transcript.Item, error) {
				return transcript.NewCompaction(identity, "summary", 12)
			},
		},
	}
	for _, constructor := range constructors {
		t.Run(constructor.name, func(t *testing.T) {
			item, err := constructor.new()
			if err != nil {
				t.Fatalf("construct Item: %v", err)
			}
			if item.Kind() != constructor.kind || item.Status() != transcript.ItemCompleted {
				t.Fatalf("Item kind/status = %v/%v, want %v/completed", item.Kind(), item.Status(), constructor.kind)
			}
			if item.OccurredAt().Location() != time.UTC {
				t.Fatalf("occurrence location = %v, want UTC", item.OccurredAt().Location())
			}
		})
	}
}

func TestAgentMessageRequiresAnAuthoredPhase(t *testing.T) {
	if _, err := transcript.NewAgentMessage(
		itemIdentity(),
		transcript.MessagePhaseNone,
		[]transcript.ContentBlock{{Kind: transcript.TextContent, Text: "hello"}},
	); err == nil {
		t.Fatal("NewAgentMessage accepted an absent phase")
	}
}

func TestItemOwnsMutablePayloads(t *testing.T) {
	image := []byte{1, 2, 3}
	message, err := transcript.NewUserMessage(itemIdentity(), []transcript.ContentBlock{{
		Kind: transcript.ImageContent, MediaType: "image/png", Bytes: image,
	}})
	if err != nil {
		t.Fatalf("NewUserMessage: %v", err)
	}
	image[0] = 9
	content := message.Body().(transcript.UserMessageBody).Content
	content[0].Bytes[1] = 8
	if got := message.Body().(transcript.UserMessageBody).Content[0].Bytes; got[0] != 1 || got[1] != 2 {
		t.Fatalf("message shares mutable image bytes: %v", got)
	}

	questionInput := transcript.Question{Fields: []transcript.QuestionField{{
		Prompt: "Choose", Kind: questioncontract.Choice,
		Options: []transcript.QuestionOption{{Label: "A"}, {Label: "B"}},
	}}}
	questionItem, err := transcript.NewQuestion(itemIdentity(), questionInput)
	if err != nil {
		t.Fatalf("NewQuestion: %v", err)
	}
	questionInput.Fields[0].Options[0].Label = "changed"
	question, _ := questionItem.Question()
	question.Fields[0].Options[1].Label = "changed"
	owned, _ := questionItem.Question()
	if owned.Fields[0].Options[0].Label != "A" || owned.Fields[0].Options[1].Label != "B" {
		t.Fatalf("question shares mutable option storage: %+v", owned)
	}
}

func TestAnswerQuestionEnrichesAnImmutablePromptExactlyOnce(t *testing.T) {
	prompt, err := transcript.NewQuestion(itemIdentity(), transcript.Question{
		Fields: []transcript.QuestionField{{
			Prompt: "Choose", Kind: questioncontract.Choice,
			Options: []transcript.QuestionOption{{Label: "A"}, {Label: "B"}},
		}},
	})
	if err != nil {
		t.Fatalf("NewQuestion: %v", err)
	}
	answers := [][]string{{"B"}}
	if _, err := prompt.AnswerQuestion(nil); err == nil {
		t.Fatal("AnswerQuestion accepted an absent answer")
	}
	answer, err := prompt.AnswerQuestion(answers)
	if err != nil {
		t.Fatalf("AnswerQuestion: %v", err)
	}
	answered := answer.State()
	answers[0][0] = "A"
	original, _ := prompt.Question()
	accepted, _ := answered.Question()
	if original.Answered() || !accepted.Answered() || accepted.Answers[0][0] != "B" {
		t.Fatalf("prompt/answered = %+v / %+v", original, accepted)
	}
	accepted.Answers[0][0] = "A"
	again, _ := answered.Question()
	if again.Answers[0][0] != "B" {
		t.Fatalf("answered Question shares returned answer storage: %+v", again)
	}
	if _, err := answered.AnswerQuestion([][]string{{"A"}}); err == nil {
		t.Fatal("AnswerQuestion accepted a second answer")
	}
}

func TestItemForkReidentifiesTerminalHistoryAndKeepsOffload(t *testing.T) {
	running, err := transcript.NewToolCall(
		itemIdentity(), transcript.ToolInvocation{Name: "read_large"}, tool.SafetyClassSafe,
	)
	if err != nil {
		t.Fatalf("NewToolCall: %v", err)
	}
	childSession, err := flameidentity.ParseSession("session-child")
	if err != nil {
		t.Fatal(err)
	}
	childRun, err := flameidentity.ParseRun("run-child")
	if err != nil {
		t.Fatal(err)
	}
	if _, forkErr := running.Fork(childSession, childRun, "item-child"); forkErr == nil {
		t.Fatal("Fork accepted a running Item")
	}
	preview, err := tool.NewResult("preview")
	if err != nil {
		t.Fatal(err)
	}
	settlement, err := running.CompleteToolCall(transcript.ToolInvocation{
		Name: "read_large", Result: &preview, Offload: &transcript.ToolResultRef{ID: "SOURCE23"},
	}, running.OccurredAt(), running.OccurredAt().Add(time.Second))
	if err != nil {
		t.Fatalf("CompleteToolCall: %v", err)
	}
	completed := settlement.State()
	forked, err := completed.Fork(childSession, childRun, "item-child")
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	invocation := toolCall(t, forked).Invocation
	if forked.SessionID() != "session-child" || forked.RunID() != "run-child" ||
		forked.ID() != "item-child" || invocation.Offload == nil ||
		invocation.Offload.ID != "SOURCE23" {
		t.Fatalf("forked Item = %+v", forked.Snapshot())
	}
	sourceInvocation := toolCall(t, completed).Invocation
	if completed.SessionID() != "session-1" || completed.ID() != "item-1" ||
		sourceInvocation.Offload == nil || sourceInvocation.Offload.ID != "SOURCE23" {
		t.Fatalf("fork mutated source Item: %+v", completed.Snapshot())
	}
}

func TestToolCallSettlementIsTerminalAndPreservesIdentity(t *testing.T) {
	arguments, err := tool.ParseArguments(`{"path":"README.md"}`)
	if err != nil {
		t.Fatalf("ParseArguments: %v", err)
	}
	running, err := transcript.NewToolCall(
		itemIdentity(),
		transcript.ToolInvocation{Name: "read_file", Input: transcript.AdmittedInput(arguments)},
		tool.SafetyClassSafe,
	)
	if err != nil {
		t.Fatalf("NewToolCall: %v", err)
	}
	if running.Status() != transcript.ItemRunning || !toolCall(t, running).FinishedAt.IsZero() {
		t.Fatalf("running ToolCall = %+v", running.Snapshot())
	}

	finishedAt := running.OccurredAt().Add(time.Second)
	executionStartedAt := running.OccurredAt().Add(250 * time.Millisecond)
	result, err := tool.NewResult("contents")
	if err != nil {
		t.Fatal(err)
	}
	settlement := transcript.ToolInvocation{Name: "read_file", Input: transcript.AdmittedInput(arguments), Result: &result}
	completion, err := running.CompleteToolCall(settlement, executionStartedAt, finishedAt)
	if err != nil {
		t.Fatalf("CompleteToolCall: %v", err)
	}
	completed := completion.State()
	call := toolCall(t, completed)
	if completed.Status() != transcript.ItemCompleted || !call.FinishedAt.Equal(finishedAt) {
		t.Fatalf("completed ToolCall = %+v", completed.Snapshot())
	}
	if call.ExecutionDuration == nil || *call.ExecutionDuration != 750*time.Millisecond {
		t.Fatalf("completed execution duration = %v", call.ExecutionDuration)
	}
	if _, err := completed.FailToolCall(
		settlement, tool.Failure{Kind: tool.FailureExecution}, executionStartedAt, finishedAt,
	); err == nil {
		t.Fatal("terminal ToolCall accepted a second settlement")
	}

	if _, err := running.CompleteToolCall(
		transcript.ToolInvocation{Name: "other_tool", Input: transcript.AdmittedInput(arguments), Result: &result},
		executionStartedAt,
		finishedAt,
	); err == nil {
		t.Fatal("ToolCall settlement changed invocation identity")
	}
}

func TestToolCallFailureAndAbandonmentHaveOneWayTransitions(t *testing.T) {
	running, err := transcript.NewToolCall(
		itemIdentity(), transcript.ToolInvocation{Name: "shell"}, tool.SafetyClassExec,
	)
	if err != nil {
		t.Fatalf("NewToolCall: %v", err)
	}
	finishedAt := running.OccurredAt().Add(time.Second)
	executionStartedAt := running.OccurredAt().Add(500 * time.Millisecond)
	failure := tool.Failure{Kind: tool.FailureDenied, Detail: "not approved"}
	failed, err := running.FailToolCall(
		transcript.ToolInvocation{Name: "shell"}, failure, executionStartedAt, finishedAt,
	)
	if err != nil {
		t.Fatalf("FailToolCall: %v", err)
	}
	gotFailure := toolCall(t, failed.State()).Failure
	if failed.State().Status() != transcript.ItemIncomplete || gotFailure == nil || *gotFailure != failure {
		t.Fatalf("failed ToolCall = %+v", failed.State().Snapshot())
	}

	abandonment, err := running.AbandonToolCall(nil, finishedAt)
	if err != nil {
		t.Fatalf("AbandonToolCall: %v", err)
	}
	abandoned := toolCall(t, abandonment.State())
	if abandoned.ExecutionDuration != nil {
		t.Fatalf("unstarted abandonment execution duration = %v", *abandoned.ExecutionDuration)
	}
	if abandoned.Failure != nil {
		t.Fatal("unstarted abandonment invented a failure")
	}
}

// TestCloneAnswersPublishesASkippedFieldAsNull pins the shape a skipped answer
// reaches the wire as. slices.Clone would keep an empty row non-nil and encode
// it as [] instead, which is why the copy is an append onto nil.
func TestCloneAnswersPublishesASkippedFieldAsNull(t *testing.T) {
	t.Parallel()

	cloned := transcript.CloneAnswers([][]string{{}, {"yes"}})
	if len(cloned) != 2 {
		t.Fatalf("CloneAnswers returned %d rows, want 2", len(cloned))
	}
	if cloned[0] != nil {
		t.Fatalf("a skipped field cloned to %#v, want nil so it encodes as null", cloned[0])
	}
	if len(cloned[1]) != 1 || cloned[1][0] != "yes" {
		t.Fatalf("an answered field cloned to %#v", cloned[1])
	}
	if transcript.CloneAnswers(nil) != nil {
		t.Fatal("transcript.CloneAnswers(nil) must stay nil")
	}
}

func TestCloneAnswersOwnsItsRows(t *testing.T) {
	t.Parallel()

	source := [][]string{{"first"}}
	cloned := transcript.CloneAnswers(source)
	source[0][0] = "mutated"
	if cloned[0][0] != "first" {
		t.Fatalf("clone shares a row with its producer: %q", cloned[0][0])
	}
}
