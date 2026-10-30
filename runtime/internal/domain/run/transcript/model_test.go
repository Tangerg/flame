package transcript_test

import (
	"math"
	"testing"
	"time"

	questioncontract "github.com/Tangerg/flame/packages/go-sdk/question"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func TestSequenceOrderIsAClosedVocabulary(t *testing.T) {
	for _, order := range []transcript.SequenceOrder{transcript.OldestFirst, transcript.NewestFirst} {
		if err := order.Validate(); err != nil {
			t.Fatalf("Validate(%q): %v", order, err)
		}
		if order.String() != string(order) {
			t.Fatalf("String(%q) = %q", order, order.String())
		}
	}
	for _, order := range []transcript.SequenceOrder{"", "ascending", "descending"} {
		if err := order.Validate(); err == nil {
			t.Fatalf("Validate(%q) succeeded", order)
		}
	}
}

func TestCloneContentOwnsImageBytes(t *testing.T) {
	original := []transcript.ContentBlock{{
		Kind: transcript.ImageContent, MediaType: "image/png", Bytes: []byte{1, 2, 3},
	}}
	cloned := transcript.CloneContent(original)

	cloned[0].Bytes[0] = 9
	if original[0].Bytes[0] != 1 {
		t.Fatalf("CloneContent shares image storage: original bytes = %v", original[0].Bytes)
	}
	original[0].Bytes[1] = 8
	if cloned[0].Bytes[1] != 2 {
		t.Fatalf("CloneContent is not ownership-isolated: cloned bytes = %v", cloned[0].Bytes)
	}
}

func TestTextContentRequiresNonWhitespaceText(t *testing.T) {
	for _, text := range []string{"", " \n\t"} {
		if err := (transcript.ContentBlock{Kind: transcript.TextContent, Text: text}).Validate(); err == nil {
			t.Fatalf("Validate accepted text %q", text)
		}
	}
	if err := (transcript.ContentBlock{Kind: transcript.TextContent, Text: " continue "}).Validate(); err != nil {
		t.Fatalf("Validate rejected meaningful text: %v", err)
	}
}

func TestItemValidateOwnsPayloadInvariants(t *testing.T) {
	identity := transcript.ItemIdentity{
		SessionID: "session-1", RunID: "run-1", ItemID: "item-1",
		OccurredAt: time.Unix(1, 0).UTC(),
	}
	negativeDuration := -time.Nanosecond
	tooLongDuration := 2 * time.Second
	hello := []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "hello"}}
	finishedAt := identity.OccurredAt.Add(time.Second)
	tests := []struct {
		name     string
		snapshot transcript.ItemSnapshot
		wantErr  bool
	}{
		{
			name:     "missing body",
			snapshot: transcript.ItemSnapshot{Identity: identity, Status: transcript.ItemCompleted},
			wantErr:  true,
		},
		{
			name: "missing occurrence time",
			snapshot: transcript.ItemSnapshot{
				Identity: transcript.ItemIdentity{SessionID: "session-1", RunID: "run-1", ItemID: "item-1"},
				Status:   transcript.ItemCompleted, Body: transcript.UserMessageBody{Content: hello},
			},
			wantErr: true,
		},
		{
			name: "valid message",
			snapshot: transcript.ItemSnapshot{
				Identity: identity, Status: transcript.ItemCompleted, Body: transcript.UserMessageBody{Content: hello},
			},
		},
		{
			name: "negative tool execution duration",
			snapshot: transcript.ItemSnapshot{
				Identity: identity, Status: transcript.ItemIncomplete,
				Body: transcript.ToolCallBody{
					Invocation: transcript.ToolInvocation{Name: "shell"}, FinishedAt: finishedAt, ExecutionDuration: &negativeDuration,
				},
			},
			wantErr: true,
		},
		{
			name: "completed tool missing execution duration",
			snapshot: transcript.ItemSnapshot{
				Identity: identity, Status: transcript.ItemCompleted,
				Body: transcript.ToolCallBody{Invocation: transcript.ToolInvocation{Name: "shell"}, FinishedAt: finishedAt},
			},
			wantErr: true,
		},
		{
			name: "tool execution duration exceeds lifecycle",
			snapshot: transcript.ItemSnapshot{
				Identity: identity, Status: transcript.ItemCompleted,
				Body: transcript.ToolCallBody{
					Invocation: transcript.ToolInvocation{Name: "shell"}, FinishedAt: finishedAt, ExecutionDuration: &tooLongDuration,
				},
			},
			wantErr: true,
		},
		{
			name: "running tool with terminal facts",
			snapshot: transcript.ItemSnapshot{
				Identity: identity, Status: transcript.ItemRunning,
				Body: transcript.ToolCallBody{Invocation: transcript.ToolInvocation{Name: "shell"}, FinishedAt: finishedAt},
			},
			wantErr: true,
		},
		{
			name: "message without content",
			snapshot: transcript.ItemSnapshot{
				Identity: identity, Status: transcript.ItemCompleted,
				Body: transcript.AgentMessageBody{Phase: transcript.MessageFinalAnswer},
			},
			wantErr: true,
		},
		{
			name: "reasoning without text",
			snapshot: transcript.ItemSnapshot{
				Identity: identity, Status: transcript.ItemCompleted, Body: transcript.ReasoningBody{},
			},
			wantErr: true,
		},
		{
			name: "question without a prompt",
			snapshot: transcript.ItemSnapshot{
				Identity: identity, Status: transcript.ItemCompleted, Body: transcript.QuestionBody{},
			},
			wantErr: true,
		},
	}
	// Visible model observations may settle incomplete; running anchors are never stored.
	settled := []transcript.Body{
		transcript.UserMessageBody{Content: hello},
		transcript.AgentMessageBody{Phase: transcript.MessageFinalAnswer, Content: hello},
		transcript.ReasoningBody{Text: "thinking"},
		transcript.QuestionBody{Question: transcript.Question{
			Fields: []transcript.QuestionField{
				{Prompt: "pick", Header: "Pick", Kind: questioncontract.Text},
			},
		}},
		transcript.CompactionBody{Summary: "folded"},
	}
	for _, body := range settled {
		kind := body.Kind()
		for _, status := range []transcript.ItemStatus{transcript.ItemCompleted, transcript.ItemRunning, transcript.ItemIncomplete} {
			tests = append(tests, struct {
				name     string
				snapshot transcript.ItemSnapshot
				wantErr  bool
			}{
				name:     string(kind) + " " + string(status),
				snapshot: transcript.ItemSnapshot{Identity: identity, Status: status, Body: body},
				wantErr:  status != transcript.ItemCompleted && !(status == transcript.ItemIncomplete && (kind == transcript.AgentMessage || kind == transcript.Reasoning)),
			})
		}
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := transcript.RestoreItem(test.snapshot)
			if (err != nil) != test.wantErr {
				t.Fatalf("RestoreItem() error = %v, want error %t", err, test.wantErr)
			}
		})
	}
}

