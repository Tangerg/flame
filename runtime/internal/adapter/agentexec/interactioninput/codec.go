package interactioninput

import (
	"encoding/json/jsontext"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	agent "github.com/Tangerg/scope/agent"
)

// EncodePrompt converts one validated product interrupt to its strict executor
// boundary representation.
func EncodePrompt(prompt runs.Interrupt) (jsontext.Value, error) {
	if err := prompt.Validate(); err != nil {
		return nil, err
	}
	encoded, err := agent.EncodePayload(promptWireFrom(prompt))
	if err != nil {
		return nil, fmt.Errorf("agentexec interaction input codec: encode prompt: %w", err)
	}
	return encoded.JSON(), nil
}

// DecodePrompt restores an application interrupt from persisted executor input
// JSON. Field names must match exactly; duplicate, unknown, and trailing values
// are rejected.
func DecodePrompt(raw []byte) (runs.Interrupt, error) {
	wire, err := decode[interruptWire](raw)
	if err != nil {
		return runs.Interrupt{}, fmt.Errorf("agentexec interaction input codec: decode interrupt: %w", err)
	}
	interrupt, err := wire.interrupt()
	if err != nil {
		return runs.Interrupt{}, err
	}
	if err := interrupt.Validate(); err != nil {
		return runs.Interrupt{}, err
	}
	return interrupt, nil
}

// DecodeResolution restores a typed user decision from persisted agent-process
// response JSON. It applies the same exact-field and single-value contract as
// [DecodePrompt].
func DecodeResolution(raw []byte) (interrupt.Resolution, error) {
	wire, err := decode[ResolutionPayload](raw)
	if err != nil {
		return interrupt.Resolution{}, fmt.Errorf("agentexec interaction input codec: decode resolution: %w", err)
	}
	return wire.Resolution()
}

// EncodeResolution converts a typed human decision to the JSON the executor
// validates against its pending-input response schema before continuing.
func EncodeResolution(resolution interrupt.Resolution) (jsontext.Value, error) {
	if resolution.RememberScope != "" && !resolution.RememberScope.Valid() {
		return nil, fmt.Errorf("agentexec interaction input codec: unknown remember scope %q", resolution.RememberScope)
	}
	approved := resolution.Approved
	encoded, err := agent.EncodePayload(ResolutionPayload{
		Approved: &approved, Arguments: resolution.Arguments, Answers: resolution.Answers,
		Reason: resolution.Reason, RememberScope: resolution.RememberScope,
	})
	if err != nil {
		return nil, fmt.Errorf("agentexec interaction input codec: encode resolution: %w", err)
	}
	return encoded.JSON(), nil
}

func decode[T any](raw []byte) (T, error) {
	payload, err := agent.ParsePayload(raw)
	if err != nil {
		var zero T
		return zero, err
	}
	return payload.Decode[T]()
}

type interruptWire struct {
	Kind     interrupt.Kind      `json:"kind"`
	Approval *approvalPromptWire `json:"approval,omitzero"`
	Question *questionPromptWire `json:"question,omitzero"`
}

type approvalPromptWire struct {
	CallID       string           `json:"callId,omitempty"`
	ToolName     string           `json:"toolName"`
	Arguments    string           `json:"arguments"`
	SafetyClass  tool.SafetyClass `json:"safetyClass"`
	Risk         tool.RiskLevel   `json:"risk,omitempty"`
	Reason       string           `json:"reason,omitempty"`
	Rememberable bool             `json:"rememberable,omitempty"`
}

type questionPromptWire struct {
	ToolName  string                  `json:"toolName"`
	Arguments string                  `json:"arguments"`
	Fields    []questionFieldSpecWire `json:"fields"`
}

type questionFieldSpecWire struct {
	Prompt      string               `json:"prompt"`
	Header      string               `json:"header,omitempty"`
	Options     []questionOptionWire `json:"options,omitempty"`
	Multiple    bool                 `json:"multiple,omitempty"`
	AllowCustom bool                 `json:"allowCustom,omitempty"`
}

