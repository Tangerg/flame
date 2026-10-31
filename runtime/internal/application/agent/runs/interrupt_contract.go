package runs

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"slices"
	"strconv"
	"strings"

	questioncontract "github.com/Tangerg/flame/packages/go-sdk/question"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"
	"github.com/Tangerg/qimu/integrity"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// InterruptFunc is the consumer-owned capability a tool uses to park the
// current execution on one application interrupt. On re-entry (Scope carries
// the answer on the invocation) it returns the resolved decision; on a first
// request it returns a Scope InputRequest the tool returns as
// tool.Result{InputRequest}. Tool packages depend only on this contract and
// never on execution internals.
type InterruptFunc func(context.Context, toolcontract.Invocation, Interrupt) (run.InterruptResolution, *toolcontract.InputRequest, error)

// InterruptUnavailable is the fail-closed default for a tool environment that
// has no execution interrupt provider.
func InterruptUnavailable(context.Context, toolcontract.Invocation, Interrupt) (run.InterruptResolution, *toolcontract.InputRequest, error) {
	return run.InterruptResolution{}, nil, errors.New("runs: execution interrupts are unavailable")
}

// ApprovalPrompt is the complete durable plan for one gated tool call.
// Arguments are the effective arguments after PreToolUse rewriting, so a
// continuation (including one restored after restart) can resume without
// running the hook or policy decision a second time. Tool names the gated call;
// its model-visible name is Tool.ModelName.
type ApprovalPrompt struct {
	Tool              tool.Ref
	SourceFingerprint integrity.Digest
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

// Interrupt is the durable product request for external input: an
// ApprovalPrompt or a QuestionPrompt, and the prompt alone decides its Kind.
// Executor continuation data is deliberately absent.
type Interrupt interface {
	Type() run.InterruptType
	// ToolCall returns the logical tool call that owns this interrupt.
	ToolCall() (name, arguments string)
	// Validate rejects a malformed prompt before it becomes a durable Pending
	// aggregate or application event.
	Validate() error
	clone() Interrupt
}

// InterruptKey is the stable identity prompt is answered under. The kind is
// part of it so an approval and a question over the same call never collide;
// the hash keeps serialized keys compact for transport and durable stores.
func InterruptKey(prompt Interrupt) string {
	name, arguments := prompt.ToolCall()
	h := fnv.New64a()
	_, _ = h.Write([]byte(prompt.Type()))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(name))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(arguments))
	return string(prompt.Type()) + "." + strconv.FormatUint(h.Sum64(), 16)
}

func validateInterrupt(interrupt Interrupt) error {
	if interrupt == nil {
		return errors.New("runs: interrupt has no prompt")
	}
	return interrupt.Validate()
}

func (ApprovalPrompt) Type() run.InterruptType { return run.InterruptApproval }
func (QuestionPrompt) Type() run.InterruptType { return run.InterruptQuestion }

func (a ApprovalPrompt) ToolCall() (name, arguments string) { return a.Tool.ModelName(), a.Arguments }
func (q QuestionPrompt) ToolCall() (name, arguments string) { return q.ToolName, q.Arguments }

func (a ApprovalPrompt) clone() Interrupt { return a }

func (q QuestionPrompt) clone() Interrupt {
	q.Fields = slices.Clone(q.Fields)
	for index := range q.Fields {
		q.Fields[index].Options = slices.Clone(q.Fields[index].Options)
	}
	return q
}

func (a ApprovalPrompt) Validate() error {
	if err := a.Tool.ValidateFingerprint(a.SourceFingerprint); err != nil {
		return err
	}
	if err := ValidateInvocation(a.CallID); err != nil {
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

func (q QuestionPrompt) Validate() error {
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
			Kind:   questioncontract.Text,
		}
		if len(spec.Options) > 0 {
			field.Kind = questioncontract.Choice
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
