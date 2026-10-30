package testsupport

import (
	"time"

	questioncontract "github.com/Tangerg/flame/packages/go-sdk/question"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

// ItemInput mirrors the facts old fixtures commonly spell out while keeping the
// Domain Item itself private. It is intentionally test-only and is not a second
// production representation.
type ItemInput struct {
	SessionID         string
	RunID             string
	ID                string
	Status            transcript.ItemStatus
	FinishedAt        time.Time
	ExecutionDuration *time.Duration
	Kind              transcript.ItemKind
	MessagePhase      transcript.MessagePhase
	OccurredAt        time.Time
	Content           []transcript.ContentBlock
	Text              string
	Redacted          bool
	Question          *transcript.Question
	Tool              *transcript.ToolInvocation
	SafetyClass       tool.SafetyClass
	Approval          *approval.Resolution
	Failure           *tool.Failure
	Summary           string
	DroppedMessages   int
}

// MustRestoreItem returns one valid Item or panics. Tests exercising invalid
// snapshots must call transcript.RestoreItem directly and assert the error.
func MustRestoreItem(input ItemInput) transcript.Item {
	if input.SessionID == "" {
		input.SessionID = "session_fixture"
	}
	if input.RunID == "" {
		input.RunID = "run_fixture"
	}
	if input.ID == "" {
		input.ID = "item_fixture"
	}
	if input.OccurredAt.IsZero() {
		input.OccurredAt = time.Unix(1, 0).UTC()
	}
	if input.Kind == "" {
		input.Kind = transcript.UserMessage
	}
	switch input.Kind {
	case transcript.UserMessage, transcript.AgentMessage:
		input.Status = transcript.ItemCompleted
		if len(input.Content) == 0 {
			input.Content = []transcript.ContentBlock{{Kind: transcript.TextContent, Text: "fixture"}}
		}
		if input.Kind == transcript.AgentMessage && input.MessagePhase == transcript.MessagePhaseNone {
			input.MessagePhase = transcript.MessageFinalAnswer
		}
	case transcript.Reasoning:
		input.Status = transcript.ItemCompleted
		if input.Text == "" {
			input.Text = "fixture"
		}
	case transcript.QuestionItem:
		input.Status = transcript.ItemCompleted
		if input.Question == nil {
			input.Question = &transcript.Question{Fields: []transcript.QuestionField{{
				Prompt: "fixture", Kind: questioncontract.Text,
			}}}
		}
	case transcript.Compaction:
		input.Status = transcript.ItemCompleted
	case transcript.ToolCall:
		if input.Tool == nil {
			input.Tool = &transcript.ToolInvocation{Name: "fixture"}
		}
		if input.Status != transcript.ItemRunning && input.FinishedAt.IsZero() {
			input.FinishedAt = input.OccurredAt
		}
		if input.Status != transcript.ItemRunning && input.ExecutionDuration == nil {
			duration := input.FinishedAt.Sub(input.OccurredAt)
			input.ExecutionDuration = &duration
		}
	}
	item, err := transcript.RestoreItem(transcript.ItemSnapshot{
		Identity: transcript.ItemIdentity{
			SessionID: input.SessionID, RunID: input.RunID, ItemID: input.ID,
			OccurredAt: input.OccurredAt,
		},
		Status: input.Status, Body: input.payload(),
	})
	if err != nil {
		panic(err)
	}
	return item
}

func (input ItemInput) payload() transcript.Body {
	switch input.Kind {
	case transcript.UserMessage:
		return transcript.UserMessageBody{Content: input.Content}
	case transcript.AgentMessage:
		return transcript.AgentMessageBody{Phase: input.MessagePhase, Content: input.Content}
	case transcript.Reasoning:
		return transcript.ReasoningBody{Text: input.Text, Redacted: input.Redacted}
	case transcript.QuestionItem:
		return transcript.QuestionBody{Question: *input.Question}
	case transcript.Compaction:
		return transcript.CompactionBody{Summary: input.Summary, DroppedMessages: input.DroppedMessages}
	case transcript.ToolCall:
		return transcript.ToolCallBody{
			Invocation: *input.Tool, SafetyClass: input.SafetyClass, Approval: input.Approval,
			FinishedAt: input.FinishedAt, ExecutionDuration: input.ExecutionDuration, Failure: input.Failure,
		}
	}
	panic("testsupport: unknown Item kind " + string(input.Kind))
}

// MustItemReplacement applies one named Item transition or panics.
func MustItemReplacement(replacement transcript.Replacement, err error) transcript.Replacement {
	if err != nil {
		panic(err)
	}
	return replacement
}

// Facts flattens an Item into the fixture shape so assertions can read any
// variant's fields without a type switch per check.
func Facts(item transcript.Item) ItemInput {
	facts := ItemInput{
		SessionID: item.SessionID(), RunID: item.RunID(), ID: item.ID(),
		Status: item.Status(), Kind: item.Kind(), OccurredAt: item.OccurredAt(),
	}
	switch payload := item.Body().(type) {
	case transcript.UserMessageBody:
		facts.Content = payload.Content
	case transcript.AgentMessageBody:
		facts.MessagePhase, facts.Content = payload.Phase, payload.Content
	case transcript.ReasoningBody:
		facts.Text, facts.Redacted = payload.Text, payload.Redacted
	case transcript.QuestionBody:
		facts.Question = &payload.Question
	case transcript.CompactionBody:
		facts.Summary, facts.DroppedMessages = payload.Summary, payload.DroppedMessages
	case transcript.ToolCallBody:
		facts.Tool, facts.SafetyClass, facts.Approval = &payload.Invocation, payload.SafetyClass, payload.Approval
		facts.FinishedAt, facts.ExecutionDuration, facts.Failure = payload.FinishedAt, payload.ExecutionDuration, payload.Failure
	}
	return facts
}

// ApprovalDecision returns the accepted verdict, empty while unresolved.
func (input ItemInput) ApprovalDecision() approval.Decision {
	if input.Approval == nil {
		return ""
	}
	return input.Approval.Decision
}

// ToolInvocationOf returns the ToolCall invocation when item is a ToolCall.
func ToolInvocationOf(item transcript.Item) (transcript.ToolInvocation, bool) {
	call, ok := item.ToolCall()
	return call.Invocation, ok
}

// FailureOf returns the classified ToolCall failure when one was recorded.
func FailureOf(item transcript.Item) (tool.Failure, bool) {
	call, _ := item.ToolCall()
	if call.Failure == nil {
		return tool.Failure{}, false
	}
	return *call.Failure, true
}

// ExecutionDurationOf returns the ToolCall execution interval when known.
func ExecutionDurationOf(item transcript.Item) (time.Duration, bool) {
	call, _ := item.ToolCall()
	if call.ExecutionDuration == nil {
		return 0, false
	}
	return *call.ExecutionDuration, true
}