func TestToolCallTimingLifecycle(t *testing.T) {
	startedAt := time.Date(2026, 8, 4, 10, 0, 0, 0, time.UTC)
	executionStartedAt := startedAt.Add(500 * time.Millisecond)
	finishedAt := startedAt.Add(1500 * time.Millisecond)
	identity := transcript.ItemIdentity{
		SessionID: "session-1", RunID: "run-1", ItemID: "item-1", OccurredAt: startedAt,
	}
	invocation := transcript.ToolInvocation{Name: "shell"}
	running, err := transcript.NewToolCall(identity, invocation, tool.SafetyClass(""))
	if err != nil {
		t.Fatalf("NewToolCall(): %v", err)
	}
	if call, _ := running.ToolCall(); running.Status() != transcript.ItemRunning || !call.FinishedAt.IsZero() {
		t.Fatalf("running timing = (%v, %v)", running.Status(), call.FinishedAt)
	}
	completion, err := running.CompleteToolCall(invocation, executionStartedAt, finishedAt)
	if err != nil {
		t.Fatalf("CompleteToolCall(): %v", err)
	}
	completed := completion.State()
	if call, _ := completed.ToolCall(); completed.Status() != transcript.ItemCompleted || !call.FinishedAt.Equal(finishedAt) {
		t.Fatalf("completed timing = (%v, %v)", completed.Status(), call.FinishedAt)
	}
	if _, completeToolCallErr := completed.CompleteToolCall(invocation, executionStartedAt, finishedAt.Add(time.Second)); completeToolCallErr == nil {
		t.Fatal("second CompleteToolCall() succeeded")
	}
	if _, completeToolCallErr := running.CompleteToolCall(invocation, startedAt.Add(-time.Millisecond), finishedAt); completeToolCallErr == nil {
		t.Fatal("CompleteToolCall() accepted execution before occurrence")
	}
	abandonment, err := running.AbandonToolCall(nil, finishedAt)
	if err != nil {
		t.Fatalf("AbandonToolCall(): %v", err)
	}
	if abandonment.State().Status() != transcript.ItemIncomplete {
		t.Fatalf("abandoned status = %v", abandonment.State().Status())
	}
	if call, _ := abandonment.State().ToolCall(); call.ExecutionDuration != nil {
		t.Fatal("unstarted abandonment fabricated an execution duration")
	}
	startedAbandonment, err := running.AbandonStartedToolCall(invocation, nil, executionStartedAt, finishedAt)
	if err != nil {
		t.Fatalf("AbandonStartedToolCall(): %v", err)
	}
	if call, _ := startedAbandonment.State().ToolCall(); call.ExecutionDuration == nil || *call.ExecutionDuration != time.Second {
		t.Fatalf("started abandonment execution duration = %v, want 1s", call.ExecutionDuration)
	}
}