type questionOptionWire struct {
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

func promptWireFrom(interrupt runs.Interrupt) interruptWire {
	result := interruptWire{Kind: interrupt.Kind}
	if prompt := interrupt.Approval; prompt != nil {
		result.Approval = &approvalPromptWire{
			CallID: prompt.CallID, ToolName: prompt.ToolName, Arguments: prompt.Arguments,
			SafetyClass: prompt.SafetyClass, Risk: prompt.Risk, Reason: prompt.Reason, Rememberable: prompt.Rememberable,
		}
	}
	if prompt := interrupt.Question; prompt != nil {
		result.Question = &questionPromptWire{
			ToolName: prompt.ToolName, Arguments: prompt.Arguments,
			Fields: questionFieldWiresFrom(prompt.Fields),
		}
	}
	return result
}

func (i interruptWire) interrupt() (runs.Interrupt, error) {
	if !i.Kind.Valid() {
		return runs.Interrupt{}, fmt.Errorf("agentexec interaction input codec: unknown interrupt kind %q", i.Kind)
	}
	result := runs.Interrupt{Kind: i.Kind}
	if prompt := i.Approval; prompt != nil {
		if !prompt.SafetyClass.Valid() {
			return runs.Interrupt{}, fmt.Errorf("agentexec interaction input codec: unknown safety class %q", prompt.SafetyClass)
		}
		if prompt.Risk != "" && !prompt.Risk.Valid() {
			return runs.Interrupt{}, fmt.Errorf("agentexec interaction input codec: unknown risk level %q", prompt.Risk)
		}
		result.Approval = &runs.ApprovalPrompt{
			CallID: prompt.CallID, ToolName: prompt.ToolName, Arguments: prompt.Arguments,
			SafetyClass: prompt.SafetyClass, Risk: prompt.Risk, Reason: prompt.Reason, Rememberable: prompt.Rememberable,
		}
	}
	if prompt := i.Question; prompt != nil {
		result.Question = &runs.QuestionPrompt{
			ToolName: prompt.ToolName, Arguments: prompt.Arguments,
			Fields: questionFieldSpecsFrom(i.Question.Fields),
		}
	}
	return result, nil
}

func questionFieldWiresFrom(specs []runs.QuestionFieldSpec) []questionFieldSpecWire {
	if specs == nil {
		return nil
	}
	result := make([]questionFieldSpecWire, len(specs))
	for index, spec := range specs {
		result[index] = questionFieldSpecWire{
			Prompt: spec.Prompt, Header: spec.Header, Multiple: spec.Multiple,
			AllowCustom: spec.AllowCustom,
			Options:     questionOptionWiresFrom(spec.Options),
		}
	}
	return result
}

func questionOptionWiresFrom(options []runs.QuestionOptionSpec) []questionOptionWire {
	if options == nil {
		return nil
	}
	result := make([]questionOptionWire, len(options))
	for index, option := range options {
		result[index] = questionOptionWire{Label: option.Label, Description: option.Description}
	}
	return result
}

func questionFieldSpecsFrom(specs []questionFieldSpecWire) []runs.QuestionFieldSpec {
	if len(specs) == 0 {
		return nil
	}
	result := make([]runs.QuestionFieldSpec, len(specs))
	for index, spec := range specs {
		result[index] = runs.QuestionFieldSpec{
			Prompt: spec.Prompt, Header: spec.Header, Multiple: spec.Multiple,
			AllowCustom: spec.AllowCustom,
			Options:     questionOptionsFrom(spec.Options),
		}
	}
	return result
}

func questionOptionsFrom(options []questionOptionWire) []runs.QuestionOptionSpec {
	if len(options) == 0 {
		return nil
	}
	result := make([]runs.QuestionOptionSpec, len(options))
	for index, option := range options {
		result[index] = runs.QuestionOptionSpec{Label: option.Label, Description: option.Description}
	}
	return result
}

// ResolutionPayload is the exact technical response shape used by the executor
// continuation codec. Callers should use [EncodeResolution] and
// [DecodeResolution].
type ResolutionPayload struct {
	Approved      *bool          `json:"approved"`
	Arguments     string         `json:"arguments,omitempty"`
	Answers       [][]string     `json:"answers,omitempty"`
	Reason        string         `json:"reason,omitempty"`
	RememberScope approval.Scope `json:"remember_scope,omitempty"`
}

// Resolution converts the validated technical payload to its Domain value.
func (r ResolutionPayload) Resolution() (interrupt.Resolution, error) {
	if r.Approved == nil {
		return interrupt.Resolution{}, errors.New("agentexec interaction input codec: approved is required")
	}
	if r.RememberScope != "" && !r.RememberScope.Valid() {
		return interrupt.Resolution{}, fmt.Errorf("agentexec interaction input codec: unknown remember scope %q", r.RememberScope)
	}
	answers := r.Answers
	if len(answers) == 0 {
		answers = nil
	}
	return interrupt.Resolution{
		Approved: *r.Approved, Arguments: r.Arguments, Answers: answers,
		Reason: r.Reason, RememberScope: r.RememberScope,
	}, nil
}
