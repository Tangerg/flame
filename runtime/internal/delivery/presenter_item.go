package delivery

import (
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func presentItem(item transcript.Item) protocol.Item {
	var failureRef *tool.Failure
	if failure, present := item.Failure(); present {
		failureRef = &failure
	}
	out := protocol.Item{
		ID: item.ID(), RunID: item.RunID(), Status: presentItemStatus(item.Status()),
		Type:  presentItemKind(item.Kind()),
		Phase: presentMessagePhase(item.MessagePhase()), Text: item.Text(), Redacted: item.Redacted(),
		SafetyClass: presentSafetyClass(item.SafetyClass()), Error: presentToolFailure(failureRef),
		ApprovalDecision: presentItemApprovalDecision(item.ApprovalDecision()),
		Summary:          item.Summary(), DroppedMessages: item.DroppedMessages(),
	}
	content := item.Content()
	if len(content) > 0 {
		out.Content = make([]protocol.ContentBlock, len(content))
		for i, block := range content {
			out.Content[i] = presentContent(block)
		}
	}
	if value, present := item.Question(); present {
		question := presentQuestion(value)
		out.Question = &question
	}
	if value, present := item.ToolInvocation(); present {
		tool := presentTool(value)
		out.Tool = &tool
	}
	if item.Kind() == transcript.ToolCall {
		out.StartedAt = item.OccurredAt()
		out.FinishedAt = item.FinishedAt()
		out.DurationMillis = presentToolDurationMillis(item)
	} else {
		out.CreatedAt = item.OccurredAt()
	}
	return out
}

func presentMessagePhase(phase transcript.MessagePhase) protocol.MessagePhase {
	switch phase {
	case transcript.MessagePhaseNone:
		return ""
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

func presentToolDurationMillis(item transcript.Item) *int64 {
	duration, known := item.ExecutionDuration()
	if !known {
		return nil
	}
	millis := duration.Milliseconds()
	return &millis
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
		case transcript.QuestionText:
			kind = protocol.QuestionFieldText
		case transcript.QuestionChoice:
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
	return protocol.ToolInvocation{
		Name: invocation.Name, Category: presentToolCategory(invocation.Name),
		ArgumentsText: invocation.ArgumentsText, Arguments: invocation.Arguments.Map(), Result: result,
	}
}

// presentToolCategory owns the one tool-to-category mapping every client renders
// from, replacing the copies the CLI and Web each kept. Built-in identities are
// grouped; plugin and MCP tools stay unknown.
func presentToolCategory(name string) protocol.ToolCategory {
	switch tool.BuiltInName(name) {
	case tool.Shell, tool.ReadShellOutput, tool.StopShell:
		return protocol.ToolCategoryShell
	case tool.ApplyPatch, tool.Edit:
		return protocol.ToolCategoryEdit
	case tool.Read, tool.ReadSkillResource, tool.ReadToolResult:
		return protocol.ToolCategoryRead
	case tool.Glob, tool.Grep, tool.SearchTools:
		return protocol.ToolCategorySearch
	case tool.WebSearch, tool.WebFetch:
		return protocol.ToolCategoryWeb
	case tool.AskUser, tool.DelegateTask, tool.CreateGoal, tool.GetGoal, tool.ReportGoalOutcome,
		tool.LoadSkill, tool.ListSkills, tool.ProposeSkill, tool.EnterPlanMode, tool.ExitPlanMode, tool.SetPlan:
		return protocol.ToolCategoryTask
	default:
		return protocol.ToolCategoryUnknown
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
