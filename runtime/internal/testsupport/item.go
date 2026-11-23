package testsupport

import (
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	questioncontract "github.com/Tangerg/flame/packages/go-sdk/question"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

const (
	fixtureSessionID = "session_fixture"
	fixtureRunID     = "run_fixture"
	fixtureText      = "fixture"
)

type ItemInput struct {
	SessionID         identity.SessionID
	RunID             identity.RunID
	ID                identity.ItemID
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

func (input *ItemInput) defaultIdentity() {
	if input.SessionID.IsZero() {
		input.SessionID = MustSessionID(fixtureSessionID)
	}
	if input.RunID.IsZero() {
		input.RunID = MustRunID(fixtureRunID)
	}
	if input.ID.IsZero() {
		input.ID = MustItemID("item_fixture")
	}
	if input.OccurredAt.IsZero() {
		input.OccurredAt = time.Unix(1, 0).UTC()
	}
	if input.Kind == "" {
		input.Kind = transcript.UserMessage
	}
}

func (input *ItemInput) defaultBody() {
	switch input.Kind {
	case transcript.UserMessage, transcript.AgentMessage:
		input.Status = transcript.ItemCompleted
		if len(input.Content) == 0 {
			input.Content = []transcript.ContentBlock{{Kind: transcript.TextContent, Text: fixtureText}}
		}
		if input.Kind == transcript.AgentMessage && input.MessagePhase == transcript.MessagePhaseNone {
			input.MessagePhase = transcript.MessageFinalAnswer
		}
	case transcript.Reasoning:
		input.Status = transcript.ItemCompleted
		if input.Text == "" {
			input.Text = fixtureText
		}
	case transcript.QuestionItem:
		input.Status = transcript.ItemCompleted
		if input.Question == nil {
			input.Question = &transcript.Question{Fields: []transcript.QuestionField{{
				Prompt: fixtureText, Kind: questioncontract.Text,
			}}}
		}
	case transcript.Compaction:
		input.Status = transcript.ItemCompleted
	case transcript.ToolCall:
		input.defaultToolCall()
	}
}

func (input *ItemInput) defaultToolCall() {
	if input.Tool == nil {
		input.Tool = &transcript.ToolInvocation{Name: fixtureText}
	}
	if input.Status == transcript.ItemRunning {
		return
	}
	if input.FinishedAt.IsZero() {
		input.FinishedAt = input.OccurredAt
	}
	if input.ExecutionDuration == nil {
		duration := input.FinishedAt.Sub(input.OccurredAt)
		input.ExecutionDuration = &duration
	}
}

func MustRestoreItem(input ItemInput) transcript.Item {
	input.defaultIdentity()
	input.defaultBody()
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

func (input *ItemInput) payload() transcript.Body {
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

func ToolInvocationOf(item transcript.Item) (transcript.ToolInvocation, bool) {
	call, ok := item.ToolCall()
	return call.Invocation, ok
}