func TestUsageAndToolFailureValidate(t *testing.T) {
	negativeCost := -0.1
	nanCost := math.NaN()
	infiniteCost := math.Inf(1)
	tests := []struct {
		name    string
		usage   *accounting.Usage
		failure *tool.Failure
		wantErr bool
	}{
		{name: "valid"},
		{name: "negative token", usage: &accounting.Usage{Total: accounting.Totals{InputTokens: -1}}, wantErr: true},
		{name: "negative cost", usage: &accounting.Usage{Total: accounting.Totals{CostUSD: &negativeCost}}, wantErr: true},
		{name: "nan cost", usage: &accounting.Usage{Total: accounting.Totals{CostUSD: &nanCost}}, wantErr: true},
		{name: "infinite cost", usage: &accounting.Usage{Total: accounting.Totals{CostUSD: &infiniteCost}}, wantErr: true},
		{name: "unknown failure kind", failure: &tool.Failure{Kind: tool.FailureKind("invalid")}, wantErr: true},
		{name: "valid execution failure", failure: &tool.Failure{Kind: tool.FailureExecution}},
		{name: "valid canceled failure", failure: &tool.Failure{Kind: tool.FailureCanceled}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var err error
			if test.usage != nil {
				err = test.usage.Validate()
			}
			if err == nil && test.failure != nil {
				err = test.failure.Validate()
			}
			if (err != nil) != test.wantErr {
				t.Fatalf("validation error = %v, want error %t", err, test.wantErr)
			}
		})
	}
}

