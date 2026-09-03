package runtimebinding

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/cli/internal/domain/workspace"
	flameruntime "github.com/Tangerg/flame/runtime"
	"github.com/Tangerg/flame/runtime/protocol"
)

type diagnosticToolBinding interface {
	ListTools(context.Context, flameruntime.CallOptions) (*protocol.Page[protocol.ToolSpec], error)
	InvokeTool(context.Context, protocol.InvokeToolRequest, flameruntime.CommandOptions) (any, error)
}

type DiagnosticTools struct{ runtime *Connection }

func (d *DiagnosticTools) Tools(ctx context.Context) ([]workspace.DiagnosticToolDescriptor, error) {
	r := d.runtime
	page, err := r.diagnosticTools.ListTools(ctx, r.callOptions())
	if err != nil {
		return nil, classifyError(err)
	}
	values, err := requireCompletePage("list diagnostic tools", page)
	if err != nil {
		return nil, err
	}
	tools := make([]workspace.DiagnosticToolDescriptor, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for index, value := range values {
		if value.SafetyClass != protocol.SafetyClassSafe {
			return nil, runtimeContractViolation(
				"list diagnostic tools item %d is not safe for direct invocation: %q",
				index+1,
				value.SafetyClass,
			)
		}
		schema, marshalErr := json.Marshal(value.Parameters)
		if marshalErr != nil {
			return nil, runtimeContractViolation("list diagnostic tools item %d has an invalid schema: %v", index+1, marshalErr)
		}
		descriptor := workspace.DiagnosticToolDescriptor{
			Name: value.Name, Description: value.Description,
			Schema: schema,
		}
		if err := descriptor.Validate(); err != nil {
			return nil, runtimeContractViolation("list diagnostic tools item %d is invalid: %v", index+1, err)
		}
		if _, duplicate := seen[descriptor.Name]; duplicate {
			return nil, runtimeContractViolation("list diagnostic tools repeats %q", descriptor.Name)
		}
		seen[descriptor.Name] = struct{}{}
		if len(tools) > 0 && descriptor.Name < tools[len(tools)-1].Name {
			return nil, runtimeContractViolation(
				"list diagnostic tools item %d is out of name order: %q follows %q",
				index+1,
				descriptor.Name,
				tools[len(tools)-1].Name,
			)
		}
		tools = append(tools, descriptor)
	}
	return tools, nil
}

func (d *DiagnosticTools) Invoke(ctx context.Context, invocation workspace.DiagnosticToolInvocation) (workspace.DiagnosticToolResult, error) {
	r := d.runtime
	if err := invocation.Validate(); err != nil {
		return workspace.DiagnosticToolResult{}, err
	}
	var arguments map[string]any
	if err := json.Unmarshal(invocation.Arguments, &arguments); err != nil {
		return workspace.DiagnosticToolResult{}, fmt.Errorf("decode diagnostic tool arguments: %w", err)
	}
	options, err := r.commandOptions()
	if err != nil {
		return workspace.DiagnosticToolResult{}, err
	}
	value, err := r.diagnosticTools.InvokeTool(ctx, protocol.InvokeToolRequest{
		Name: strings.TrimSpace(invocation.Tool.Name), Arguments: arguments,
		Workspace: &protocol.WorkspaceRef{Path: strings.TrimSpace(invocation.Workspace)},
	}, options)
	if err != nil {
		return workspace.DiagnosticToolResult{}, classifyError(err)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return workspace.DiagnosticToolResult{}, runtimeContractViolation("diagnostic tool result cannot be encoded: %v", err)
	}
	result := workspace.DiagnosticToolResult{JSON: encoded}
	if err := result.Validate(); err != nil {
		return workspace.DiagnosticToolResult{}, runtimeContractViolation("diagnostic tool returned an invalid result: %v", err)
	}
	return result, nil
}
