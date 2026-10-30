package persistence

import (
	"encoding/base64"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"time"

	questioncontract "github.com/Tangerg/flame/packages/go-sdk/question"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
)

func encodeUserContent(content []transcript.ContentBlock) ([]byte, error) {
	payload := make([]contentPayload, len(content))
	for index, block := range content {
		if err := block.Validate(); err != nil {
			return nil, err
		}
		var err error
		payload[index], err = encodeContentPayload(block)
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(payload)
}

func decodeUserContent(encoded []byte) ([]transcript.ContentBlock, error) {
	var payload []contentPayload
	if err := json.Unmarshal(encoded, &payload, json.RejectUnknownMembers(true)); err != nil {
		return nil, err
	}
	content := make([]transcript.ContentBlock, len(payload))
	for index, block := range payload {
		var err error
		content[index], err = decodeContentPayload(block)
		if err != nil {
			return nil, err
		}
		if err := content[index].Validate(); err != nil {
			return nil, err
		}
	}
	return content, nil
}

type transcriptItemPayload struct {
	Status                 transcript.ItemStatus   `json:"status"`
	FinishedAt             *int64                  `json:"finishedAt,omitzero"`
	ExecutionDurationNanos *int64                  `json:"executionDurationNanos,omitzero"`
	Kind                   transcript.ItemKind     `json:"kind"`
	Phase                  transcript.MessagePhase `json:"phase,omitempty"`
	Content                []contentPayload        `json:"content,omitempty"`
	Text                   string                  `json:"text,omitempty"`
	Redacted               bool                    `json:"redacted,omitzero"`
	Question               *questionPayload        `json:"question,omitzero"`
	Tool                   *toolInvocationPayload  `json:"tool,omitzero"`
	SafetyClass            tool.SafetyClass        `json:"safetyClass,omitempty"`
	ApprovalDecision       approval.Decision       `json:"approvalDecision,omitempty"`
	EditedArguments        string                  `json:"editedArguments,omitempty"`
	DenialReason           string                  `json:"denialReason,omitempty"`
	RememberScope          approval.Scope          `json:"rememberScope,omitempty"`
	Failure                *toolFailurePayload     `json:"failure,omitzero"`
	Summary                string                  `json:"summary,omitempty"`
	DroppedMessages        int                     `json:"droppedMessages,omitzero"`
}

type contentPayload struct {
	Kind      transcript.ContentKind `json:"kind"`
	Text      string                 `json:"text,omitempty"`
	MediaType string                 `json:"mediaType,omitempty"`
	Data      string                 `json:"data,omitempty"`
}

type questionPayload struct {
	Fields  []questionFieldPayload `json:"fields"`
	Answers [][]string             `json:"answers,omitempty"`
}

type questionFieldPayload struct {
	Prompt      string                  `json:"prompt"`
	Header      string                  `json:"header,omitempty"`
	Kind        questioncontract.Kind   `json:"kind"`
	Options     []questionOptionPayload `json:"options,omitempty"`
	Multiple    bool                    `json:"multiple,omitzero"`
	AllowCustom bool                    `json:"allowCustom,omitzero"`
}

type questionOptionPayload struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Preview     string `json:"preview,omitempty"`
}

type toolInvocationPayload struct {
	ArgumentsText string         `json:"argumentsText,omitempty"`
	Name          string         `json:"name"`
	Arguments     jsontext.Value `json:"arguments"`
	Result        jsontext.Value `json:"result,omitzero"`
}

type toolFailurePayload struct {
	Kind   tool.FailureKind `json:"kind"`
	Detail string           `json:"detail,omitempty"`
	DocURL string           `json:"docUrl,omitempty"`
}

func encodeTranscriptItem(item transcript.Item) ([]byte, error) {
	payload := transcriptItemPayload{Status: item.Status(), Kind: item.Kind()}
	var content []transcript.ContentBlock
	switch variant := item.Body().(type) {
	case transcript.UserMessageBody:
		content = variant.Content
	case transcript.AgentMessageBody:
		payload.Phase, content = variant.Phase, variant.Content
	case transcript.ReasoningBody:
		payload.Text, payload.Redacted = variant.Text, variant.Redacted
	case transcript.QuestionBody:
		encoded, err := encodeQuestionPayload(variant.Question)
		if err != nil {
			return nil, err
		}
		payload.Question = &encoded
	case transcript.CompactionBody:
		payload.Summary, payload.DroppedMessages = variant.Summary, variant.DroppedMessages
	case transcript.ToolCallBody:
		if err := encodeToolCall(&payload, variant); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unknown item kind %q", item.Kind())
	}
	if len(content) > 0 {
		payload.Content = make([]contentPayload, len(content))
		for index, block := range content {
			encoded, err := encodeContentPayload(block)
			if err != nil {
				return nil, fmt.Errorf("content %d: %w", index, err)
			}
			payload.Content[index] = encoded
		}
	}
	return encodeStoredJSON(payload)
}

