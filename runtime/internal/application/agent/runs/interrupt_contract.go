package runs

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	"github.com/Tangerg/flame/runtime/internal/optional"
)

// InterruptFunc is the consumer-owned capability a tool uses to park the
// current execution on one application interrupt. Tool packages depend only on
// this contract and never on execution internals.
type InterruptFunc func(context.Context, string, Interrupt) (interrupt.Resolution, error)

// InterruptUnavailable is the fail-closed default for a tool environment that
// has no execution interrupt provider.
func InterruptUnavailable(context.Context, string, Interrupt) (interrupt.Resolution, error) {
	return interrupt.Resolution{}, errors.New("runs: execution interrupts are unavailable")
}

// ApprovalPrompt is the complete durable plan for one gated tool call.
// Arguments are the effective arguments after PreToolUse rewriting, so a
// continuation (including one restored after restart) can resume without
// running the hook or policy decision a second time. Tool names the gated call;
// its model-visible name is Tool.ModelName.
type ApprovalPrompt struct {
	Tool              tool.Ref
	SourceFingerprint fingerprint.Digest
	CallID            string
	Arguments         string
	SafetyClass       tool.SafetyClass
	Risk              tool.RiskLevel
	Reason            string
	// Rememberable persists whether the response may create a standing rule;
	// restoration must not make a one-off decision eligible for reuse.
	Rememberable bool
}

// QuestionPrompt is the complete durable plan for a question-producing tool
// call. ToolName and Arguments preserve the semantic input for restoring the
// question-producing handler. Fields are the client-facing answer schema.
type QuestionPrompt struct {
	ToolName  string
	Arguments string
	Fields    []QuestionFieldSpec
}

// QuestionFieldSpec is one ordered answer field. An empty Options slice means
// free-text; otherwise 2-4 unique options are accepted. A response still carries
// one entry per field, but an empty entry explicitly skips that field.
type QuestionFieldSpec struct {
	Prompt      string
	Header      string
	Options     []QuestionOptionSpec
	Multiple    bool
	AllowCustom bool
}

type QuestionOptionSpec struct {
	Label       string
	Description string
}

// Interrupt is the durable product request for external input. Exactly
// one payload must be present and must match Kind. Executor continuation data is
// deliberately absent.
type Interrupt struct {
	Kind     interrupt.Kind
	Approval *ApprovalPrompt
	Question *QuestionPrompt
}

func cloneInterrupt(value Interrupt) Interrupt {
	value.Approval = optional.Clone(value.Approval)
	if value.Question != nil {
		question := *value.Question
		question.Fields = slices.Clone(question.Fields)
		for index := range question.Fields {
			question.Fields[index].Options = slices.Clone(question.Fields[index].Options)
		}
		value.Question = &question
	}
	return value
}

// Tool returns the logical tool call that owns this interrupt.
func (i Interrupt) Tool() (name, arguments string) {
	switch i.Kind {
	case interrupt.Approval:
		if i.Approval != nil {
			return i.Approval.Tool.ModelName(), i.Approval.Arguments
		}
	case interrupt.Question:
		if i.Question != nil {
			return i.Question.ToolName, i.Question.Arguments
		}
	}
	return "", ""
}

// Validate rejects malformed or ambiguous envelopes before they become
// a durable Pending aggregate or application events.
func (i Interrupt) Validate() error {
	switch i.Kind {
	case interrupt.Approval:
		if i.Approval == nil || i.Question != nil {
			return errors.New("runs: malformed approval interrupt")
		}
		return i.Approval.validate()
	case interrupt.Question:
		if i.Question == nil || i.Approval != nil {
			return errors.New("runs: malformed question interrupt")
		}
		return i.Question.validate()
	default:
		return fmt.Errorf("runs: unknown interrupt kind %q", i.Kind)
	}
}

func (a ApprovalPrompt) validate() error {
	if err := a.Tool.ValidateFingerprint(a.SourceFingerprint); err != nil {
		return err
	}
	if err := runtimeidentity.ValidateEffect(a.CallID); err != nil {
		return fmt.Errorf("runs: approval: %w", err)
	}
	if err := validateArguments(a.Arguments); err != nil {
		return fmt.Errorf("runs: approval arguments: %w", err)
	}
	if !a.SafetyClass.Valid() {
		return fmt.Errorf("runs: unknown approval safety class %q", a.SafetyClass)
	}
	if !a.Risk.Valid() {
		return fmt.Errorf("runs: unknown approval risk %q", a.Risk)
	}
	return nil
}

func (q QuestionPrompt) validate() error {
	if strings.TrimSpace(q.ToolName) == "" {
		return errors.New("runs: question tool name is required")
	}
	if err := validateArguments(q.Arguments); err != nil {
		return fmt.Errorf("runs: question arguments: %w", err)
	}
	if err := q.question().Validate(); err != nil {
		return fmt.Errorf("runs: question: %w", err)
	}
	return nil
}

func (q QuestionPrompt) question() transcript.Question {
	fields := make([]transcript.QuestionField, len(q.Fields))
	for index, spec := range q.Fields {
		field := transcript.QuestionField{
			Prompt: spec.Prompt,
			Header: spec.Header,
			Kind:   transcript.QuestionText,
		}
		if len(spec.Options) > 0 {
			field.Kind = transcript.QuestionChoice
			field.Multiple = spec.Multiple
			field.AllowCustom = spec.AllowCustom
			field.Options = make([]transcript.QuestionOption, len(spec.Options))
			for optionIndex, option := range spec.Options {
				field.Options[optionIndex] = transcript.QuestionOption{
					Label:       option.Label,
					Description: option.Description,
				}
			}
		}
		fields[index] = field
	}
	return transcript.Question{Fields: fields}
}

func validateArguments(arguments string) error {
	if strings.TrimSpace(arguments) == "" {
		return fmt.Errorf("%w: value is required", tool.ErrInvalidArguments)
	}
	_, err := tool.ParseArguments(arguments)
	return err
}
