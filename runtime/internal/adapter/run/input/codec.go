package input

import (
	"encoding/json/jsontext"
	"errors"
	"fmt"

	questioncontract "github.com/Tangerg/flame/packages/go-sdk/question"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"

	framevalue "github.com/Tangerg/frame/value"
	scoperun "github.com/Tangerg/scope/agent/run"
)

func EncodePrompt(prompt runs.Interrupt) (jsontext.Value, error) {
	if err := prompt.Validate(); err != nil {
		return nil, err
	}
	encoded, err := scoperun.EncodePayload(promptWireFrom(prompt))
	if err != nil {
		return nil, fmt.Errorf("run input codec: encode prompt: %w", err)
	}
	return encoded.JSON(), nil
}

func DecodePrompt(raw []byte) (runs.Interrupt, error) {
	wire, err := decode[interruptWire](raw)
	if err != nil {
		return nil, fmt.Errorf("run input codec: decode interrupt: %w", err)
	}
	interrupt, err := wire.interrupt()
	if err != nil {
		return nil, fmt.Errorf("run input codec: decode interrupt: %w", err)
	}
	if err := interrupt.Validate(); err != nil {
		return nil, err
	}
	return interrupt, nil
}

func decodeResolution(raw []byte) (run.InterruptResolution, error) {
	wire, err := decode[resolutionPayload](raw)
	if err != nil {
		return run.InterruptResolution{}, fmt.Errorf("run input codec: decode resolution: %w", err)
	}
	return wire.resolution()
}

func EncodeResolution(resolution run.InterruptResolution) (jsontext.Value, error) {
	wire := resolutionPayload{}
	switch resolution.Type() {
	case run.InterruptQuestion:
		wire.Question = &questionAnswerPayload{Answers: resolution.Question().Answers()}
	case run.InterruptApproval:
		value := resolution.Approval()
		wire.Approval = &approvalAnswerPayload{Decision: value.Decision, Arguments: value.EditedArguments, Reason: value.Reason, RememberScope: value.RememberScope}
	default:
		return nil, errors.New("run input codec: answer is required")
	}
	encoded, err := scoperun.EncodePayload(wire)
	if err != nil {
		return nil, fmt.Errorf("run input codec: encode resolution: %w", err)
	}
	return encoded.JSON(), nil
}

func decode[T any](raw []byte) (T, error) {
	payload, err := scoperun.ParsePayload(raw)
	if err != nil {
		var zero T
		return zero, err
	}
	return payload.Decode[T]()
}

type interruptWire struct {
	Kind     run.InterruptType   `json:"kind"`
	Approval *approvalPromptWire `json:"approval,omitzero"`
	Question *questionPromptWire `json:"question,omitzero"`
}

type approvalPromptWire struct {
	Tool              tool.Ref          `json:"tool"`
	QualifiedName     bool              `json:"qualifiedName,omitzero"`
	SourceFingerprint framevalue.Digest `json:"sourceFingerprint,omitzero"`
	CallID            string            `json:"callId"`
	Arguments         string            `json:"arguments"`
	SafetyClass       tool.SafetyClass  `json:"safetyClass"`
	Risk              tool.RiskLevel    `json:"risk"`
	Reason            string            `json:"reason,omitempty"`
	Rememberable      bool              `json:"rememberable,omitzero"`
}

type questionPromptWire struct {
	ToolName  string              `json:"toolName"`
	Arguments string              `json:"arguments"`
	Fields    []questionFieldWire `json:"fields"`
}

type questionFieldWire struct {
	Prompt      string                `json:"prompt"`
	Header      string                `json:"header,omitempty"`
	Kind        questioncontract.Kind `json:"kind"`
	Options     []questionOptionWire  `json:"options,omitempty"`
	Multiple    bool                  `json:"multiple,omitzero"`
	AllowCustom bool                  `json:"allowCustom,omitzero"`
}

type questionOptionWire struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Preview     string `json:"preview,omitempty"`
}

