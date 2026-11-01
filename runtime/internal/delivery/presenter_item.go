package delivery

import (
	builtincontract "github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	questioncontract "github.com/Tangerg/flame/packages/go-sdk/question"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func presentItem(item transcript.Item) protocol.Item {
	out := protocol.Item{
		ID: item.ID(), RunID: item.RunID(), Status: presentItemStatus(item.Status()),
		Type: presentItemKind(item.Kind()),
	}
	payload := item.Body()
	if _, isToolCall := payload.(transcript.ToolCallBody); !isToolCall {
		out.CreatedAt = item.OccurredAt()
	}
	switch payload := payload.(type) {
	case transcript.UserMessageBody:
		out.Content = presentContents(payload.Content)
	case transcript.AgentMessageBody:
		out.Phase = presentMessagePhase(payload.Phase)
		out.Content = presentContents(payload.Content)
	case transcript.ReasoningBody:
		out.Text, out.Redacted = payload.Text, payload.Redacted
	case transcript.QuestionBody:
		question := presentQuestion(payload.Question)
		out.Question = &question
	case transcript.CompactionBody:
		out.Summary, out.DroppedMessages = payload.Summary, payload.DroppedMessages
	case transcript.ToolCallBody:
		tool := presentTool(payload.Invocation)
		out.Tool = &tool
		out.SafetyClass = presentSafetyClass(payload.SafetyClass)
		out.Error = presentToolFailure(payload.Failure)
		if payload.Approval != nil {
			out.ApprovalDecision = presentItemApprovalDecision(payload.Approval.Decision)
		}
		out.StartedAt = item.OccurredAt()
		out.FinishedAt = payload.FinishedAt
		if payload.ExecutionDuration != nil {
			out.DurationMillis = new(payload.ExecutionDuration.Milliseconds())
		}
	}
	return out
}

func presentContents(content []transcript.ContentBlock) []protocol.ContentBlock {
	if len(content) == 0 {
		return nil
	}
	out := make([]protocol.ContentBlock, len(content))
	for i, block := range content {
		out[i] = presentContent(block)
	}
	return out
}

func presentMessagePhase(phase transcript.MessagePhase) protocol.MessagePhase {
	switch phase {
	case transcript.MessageCommentary:
		return protocol.MessagePhaseCommentary
	case transcript.MessageFinalAnswer:
		return protocol.MessagePhaseFinalAnswer
	default:
		panic("delivery: unknown transcript message phase")
	}
}

func presentItemApprovalDecision(decision approval.Decision) protocol.ApprovalDecision {
	switch decision {
	case approval.Allow:
		return protocol.ApprovalApprove
	case approval.Deny:
		return protocol.ApprovalDeny
	default:
		return ""
	}
}

func presentItemStart(item runs.ItemStart) protocol.Item {
	out := protocol.Item{
		ID: item.ItemID, RunID: item.RunID, Status: protocol.ItemStatusRunning,
		Type: presentItemKind(item.Kind), SafetyClass: presentSafetyClass(item.SafetyClass),
	}
	if item.Kind == transcript.ToolCall {
		out.StartedAt = item.OccurredAt
		if item.ToolInvocation != nil {
			tool := presentTool(*item.ToolInvocation)
			out.Tool = &tool
		}
	} else {
		out.CreatedAt = item.OccurredAt
	}
	return out
}

func presentItemStatus(status transcript.ItemStatus) protocol.ItemStatus {
	switch status {
	case transcript.ItemRunning:
		return protocol.ItemStatusRunning
	case transcript.ItemCompleted:
		return protocol.ItemStatusCompleted
	case transcript.ItemIncomplete:
		return protocol.ItemStatusIncomplete
	default:
		panic("delivery: unknown transcript item status")
	}
}

func presentItemKind(kind transcript.ItemKind) protocol.ItemType {
	if !kind.Valid() {
		panic("delivery: unknown transcript item kind")
	}
	return protocol.ItemType(kind)
}

func presentContent(block transcript.ContentBlock) protocol.ContentBlock {
	encoded, err := encodeContent(block)
	if err != nil {
		panic("delivery: " + err.Error())
	}
	return protocol.ContentBlock{Type: encoded.kind, Text: encoded.text, Mime: encoded.mime, Data: encoded.data}
}

func presentQuestion(question transcript.Question) protocol.Question {
	fields := make([]protocol.QuestionField, len(question.Fields))
	for i, field := range question.Fields {
		var kind protocol.QuestionFieldType
		switch field.Kind {
		case questioncontract.Text:
			kind = protocol.QuestionFieldText
		case questioncontract.Choice:
			kind = protocol.QuestionFieldChoice
		default:
			panic("delivery: unknown transcript question-field kind")
		}
		var options []protocol.QuestionOption
		if len(field.Options) > 0 {
			options = make([]protocol.QuestionOption, len(field.Options))
			for j, option := range field.Options {
				options[j] = protocol.QuestionOption{
					Label: option.Label, Description: option.Description, Preview: option.Preview,
				}
			}
		}
		fields[i] = protocol.QuestionField{
			Prompt: field.Prompt, Header: field.Header, Type: kind,
			Options: options, Multiple: field.Multiple, AllowCustom: field.AllowCustom,
		}
	}
	return protocol.Question{Fields: fields, Answers: transcript.CloneAnswers(question.Answers)}
}

func presentTool(invocation transcript.ToolInvocation) protocol.ToolInvocation {
	var result any
	if invocation.Result != nil {
		result = invocation.Result.Any()
	}
	rejected, _ := invocation.Input.Rejected()
	return protocol.ToolInvocation{
		Name: invocation.Name, Source: recordedSource(invocation.Source), Category: builtincontract.Name(invocation.Name).Category(),
		ResultShape:   builtincontract.Name(invocation.Name).ResultShape(),
		ArgumentsText: rejected, Arguments: invocation.Input.Arguments().Map(), Result: result,
	}
}

func presentDelta(delta runs.ItemDelta) protocol.ItemDelta {
	switch delta := delta.(type) {
	case runs.ContentItemDelta:
		return protocol.ItemDelta{Type: protocol.DeltaContent, Text: delta.Text()}
	case runs.ReasoningItemDelta:
		return protocol.ItemDelta{Type: protocol.DeltaReasoning, Text: delta.Text()}
	case runs.ToolArgumentsItemDelta:
		return protocol.ItemDelta{Type: protocol.DeltaToolArguments, ArgumentsTextDelta: delta.Text()}
	case runs.ToolOutputItemDelta:
		return protocol.ItemDelta{Type: protocol.DeltaToolOutput, Text: delta.Text()}
	default:
		panic("delivery: unknown Item delta variant")
	}
}