func encodeToolCall(payload *transcriptItemPayload, call transcript.ToolCallBody) error {
	invocation := encodeToolInvocationPayload(call.Invocation)
	payload.Tool = &invocation
	payload.SafetyClass = call.SafetyClass
	if call.Approval != nil {
		payload.ApprovalDecision = call.Approval.Decision
		payload.EditedArguments = call.Approval.EditedArguments
		payload.DenialReason = call.Approval.Reason
		payload.RememberScope = call.Approval.RememberScope
	}
	if !call.FinishedAt.IsZero() {
		payload.FinishedAt = new(call.FinishedAt.UnixNano())
	}
	if call.ExecutionDuration != nil {
		payload.ExecutionDurationNanos = new(call.ExecutionDuration.Nanoseconds())
	}
	if call.Failure != nil {
		encoded, err := encodeToolFailurePayload(*call.Failure)
		if err != nil {
			return err
		}
		payload.Failure = &encoded
	}
	return nil
}

func decodeTranscriptItem(data []byte) (transcript.ItemSnapshot, error) {
	var payload transcriptItemPayload
	if err := decodeStoredJSON(data, &payload); err != nil {
		return transcript.ItemSnapshot{}, err
	}
	if !payload.Status.Valid() {
		return transcript.ItemSnapshot{}, fmt.Errorf("unknown item status %q", payload.Status)
	}
	var content []transcript.ContentBlock
	if len(payload.Content) > 0 {
		content = make([]transcript.ContentBlock, len(payload.Content))
		for index, encoded := range payload.Content {
			block, err := decodeContentPayload(encoded)
			if err != nil {
				return transcript.ItemSnapshot{}, fmt.Errorf("content %d: %w", index, err)
			}
			content[index] = block
		}
	}
	snapshot := transcript.ItemSnapshot{Status: payload.Status}
	switch payload.Kind {
	case transcript.UserMessage:
		snapshot.Body = transcript.UserMessageBody{Content: content}
	case transcript.AgentMessage:
		snapshot.Body = transcript.AgentMessageBody{Phase: payload.Phase, Content: content}
	case transcript.Reasoning:
		snapshot.Body = transcript.ReasoningBody{Text: payload.Text, Redacted: payload.Redacted}
	case transcript.QuestionItem:
		if payload.Question == nil {
			return transcript.ItemSnapshot{}, errors.New("question item has no prompt")
		}
		question, err := decodeQuestionPayload(*payload.Question)
		if err != nil {
			return transcript.ItemSnapshot{}, err
		}
		snapshot.Body = transcript.QuestionBody{Question: question}
	case transcript.Compaction:
		snapshot.Body = transcript.CompactionBody{Summary: payload.Summary, DroppedMessages: payload.DroppedMessages}
	case transcript.ToolCall:
		call, err := decodeToolCall(payload)
		if err != nil {
			return transcript.ItemSnapshot{}, err
		}
		snapshot.Body = call
	default:
		return transcript.ItemSnapshot{}, fmt.Errorf("unknown item kind %q", payload.Kind)
	}
	return snapshot, nil
}

func decodeToolCall(payload transcriptItemPayload) (transcript.ToolCallBody, error) {
	if payload.Tool == nil {
		return transcript.ToolCallBody{}, errors.New("tool call item has no invocation")
	}
	invocation, err := decodeToolInvocationPayload(*payload.Tool)
	if err != nil {
		return transcript.ToolCallBody{}, err
	}
	call := transcript.ToolCallBody{Invocation: invocation, SafetyClass: payload.SafetyClass}
	if payload.ApprovalDecision != "" {
		call.Approval = &approval.Resolution{
			Decision: payload.ApprovalDecision, EditedArguments: payload.EditedArguments,
			Reason: payload.DenialReason, RememberScope: payload.RememberScope,
		}
	}
	if payload.FinishedAt != nil {
		call.FinishedAt = time.Unix(0, *payload.FinishedAt).UTC()
	}
	if payload.ExecutionDurationNanos != nil {
		call.ExecutionDuration = new(time.Duration(*payload.ExecutionDurationNanos))
	}
	if payload.Failure != nil {
		failure, err := decodeToolFailurePayload(*payload.Failure)
		if err != nil {
			return transcript.ToolCallBody{}, err
		}
		call.Failure = &failure
	}
	return call, nil
}

