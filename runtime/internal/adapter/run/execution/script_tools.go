package execution

import (
	"context"
	"crypto/sha256"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"maps"
	"slices"

	builtincontract "github.com/Tangerg/flame/packages/go-sdk/builtin"
	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/operation"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	"github.com/Tangerg/flame/runtime/internal/infra/script"

	corechat "github.com/Tangerg/scope/core/chat"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// scriptTools binds one run_script call to the read-only Tools it declared. A
// script call is admitted and executed exactly as a direct call of that Tool,
// except that it cannot pause for input. A call that would ask for approval
// runs only when the user approved the script and no hook asks for this call.
type scriptTools struct {
	caller   *observedInteractionTool
	callID   runs.InvocationID
	member   runs.ExecutorMember
	byName   map[string]*observedInteractionTool
	approved bool
}

func (o *observedInteractionTool) scriptTools(callID runs.InvocationID, member runs.ExecutorMember, arguments tool.Arguments, approved bool) (scriptTools, error) {
	names, err := toolset.ScriptToolNames(arguments)
	if err != nil {
		return scriptTools{}, err
	}
	readOnly := o.readOnlyScriptTools()
	byName := make(map[string]*observedInteractionTool, len(names))
	for _, name := range names {
		if target, found := readOnly[name]; found {
			byName[name] = target
		}
	}
	return scriptTools{caller: o, callID: callID, member: member, byName: byName, approved: approved}, nil
}

func (o *observedInteractionTool) readOnlyScriptTools() map[string]*observedInteractionTool {
	readOnly := make(map[string]*observedInteractionTool)
	for ref, candidate := range o.scriptCatalog {
		definition := candidate.Definition()
		if ref != o.ref && definition.Effect == toolcontract.EffectSafe {
			readOnly[definition.Name] = candidate
		}
	}
	return readOnly
}

var riskOrder = []tool.RiskLevel{tool.RiskLow, tool.RiskMedium, tool.RiskHigh}

// declaredApproval returns the strictest safety class of the Tools a run_script
// call declares whose own calls would ask, or none when no declared Tool would.
func (o *observedInteractionTool) declaredApproval(ctx context.Context, arguments tool.Arguments) (tool.SafetyClass, error) {
	names, err := toolset.ScriptToolNames(arguments)
	if err != nil {
		return "", err
	}
	readOnly := o.readOnlyScriptTools()
	var strictest tool.SafetyClass
	for _, name := range names {
		target, found := readOnly[name]
		if !found {
			return "", fmt.Errorf("%w: %q is not a read-only tool a script can call", tool.ErrInvalidArguments, name)
		}
		class := o.interpreter.SafetyClass(target.ref)
		asks, err := o.authorizer.AsksForTool(ctx, ToolApprovalScope{
			SessionID: o.start.SessionID, WorkspaceCWD: o.start.WorkspaceCWD,
			Tool: target.ref, SourceFingerprint: target.sourceFingerprint, SafetyClass: class,
		})
		if err != nil {
			return "", err
		}
		if asks && (strictest == "" || slices.Index(riskOrder, class.Risk()) > slices.Index(riskOrder, strictest.Risk())) {
			strictest = class
		}
	}
	return strictest, nil
}

func (s scriptTools) Names() []string { return slices.Sorted(maps.Keys(s.byName)) }

type scriptToolDescription struct {
	Name         string         `json:"name"`
	Description  string         `json:"description,omitempty"`
	InputSchema  jsontext.Value `json:"inputSchema"`
	OutputSchema jsontext.Value `json:"outputSchema,omitzero"`
	Source       string         `json:"source"`
}

func (s scriptTools) Describe(name string) (jsontext.Value, bool) {
	target, found := s.byName[name]
	if !found {
		return nil, false
	}
	definition := target.Definition()
	encoded, err := json.Marshal(scriptToolDescription{
		Name: definition.Name, Description: definition.Description,
		InputSchema: jsontext.Value(definition.InputSchema), OutputSchema: jsontext.Value(definition.OutputSchema),
		Source: target.ref.String(),
	})
	return encoded, err == nil
}

func (s scriptTools) Call(ctx context.Context, index int, call script.Call) (toolset.ScriptToolCall, error) {
	target, found := s.byName[call.Tool]
	if !found {
		return toolset.ScriptToolCall{Settlement: script.Settlement{Status: builtincontract.ScriptCallRejected,
			Message: fmt.Sprintf("%q is not a read-only tool this script declared", call.Tool)}}, nil
	}
	settled := toolset.ScriptToolCall{Source: target.ref}
	arguments, err := tool.ParseArguments(call.Arguments)
	if err != nil {
		settled.Settlement = script.Settlement{Status: builtincontract.ScriptCallFailed, Message: "invalid arguments: " + run.Diagnostic(err)}
		return settled, nil
	}
	id, err := scriptCallID(s.callID, index)
	if err != nil {
		return toolset.ScriptToolCall{}, err
	}
	ctx = operation.Adopt(ctx, id.String(), extension.Model, s.member.MemberID)
	start := s.caller.start
	request, decision, err := (toolAdmission{
		binding: target.binding, executable: target.inner, ref: target.ref, fingerprint: target.sourceFingerprint,
		interpreter: s.caller.interpreter, authorizer: s.caller.authorizer, hooks: s.caller.hooks,
		input: InteractionToolHookInput{SessionID: start.SessionID, CWD: start.CWD, WorkspaceCWD: start.WorkspaceCWD, ToolName: call.Tool, Arguments: arguments},
	}).prepare(ctx, id.String())
	if errors.Is(err, tool.ErrInvalidArguments) {
		settled.Settlement = script.Settlement{Status: builtincontract.ScriptCallFailed, Message: "invalid effective arguments: " + run.Diagnostic(err)}
		return settled, nil
	}
	if err != nil {
		return toolset.ScriptToolCall{}, err
	}
	arguments = request.Arguments
	if rewritten, ok := decision.EffectiveArguments(); ok {
		arguments = rewritten
	}
	settled.Arguments = &arguments
	if reason, denied := decision.Denied(); denied {
		settled.Settlement = script.Settlement{Status: builtincontract.ScriptCallRejected, Message: reason}
		return settled, nil
	}
	if _, ask := decision.Approval(); ask && (request.RequireApproval || !s.approved) {
		message := fmt.Sprintf("%s needs the user's approval, which this script did not receive; call it directly instead", call.Tool)
		if request.RequireApproval {
			message = fmt.Sprintf("a hook asks the user to approve this %s call; call it directly instead", call.Tool)
		}
		settled.Settlement = script.Settlement{Status: builtincontract.ScriptCallRejected, Message: message}
		return settled, nil
	}
	result, callErr := target.invoke(ctx, corechat.ToolCall{ID: id.String(), Name: call.Tool, Arguments: arguments.Canonical()}, arguments, toolcontract.Admission{})
	if callErr == nil && result.InputRequest != nil {
		settled.Settlement = script.Settlement{Status: builtincontract.ScriptCallRejected,
			Message: fmt.Sprintf("%s asks for input, which a script cannot give", call.Tool)}
		return settled, nil
	}
	target.runAfterToolUseHook(ctx, id.String(), call.Tool, arguments, result.Output, callErr)
	settled.Settlement, err = scriptSettlement(result.Output, callErr)
	return settled, err
}

func scriptSettlement(output toolcontract.Output, callErr error) (script.Settlement, error) {
	if callErr != nil {
		status := builtincontract.ScriptCallFailed
		message := run.Diagnostic(callErr)
		if failure, known := toolcontract.ClassifyFailure(callErr); known {
			if failure.Kind() == toolcontract.FailureKindRejected {
				status = builtincontract.ScriptCallRejected
			}
			if text, textual := failure.Output().Text(); textual && text != "" {
				message = text
			}
		}
		return script.Settlement{Status: status, Message: message}, nil
	}
	var value jsontext.Value
	var err error
	switch text, textual := output.Text(); {
	case len(output.Details) > 0:
		value = jsontext.Value(output.Details)
	case textual:
		value, err = json.Marshal(text)
	default:
		value, err = json.Marshal(output.Content)
	}
	if err != nil {
		return script.Settlement{}, fmt.Errorf("execution: encode script call result: %w", err)
	}
	return script.Settlement{Status: builtincontract.ScriptCallSucceeded, Value: value}, nil
}

func scriptCallID(caller runs.InvocationID, index int) (runs.InvocationID, error) {
	digest := sha256.Sum256([]byte(caller.String()))
	return parsedInvocationID(scriptCallNamespace, digest[:], uint64(index))
}
