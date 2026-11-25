package delivery

import (
	"fmt"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	questioncontract "github.com/Tangerg/flame/packages/go-sdk/question"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func portableItemFromArtifact(sessionID identity.SessionID, path string, artifact protocol.ArtifactItem) (transcript.Item, error) {
	status, err := portableItemStatus(path+".status", artifact.Status)
	if err != nil {
		return transcript.Item{}, err
	}
	kind, err := portableItemKind(path+".type", artifact.Type)
	if err != nil {
		return transcript.Item{}, err
	}
	runID, err := identity.ParseOptionalRun(artifact.RunID)
	if err != nil {
		return transcript.Item{}, invalidArtifact(path+".runId", "%v", err)
	}
	itemID, err := identity.ParseItem(artifact.ID)
	if err != nil {
		return transcript.Item{}, invalidArtifact(path+".id", "%v", err)
	}
	snapshot := transcript.ItemSnapshot{
		Identity: transcript.ItemIdentity{
			SessionID: sessionID, ItemID: itemID, RunID: runID,
			OccurredAt: artifact.CreatedAt,
		},
		Status: status,
	}
	body, err := portableItemBody(kind, status, path, artifact)
	if err != nil {
		return transcript.Item{}, err
	}
	snapshot.Body = body
	if kind == transcript.ToolCall {
		snapshot.Identity.OccurredAt = artifact.StartedAt
	}
	item, err := transcript.RestoreItem(snapshot)
	if err != nil {
		return transcript.Item{}, invalidArtifact(path, "%v", err)
	}
	return item, nil
}

func portableItemBody(kind transcript.ItemKind, status transcript.ItemStatus, path string, artifact protocol.ArtifactItem) (transcript.Body, error) {
	var content []transcript.ContentBlock
	for index, block := range artifact.Content {
		decoded, err := portableContentFromArtifact(fmt.Sprintf("%s.content[%d]", path, index), block)
		if err != nil {
			return nil, err
		}
		content = append(content, decoded)
	}
	switch kind {
	case transcript.UserMessage:
		return transcript.UserMessageBody{Content: content}, nil
	case transcript.AgentMessage:
		return transcript.AgentMessageBody{Phase: portableMessagePhase(artifact.Phase), Content: content}, nil
	case transcript.Reasoning:
		return transcript.ReasoningBody{Text: artifact.Text, Redacted: artifact.Redacted}, nil
	case transcript.Compaction:
		return transcript.CompactionBody{Summary: artifact.Summary, DroppedMessages: artifact.DroppedMessages}, nil
	case transcript.QuestionItem:
		if artifact.Question == nil {
			return nil, invalidArtifact(path+".question", "is required")
		}
		question, err := portableQuestionFromArtifact(path+".question", *artifact.Question)
		if err != nil {
			return nil, err
		}
		return transcript.QuestionBody{Question: question}, nil
	case transcript.ToolCall:
		return portableToolCallFromArtifact(path, status, artifact)
	default:
		return nil, nil
	}
}

func portableToolCallFromArtifact(path string, status transcript.ItemStatus, artifact protocol.ArtifactItem) (transcript.ToolCallBody, error) {
	if artifact.Tool == nil {
		return transcript.ToolCallBody{}, invalidArtifact(path+".tool", "is required")
	}
	invocation, err := portableToolFromArtifact(path+".tool", *artifact.Tool)
	if err != nil {
		return transcript.ToolCallBody{}, err
	}
	failure, err := portableToolFailureFromArtifact(path+".error", artifact.Error)
	if err != nil {
		return transcript.ToolCallBody{}, err
	}
	safetyClass, err := portableSafetyClass(path+".safetyClass", artifact.SafetyClass)
	if err != nil {
		return transcript.ToolCallBody{}, err
	}
	call := transcript.ToolCallBody{Invocation: invocation, SafetyClass: safetyClass, Failure: failure}
	if decision := portableItemApprovalDecision(artifact.ApprovalDecision); decision != "" {
		call.Approval = &approval.Resolution{Decision: decision}
	}
	if status != transcript.ItemRunning {
		if artifact.DurationMillis != nil {
			call.ExecutionDuration = new(time.Duration(*artifact.DurationMillis) * time.Millisecond)
		}
		call.FinishedAt = artifact.FinishedAt
	}
	return call, nil
}

func portableMessagePhase(phase protocol.MessagePhase) transcript.MessagePhase {
	switch phase {
	case protocol.MessagePhaseCommentary:
		return transcript.MessageCommentary
	case protocol.MessagePhaseFinalAnswer:
		return transcript.MessageFinalAnswer
	default:
		return transcript.MessagePhaseNone
	}
}

func portableItemApprovalDecision(decision protocol.ApprovalDecision) approval.Decision {
	switch decision {
	case protocol.ApprovalApprove:
		return approval.Allow
	case protocol.ApprovalDeny:
		return approval.Deny
	default:
		return ""
	}
}

func portableItemStatus(path string, value protocol.ItemStatus) (transcript.ItemStatus, error) {
	switch value {
	case protocol.ItemStatusRunning:
		return transcript.ItemRunning, nil
	case protocol.ItemStatusCompleted:
		return transcript.ItemCompleted, nil
	case protocol.ItemStatusIncomplete:
		return transcript.ItemIncomplete, nil
	default:
		return "", invalidArtifact(path, "unknown value %q", value)
	}
}

func portableItemKind(path string, value protocol.ItemType) (transcript.ItemKind, error) {
	kind, known := itemKinds.domain(value)
	if !known {
		return "", invalidArtifact(path, "unknown value %q", value)
	}
	return kind, nil
}

func portableContentFromArtifact(path string, artifact protocol.ContentBlock) (transcript.ContentBlock, error) {
	decoded, decodeErr := decodeContent(artifact)
	if decodeErr != nil {
		return transcript.ContentBlock{}, invalidArtifact(path+"."+decodeErr.field, "%s", decodeErr.detail)
	}
	return decoded, nil
}

func portableQuestionFromArtifact(path string, artifact protocol.Question) (transcript.Question, error) {
	fields := make([]transcript.QuestionField, len(artifact.Fields))
	for index, field := range artifact.Fields {
		kind, err := portableQuestionFieldKind(fmt.Sprintf("%s.fields[%d].type", path, index), field.Type)
		if err != nil {
			return transcript.Question{}, err
		}
		var options []transcript.QuestionOption
		if len(field.Options) > 0 {
			options = make([]transcript.QuestionOption, len(field.Options))
			for optionIndex, option := range field.Options {
				options[optionIndex] = transcript.QuestionOption{Label: option.Label, Description: option.Description, Preview: option.Preview}
			}
		}
		fields[index] = transcript.QuestionField{
			Prompt: field.Prompt, Header: field.Header, Kind: kind,
			Options: options, Multiple: field.Multiple, AllowCustom: field.AllowCustom,
		}
	}
	return transcript.Question{Fields: fields, Answers: transcript.CloneAnswers(artifact.Answers)}, nil
}

func portableQuestionFieldKind(path string, value protocol.QuestionFieldType) (questioncontract.Kind, error) {
	switch value {
	case protocol.QuestionFieldText:
		return questioncontract.Text, nil
	case protocol.QuestionFieldChoice:
		return questioncontract.Choice, nil
	default:
		return "", invalidArtifact(path, "unknown value %q", value)
	}
}

func portableSafetyClass(path string, value protocol.SafetyClass) (tool.SafetyClass, error) {
	switch value {
	case "":
		return "", nil
	case protocol.SafetyClassSafe:
		return tool.SafetyClassSafe, nil
	case protocol.SafetyClassWrite:
		return tool.SafetyClassWrite, nil
	case protocol.SafetyClassExec:
		return tool.SafetyClassExec, nil
	case protocol.SafetyClassNetwork:
		return tool.SafetyClassNetwork, nil
	default:
		return "", invalidArtifact(path, "unknown value %q", value)
	}
}

func portableToolFromArtifact(path string, artifact protocol.ToolInvocation) (transcript.ToolInvocation, error) {
	arguments, err := tool.ArgumentsFromMap(artifact.Arguments)
	if err != nil {
		return transcript.ToolInvocation{}, invalidArtifact(path+".arguments", "%v", err)
	}
	var result *tool.Result
	if artifact.Result != nil {
		value, err := tool.NewResult(artifact.Result)
		if err != nil {
			return transcript.ToolInvocation{}, invalidArtifact(path+".result", "%v", err)
		}
		result = &value
	}
	input := transcript.AdmittedInput(arguments)
	if artifact.ArgumentsText != "" {
		if !arguments.Equal(tool.Arguments{}) {
			return transcript.ToolInvocation{}, invalidArtifact(path+".arguments", "must be empty beside rejected argumentsText")
		}
		input = transcript.RejectedInput(artifact.ArgumentsText)
	}
	var source tool.Ref
	if artifact.Source != nil {
		source, err = toolRefFromWire(artifact.Source.Ref)
		if err != nil {
			return transcript.ToolInvocation{}, invalidArtifact(path+".source.ref", "%v", err)
		}
		if source.ModelName() != artifact.Source.ModelName || !source.MatchesPublishedName(artifact.Name) {
			return transcript.ToolInvocation{}, invalidArtifact(path+".source", "tool name does not match its captured source")
		}
	}
	return transcript.ToolInvocation{Name: artifact.Name, Source: source, Input: input, Result: result}, nil
}

func portableToolFailureFromArtifact(path string, artifact *protocol.ArtifactProblem) (*tool.Failure, error) {
	if artifact == nil {
		return nil, nil
	}
	kind, known := toolFailureKinds.domain(artifact.Type)
	if !known {
		return nil, invalidArtifact(path+".type", "unknown value %q", artifact.Type)
	}
	return &tool.Failure{
		Kind: kind, Detail: artifact.Detail, DocURL: artifact.DocURL,
	}, nil
}
