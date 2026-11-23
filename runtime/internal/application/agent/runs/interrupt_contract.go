package runs

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"slices"
	"strconv"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/run/transcript"

	framevalue "github.com/Tangerg/frame/value"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

type InterruptFunc func(context.Context, toolcontract.Invocation, Interrupt) (run.InterruptResolution, *toolcontract.InputRequest, error)

type ApprovalPrompt struct {
	Tool              tool.Ref
	QualifiedName     bool
	SourceFingerprint framevalue.Digest
	CallID            string
	Arguments         string
	SafetyClass       tool.SafetyClass
	Risk              tool.RiskLevel
	Reason            string
	Rememberable      bool
}

type QuestionPrompt struct {
	ToolName  string
	Arguments string
	Fields    []transcript.QuestionField
}

type Interrupt interface {
	Type() run.InterruptType
	ToolCall() (name, arguments string)
	Validate() error
	clone() Interrupt
}

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

func (a ApprovalPrompt) ToolCall() (name, arguments string) {
	return a.Tool.PublishedName(a.QualifiedName), a.Arguments
}

func (q QuestionPrompt) ToolCall() (name, arguments string) { return q.ToolName, q.Arguments }

func (a ApprovalPrompt) clone() Interrupt { return a }

func (q QuestionPrompt) clone() Interrupt {
	q.Fields = q.question().Fields
	return q
}

func (a ApprovalPrompt) Validate() error {
	if a.QualifiedName && a.Tool.Kind() != tool.MCPKind {
		return errors.New("runs: only a remote tool has a qualified name")
	}
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
	fields := slices.Clone(q.Fields)
	for index := range fields {
		fields[index].Options = slices.Clone(fields[index].Options)
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