func TestQuestionValidateRejectsUnanswerableShapes(t *testing.T) {
	choice := func(options ...string) []transcript.QuestionOption {
		result := make([]transcript.QuestionOption, len(options))
		for index, option := range options {
			result[index] = transcript.QuestionOption{Label: option}
		}
		return result
	}
	tests := []struct {
		name    string
		value   transcript.Question
		wantErr bool
	}{
		{name: "text", value: transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Explain", Kind: questioncontract.Text}}}},
		{name: "custom choice", value: transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Choose", Kind: questioncontract.Choice, Options: choice("A", "B"), AllowCustom: true}}}},
		{name: "no fields", value: transcript.Question{}, wantErr: true},
		{name: "too many fields", value: transcript.Question{Fields: []transcript.QuestionField{
			{Prompt: "One", Kind: questioncontract.Text},
			{Prompt: "Two", Kind: questioncontract.Text},
			{Prompt: "Three", Kind: questioncontract.Text},
			{Prompt: "Four", Kind: questioncontract.Text},
			{Prompt: "Five", Kind: questioncontract.Text},
		}}, wantErr: true},
		{name: "blank prompt", value: transcript.Question{Fields: []transcript.QuestionField{{Kind: questioncontract.Text}}}, wantErr: true},
		{name: "long header", value: transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Explain", Header: "1234567890123", Kind: questioncontract.Text}}}, wantErr: true},
		{name: "text carries choice settings", value: transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Explain", Kind: questioncontract.Text, AllowCustom: true}}}, wantErr: true},
		{name: "choice has one option", value: transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Choose", Kind: questioncontract.Choice, Options: choice("A")}}}, wantErr: true},
		{name: "choice has too many options", value: transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Choose", Kind: questioncontract.Choice, Options: choice("A", "B", "C", "D", "E")}}}, wantErr: true},
		{name: "duplicate choice", value: transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Choose", Kind: questioncontract.Choice, Options: choice("A", "A")}}}, wantErr: true},
		{name: "padded choice", value: transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Choose", Kind: questioncontract.Choice, Options: choice(" A", "B")}}}, wantErr: true},
		{name: "unknown kind", value: transcript.Question{Fields: []transcript.QuestionField{{Prompt: "Choose", Kind: questioncontract.Kind("invalid")}}}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.value.Validate(); (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, want error %t", err, test.wantErr)
			}
		})
	}
}

func TestQuestionEqualityOwnsSemanticRepresentation(t *testing.T) {
	left := transcript.Question{Fields: []transcript.QuestionField{{
		Prompt: "Explain", Kind: questioncontract.Text,
	}}}
	right := transcript.Question{Fields: []transcript.QuestionField{{
		Prompt: "Explain", Kind: questioncontract.Text, Options: []transcript.QuestionOption{},
	}}}
	if !left.Equal(right) {
		t.Fatal("equivalent unanswered Questions differ by empty option representation")
	}
	answeredLeft := left
	answeredLeft.Answers = [][]string{{}}
	answeredRight := right
	answeredRight.Answers = [][]string{nil}
	if !answeredLeft.Equal(answeredRight) {
		t.Fatal("equivalent answered Questions differ by empty answer representation")
	}
	if left.Equal(answeredLeft) {
		t.Fatal("unanswered Question equals answered Question")
	}
	changed := left
	changed.Fields = []transcript.QuestionField{{Prompt: "Different", Kind: questioncontract.Text}}
	if left.Equal(changed) {
		t.Fatal("Questions with different prompts are equal")
	}
}

func TestToolInvocationEqualityUsesCanonicalValues(t *testing.T) {
	leftArguments, err := tool.ParseArguments(`{"b":2,"a":1}`)
	if err != nil {
		t.Fatal(err)
	}
	rightArguments, err := tool.ParseArguments(`{"a":1,"b":2}`)
	if err != nil {
		t.Fatal(err)
	}
	leftResult := tool.Result{}
	rightResult, err := tool.ParseResult([]byte("null"))
	if err != nil {
		t.Fatal(err)
	}
	left := transcript.ToolInvocation{
		Name: "shell", Input: transcript.AdmittedInput(leftArguments), Result: &leftResult,
		Offload: &transcript.ToolResultRef{ID: "BLOB234"},
	}
	right := transcript.ToolInvocation{
		Name: "shell", Input: transcript.AdmittedInput(rightArguments), Result: &rightResult,
		Offload: &transcript.ToolResultRef{ID: "BLOB234"},
	}
	if !left.Equal(right) {
		t.Fatal("equivalent Tool invocations differ by canonical representation")
	}
	right.Offload = &transcript.ToolResultRef{ID: "BLOB235"}
	if left.Equal(right) {
		t.Fatal("Tool invocations with different offloads are equal")
	}
}

func TestApprovalValidateRequiresAPendingRiskClassifiedTool(t *testing.T) {
	result, err := tool.NewResult("already ran")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		approval transcript.Approval
		wantErr  bool
	}{
		{name: "pending", approval: transcript.Approval{Tool: transcript.ToolInvocation{Name: "shell"}, Risk: tool.RiskHigh}},
		{name: "missing tool", approval: transcript.Approval{Risk: tool.RiskHigh}, wantErr: true},
		{name: "padded tool", approval: transcript.Approval{Tool: transcript.ToolInvocation{Name: " shell"}, Risk: tool.RiskHigh}, wantErr: true},
		{name: "missing risk", approval: transcript.Approval{Tool: transcript.ToolInvocation{Name: "shell"}}, wantErr: true},
		{name: "completed result", approval: transcript.Approval{Tool: transcript.ToolInvocation{Name: "shell", Result: &result}, Risk: tool.RiskHigh}, wantErr: true},
		{name: "offloaded result", approval: transcript.Approval{Tool: transcript.ToolInvocation{Name: "shell", Offload: &transcript.ToolResultRef{ID: "BLOB234"}}, Risk: tool.RiskHigh}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.approval.Validate(); (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, want error %t", err, test.wantErr)
			}
		})
	}
}