func encodeContentPayload(block transcript.ContentBlock) (contentPayload, error) {
	switch block.Kind {
	case transcript.TextContent:
		return contentPayload{Kind: block.Kind, Text: block.Text}, nil
	case transcript.ImageContent:
		return contentPayload{
			Kind: block.Kind, MediaType: block.MediaType,
			Data: base64.StdEncoding.EncodeToString(block.Bytes),
		}, nil
	default:
		return contentPayload{}, fmt.Errorf("unknown content kind %q", block.Kind)
	}
}

func decodeContentPayload(payload contentPayload) (transcript.ContentBlock, error) {
	data, err := base64.StdEncoding.DecodeString(payload.Data)
	if err != nil {
		return transcript.ContentBlock{}, fmt.Errorf("decode content data: %w", err)
	}
	return transcript.ContentBlock{Kind: payload.Kind, Text: payload.Text, MediaType: payload.MediaType, Bytes: data}, nil
}

func encodeQuestionPayload(question transcript.Question) (questionPayload, error) {
	encoded := questionPayload{
		Fields:  make([]questionFieldPayload, len(question.Fields)),
		Answers: transcript.CloneAnswers(question.Answers),
	}
	for index, field := range question.Fields {
		if !field.Kind.Valid() {
			return questionPayload{}, fmt.Errorf("question field %d has unknown kind %q", index, field.Kind)
		}
		encodedField := questionFieldPayload{
			Prompt: field.Prompt, Header: field.Header, Kind: field.Kind,
			Multiple: field.Multiple, AllowCustom: field.AllowCustom,
		}
		if len(field.Options) > 0 {
			encodedField.Options = make([]questionOptionPayload, len(field.Options))
			for optionIndex, option := range field.Options {
				encodedField.Options[optionIndex] = questionOptionPayload{
					Label: option.Label, Description: option.Description, Preview: option.Preview,
				}
			}
		}
		encoded.Fields[index] = encodedField
	}
	return encoded, nil
}

func decodeQuestionPayload(payload questionPayload) (transcript.Question, error) {
	question := transcript.Question{
		Fields:  make([]transcript.QuestionField, len(payload.Fields)),
		Answers: transcript.CloneAnswers(payload.Answers),
	}
	for index, field := range payload.Fields {
		if !field.Kind.Valid() {
			return transcript.Question{}, fmt.Errorf("question field %d has unknown kind %q", index, field.Kind)
		}
		decoded := transcript.QuestionField{
			Prompt: field.Prompt, Header: field.Header, Kind: field.Kind,
			Multiple: field.Multiple, AllowCustom: field.AllowCustom,
		}
		if len(field.Options) > 0 {
			decoded.Options = make([]transcript.QuestionOption, len(field.Options))
			for optionIndex, option := range field.Options {
				decoded.Options[optionIndex] = transcript.QuestionOption{
					Label: option.Label, Description: option.Description, Preview: option.Preview,
				}
			}
		}
		question.Fields[index] = decoded
	}
	return question, nil
}

func encodeToolInvocationPayload(invocation transcript.ToolInvocation) toolInvocationPayload {
	payload := toolInvocationPayload{
		Name: invocation.Name, ArgumentsText: invocation.ArgumentsText, Arguments: jsontext.Value(invocation.Arguments.Canonical()),
	}
	if invocation.Result != nil {
		payload.Result = jsontext.Value(invocation.Result.Canonical())
	}
	return payload
}

func decodeToolInvocationPayload(payload toolInvocationPayload) (transcript.ToolInvocation, error) {
	arguments, err := tool.ParseArguments(string(payload.Arguments))
	if err != nil {
		return transcript.ToolInvocation{}, fmt.Errorf("tool arguments: %w", err)
	}
	invocation := transcript.ToolInvocation{Name: payload.Name, Arguments: arguments, ArgumentsText: payload.ArgumentsText}
	if len(payload.Result) > 0 {
		result, err := tool.ParseResult(payload.Result)
		if err != nil {
			return transcript.ToolInvocation{}, fmt.Errorf("tool result: %w", err)
		}
		invocation.Result = &result
	}
	return invocation, nil
}

func encodeToolFailurePayload(failure tool.Failure) (toolFailurePayload, error) {
	if !failure.Kind.Valid() {
		return toolFailurePayload{}, fmt.Errorf("unknown Tool failure kind %q", failure.Kind)
	}
	return toolFailurePayload{
		Kind: failure.Kind, Detail: failure.Detail, DocURL: failure.DocURL,
	}, nil
}

func decodeToolFailurePayload(payload toolFailurePayload) (tool.Failure, error) {
	if !payload.Kind.Valid() {
		return tool.Failure{}, fmt.Errorf("unknown Tool failure kind %q", payload.Kind)
	}
	return tool.Failure{
		Kind: payload.Kind, Detail: payload.Detail, DocURL: payload.DocURL,
	}, nil
}
