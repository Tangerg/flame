package builtin

import (
	"bytes"
	"context"
	jsonv2 "encoding/json/v2"
	"fmt"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
	"github.com/Tangerg/scope/core/chat"
	corejsonschema "github.com/Tangerg/scope/core/jsonschema"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

// interruptTool projects a typed model call into a product Interrupt, parks on
// Scope's input contract, and renders the resolved answer. Scope owns the
// suspension, wait identity, and re-entry; the tool owns only the product
// projection. plan builds the request and resolve renders the answer, so a
// tool adds just those two product halves.
type interruptTool[Args any] struct {
	definition chat.ToolDefinition
	interrupt  runs.InterruptFunc
	plan       func(context.Context, Args) (runs.Interrupt, string, error)
	resolve    func(context.Context, Args, interrupt.Resolution) (string, error)
}

func newInterruptTool[Args any](
	config toolcontract.FuncConfig,
	interrupt runs.InterruptFunc,
	plan func(context.Context, Args) (runs.Interrupt, string, error),
	resolve func(context.Context, Args, interrupt.Resolution) (string, error),
) (toolcontract.Tool, error) {
	schema, err := corejsonschema.For[Args]()
	if err != nil {
		return nil, fmt.Errorf("%s: derive input schema: %w", config.Name, err)
	}
	definition := chat.ToolDefinition{Name: config.Name, Description: config.Description, InputSchema: schema.JSON()}
	if err := definition.Validate(); err != nil {
		return nil, fmt.Errorf("%s: invalid definition: %w", config.Name, err)
	}
	if interrupt == nil {
		interrupt = runs.InterruptUnavailable
	}
	return &interruptTool[Args]{definition: definition, interrupt: interrupt, plan: plan, resolve: resolve}, nil
}

func (t *interruptTool[Args]) Definition() chat.ToolDefinition {
	return chat.ToolDefinition{
		Name:        t.definition.Name,
		Description: t.definition.Description,
		InputSchema: bytes.Clone(t.definition.InputSchema),
	}
}

// InputValidator admits arguments with the same strict decoder as Call, so Bind
// rejects unknown members before the tool runs, matching tool.NewFunc.
func (t *interruptTool[Args]) InputValidator() func([]byte) error {
	return func(arguments []byte) error {
		var args Args
		return jsonv2.Unmarshal(arguments, &args, jsonv2.RejectUnknownMembers(true))
	}
}

func (t *interruptTool[Args]) Call(ctx context.Context, invocation toolcontract.Invocation) (toolcontract.Result, error) {
	var args Args
	if err := jsonv2.Unmarshal(invocation.Arguments(), &args, jsonv2.RejectUnknownMembers(true)); err != nil {
		return toolcontract.Result{}, fmt.Errorf("%s: decode arguments: %w", t.definition.Name, err)
	}
	prompt, key, err := t.plan(ctx, args)
	if err != nil {
		return toolcontract.Result{}, err
	}
	resolution, request, err := t.interrupt(ctx, invocation, key, prompt)
	if err != nil {
		return toolcontract.Result{}, err
	}
	if request != nil {
		return toolcontract.Result{InputRequest: request}, nil
	}
	text, err := t.resolve(ctx, args, resolution)
	if err != nil {
		return toolcontract.Result{}, err
	}
	output := chat.NewTextToolOutput(text)
	if err := output.Validate(); err != nil {
		return toolcontract.Result{}, err
	}
	return toolcontract.Result{Output: output}, nil
}
