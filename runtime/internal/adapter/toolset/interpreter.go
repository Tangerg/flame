package toolset

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/identity"

	builtincontract "github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/domain/session/plan"
	"github.com/samber/lo"
)

const shellCommandField = "command"

type Interpreter struct {
	plans planStateReader
}

type planStateReader interface {
	State(ctx context.Context, sessionID identity.SessionID) (plan.Current, error)
}

func NewInterpreter(plans planStateReader) (Interpreter, error) {
	if lo.IsNil(plans) {
		return Interpreter{}, errors.New("toolset: Plan state reader is required")
	}
	return Interpreter{plans: plans}, nil
}

// Unknown tools fail closed: an extension can execute arbitrary work even when its
// name suggests a read.
func (Interpreter) SafetyClass(ref tool.Ref) tool.SafetyClass {
	if descriptor, ok := descriptorFor(ref); ok {
		return descriptor.safety
	}
	return tool.SafetyClassExec
}

func (Interpreter) UsesStandardPolicy(ref tool.Ref) bool {
	descriptor, ok := descriptorFor(ref)
	return !ok || !descriptor.orchestration
}

func (Interpreter) ApprovalSubject(ref tool.Ref, arguments tool.Arguments) (string, error) {
	var field string
	name, ok := ref.BuiltIn()
	if !ok {
		return "", nil
	}
	switch name {
	case builtincontract.Shell:
		field = shellCommandField
	case builtincontract.Read, builtincontract.Edit:
		field = "path"
	default:
		return "", nil
	}
	subject, ok := arguments.StringField(field)
	if !ok || strings.TrimSpace(subject) == "" {
		return "", fmt.Errorf("toolset: tool %q requires non-empty string argument %q: %w", ref, field, tool.ErrInvalidArguments)
	}
	return subject, nil
}

func (Interpreter) ShellCommand(ref tool.Ref, rawArguments string) string {
	if !ref.IsBuiltIn(builtincontract.Shell) {
		return ""
	}
	arguments, err := tool.ParseArguments(rawArguments)
	if err != nil {
		return ""
	}
	command, _ := arguments.StringField(shellCommandField)
	return command
}

func (i Interpreter) ProjectOutcome(
	ctx context.Context,
	sessionID identity.SessionID, ref tool.Ref,
	succeeded bool,
) (runs.ExecutionFact, error) {
	descriptor, known := descriptorFor(ref)
	if !succeeded || !known || descriptor.outcome != planOutcomeProjection {
		return nil, nil
	}
	current, err := i.plans.State(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("toolset: project Plan replacement: %w", err)
	}
	state, committed := current.State()
	if !committed {
		return nil, errors.New("toolset: successful Plan replacement left no committed state")
	}
	return runs.PlanUpdated{State: state}, nil
}