func promptWireFrom(interrupt runs.Interrupt) interruptWire {
	result := interruptWire{Kind: interrupt.Type()}
	switch prompt := interrupt.(type) {
	case runs.ApprovalPrompt:
		result.Approval = &approvalPromptWire{
			Tool: prompt.Tool, QualifiedName: prompt.QualifiedName, SourceFingerprint: prompt.SourceFingerprint, CallID: prompt.CallID, Arguments: prompt.Arguments,
			SafetyClass: prompt.SafetyClass, Risk: prompt.Risk, Reason: prompt.Reason, Rememberable: prompt.Rememberable,
		}
	case runs.QuestionPrompt:
		result.Question = &questionPromptWire{
			ToolName: prompt.ToolName, Arguments: prompt.Arguments,
			Fields: questionFieldWiresFrom(prompt.Fields),
		}
	}
	return result
}

func (i interruptWire) interrupt() (runs.Interrupt, error) {
	switch {
	case i.Kind == run.InterruptApproval && i.Approval != nil && i.Question == nil:
		prompt := i.Approval
		return runs.ApprovalPrompt{
			Tool: prompt.Tool, QualifiedName: prompt.QualifiedName, SourceFingerprint: prompt.SourceFingerprint, CallID: prompt.CallID, Arguments: prompt.Arguments,
			SafetyClass: prompt.SafetyClass, Risk: prompt.Risk, Reason: prompt.Reason, Rememberable: prompt.Rememberable,
		}, nil
	case i.Kind == run.InterruptQuestion && i.Question != nil && i.Approval == nil:
		return runs.QuestionPrompt{
			ToolName: i.Question.ToolName, Arguments: i.Question.Arguments,
			Fields: questionFieldsFrom(i.Question.Fields),
		}, nil
	default:
		return nil, fmt.Errorf("interrupt kind %q does not name its one prompt", i.Kind)
	}
}

func questionFieldWiresFrom(fields []transcript.QuestionField) []questionFieldWire {
	if fields == nil {
		return nil
	}
	result := make([]questionFieldWire, len(fields))
	for index, field := range fields {
		result[index] = questionFieldWire{
			Prompt: field.Prompt, Header: field.Header, Kind: field.Kind,
			Multiple: field.Multiple, AllowCustom: field.AllowCustom,
		}
		for _, option := range field.Options {
			result[index].Options = append(result[index].Options, questionOptionWire(option))
		}
	}
	return result
}

func questionFieldsFrom(wires []questionFieldWire) []transcript.QuestionField {
	if len(wires) == 0 {
		return nil
	}
	result := make([]transcript.QuestionField, len(wires))
	for index, wire := range wires {
		result[index] = transcript.QuestionField{
			Prompt: wire.Prompt, Header: wire.Header, Kind: wire.Kind,
			Multiple: wire.Multiple, AllowCustom: wire.AllowCustom,
		}
		for _, option := range wire.Options {
			result[index].Options = append(result[index].Options, transcript.QuestionOption(option))
		}
	}
	return result
}

type resolutionPayload struct {
	Question *questionAnswerPayload `json:"question,omitempty" jsonschema:"oneof_required=question"`
	Approval *approvalAnswerPayload `json:"approval,omitempty" jsonschema:"oneof_required=approval"`
}

type questionAnswerPayload struct {
	Answers [][]string `json:"answers" jsonschema:"minItems=1"`
}

type approvalAnswerPayload struct {
	Decision      approval.Decision `json:"decision" jsonschema:"enum=allow,enum=deny"`
	Arguments     string            `json:"arguments,omitempty"`
	Reason        string            `json:"reason,omitempty"`
	RememberScope approval.Scope    `json:"rememberScope,omitempty"`
}

func (r resolutionPayload) resolution() (run.InterruptResolution, error) {
	if (r.Question == nil) == (r.Approval == nil) {
		return run.InterruptResolution{}, errors.New("run input codec: exactly one answer kind is required")
	}
	if r.Question != nil {
		return run.NewQuestionAnswer(r.Question.Answers)
	}
	return run.NewApprovalAnswer(approval.Resolution{
		Decision: r.Approval.Decision, EditedArguments: r.Approval.Arguments,
		Reason: r.Approval.Reason, RememberScope: r.Approval.RememberScope,
	})
}
