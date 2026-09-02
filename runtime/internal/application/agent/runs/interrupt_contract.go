package runs

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
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
// running the hook or policy decision a second time.
type ApprovalPrompt struct {
	CallID      string
	ToolName    string
	Arguments   string
	SafetyClass tool.SafetyClass
	Risk        tool.RiskLevel
	Reason      string
	// Rememberable distinguishes ordinary policy approvals from one-off
	// confirmations such as the doom-loop brake. It must persist with the
	// prompt so a resumed execution cannot accidentally create a standing rule.
	Rememberable bool
}

// QuestionPrompt is the complete durable plan for a question-producing tool
// call. CallID is filled by the execution ACL when the prompt crosses the Tool
// boundary; ToolName and Arguments preserve the logical call for compatibility
// with older checkpoints and non-execution tests. Fields are the client-facing
// answer schema.
type QuestionPrompt struct {
	CallID    string
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

// Tool returns the logical tool call that owns this interrupt.
func (i Interrupt) Tool() (name, arguments string) {
	switch i.Kind {
	case interrupt.Approval:
		if i.Approval != nil {
			return i.Approval.ToolName, i.Approval.Arguments
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
	if _, err := runtimeidentity.ParseEffect(a.CallID); err != nil {
		return fmt.Errorf("runs: approval: %w", err)
	}
	if strings.TrimSpace(a.ToolName) == "" {
		return errors.New("runs: approval tool name is required")
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
	if _, _, err := runtimeidentity.ParseOptionalEffect(q.CallID); err != nil {
		return fmt.Errorf("runs: question: %w", err)
	}
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
