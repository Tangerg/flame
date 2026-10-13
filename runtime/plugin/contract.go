package plugin

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/jsoninput"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/jsonschema"
)

type MethodConfig struct {
	Name        string
	Kind        capability.MethodKind
	Target      string
	Description string
}

type Method[P, R any] struct {
	contract *capability.Method
}

func NewMethod[P, R any](config MethodConfig) (*Method[P, R], error) {
	if config.Description == "" || !utf8.ValidString(config.Description) {
		return nil, errors.New("plugin: method description is required")
	}
	input, err := jsonschema.For[P]()
	if err != nil {
		return nil, fmt.Errorf("plugin: %s input: %w", config.Name, err)
	}
	output, err := jsonschema.For[R]()
	if err != nil {
		return nil, fmt.Errorf("plugin: %s output: %w", config.Name, err)
	}
	contract, err := capability.AdmitMethod(capability.MethodDescriptor{Name: config.Name, Kind: config.Kind, Target: config.Target, Description: config.Description, Input: input.JSON(), Output: output.JSON()})
	if err != nil {
		return nil, err
	}
	return &Method[P, R]{contract: contract}, nil
}

func (m *Method[P, R]) Name() string                            { return m.contract.Name() }
func (m *Method[P, R]) Descriptor() capability.MethodDescriptor { return m.contract.Descriptor() }

// Raw input admission precedes typed business execution. Re-encoding the decoded
// value would erase missing required members by inserting their Go zero values.
func (m *Method[P, R]) RegisterServer(backend *Backend, classify func(error) string, handler func(context.Context, *sdk.ServerSession, Command[P]) (R, error)) error {
	if backend == nil || handler == nil {
		return errors.New("plugin: backend and method handler are required")
	}
	return backend.register(m.bind(classify, handler))
}

func (m *Method[P, R]) bind(classify func(error) string, handler func(context.Context, *sdk.ServerSession, Command[P]) (R, error)) boundMethod {
	return boundMethod{contract: m.contract, invoke: func(ctx context.Context, session *sdk.ServerSession, wire *invokeParams) (*Result[jsontext.Value], error) {
		ctx, command, err := admitCommand[P](ctx, m.contract, wire)
		if err != nil {
			return nil, err
		}
		value, operationErr := handler(ctx, session, command)
		if operationErr == nil {
			encoded, err := json.Marshal(value)
			if err != nil {
				return nil, err
			}
			if err := m.contract.ValidateOutput(encoded); err != nil {
				return nil, fmt.Errorf("plugin: %s output: %w", m.Name(), err)
			}
		}
		outcome, err := Reply(value, operationErr, classify)
		if err != nil {
			return nil, err
		}
		data, err := json.Marshal(outcome)
		if err != nil {
			return nil, err
		}
		var result Result[jsontext.Value]
		if err := json.Unmarshal(data, &result, json.RejectUnknownMembers(true)); err != nil {
			return nil, err
		}
		return &result, nil
	}}
}

func (m *Method[P, R]) Call(ctx context.Context, session *sdk.ClientSession, input P) (R, error) {
	var zero R
	if m.contract.Kind() != capability.Query {
		return zero, errors.New("plugin: command requires a logical identity")
	}
	return m.call(ctx, session, &Params[P]{Input: input})
}

func (m *Method[P, R]) CallCommand(ctx context.Context, session *sdk.ClientSession, id string, input P) (R, error) {
	var zero R
	if m.contract.Kind() != capability.Mutation {
		return zero, errors.New("plugin: query must not carry a command identity")
	}
	if err := validateCommandID(id); err != nil {
		return zero, err
	}
	return m.call(ctx, session, &Params[P]{Input: input, CommandID: id})
}

func (m *Method[P, R]) call(ctx context.Context, session *sdk.ClientSession, params *Params[P]) (R, error) {
	var zero R
	if session == nil {
		return zero, errors.New("plugin: client session is required")
	}
	binding := &Binding{session: session, method: m.contract}
	return m.callBinding(ctx, binding, params)
}

func (m *Method[P, R]) callBinding(ctx context.Context, binding *Binding, params *Params[P]) (R, error) {
	var zero R
	input, err := jsoninput.Encode(params.Input)
	if err != nil {
		return zero, err
	}
	wire, err := binding.invoke(ctx, params.CommandID, input, params.ParamsBase)
	if err != nil {
		return zero, err
	}
	result, err := decodeBoundResult[R](wire)
	if err != nil {
		return zero, err
	}
	return result.value(m.Name())
}

type BoundMethod[P, R any] struct {
	method  *Method[P, R]
	binding *Binding
}

func (m *Method[P, R]) Bind(capability *CapabilityBinding) (*BoundMethod[P, R], error) {
	if m == nil {
		return nil, errors.New("plugin: method is required")
	}
	binding, err := capability.Method(m.Name())
	if err != nil {
		return nil, err
	}
	if m.contract.Identity() != binding.Contract() {
		return nil, errors.New("plugin: typed method differs from capability contract")
	}
	return &BoundMethod[P, R]{method: m, binding: binding}, nil
}

func (m *BoundMethod[P, R]) Call(ctx context.Context, input P) (R, error) {
	return m.CallWithMetadata(ctx, input, sdk.ParamsBase{})
}

func (m *BoundMethod[P, R]) CallWithMetadata(ctx context.Context, input P, metadata sdk.ParamsBase) (R, error) {
	var zero R
	if m.method.contract.Kind() != capability.Query {
		return zero, errors.New("plugin: command requires a logical identity")
	}
	return m.method.callBinding(ctx, m.binding, &Params[P]{Input: input, ParamsBase: metadata})
}

func (m *BoundMethod[P, R]) CallCommand(ctx context.Context, id string, input P) (R, error) {
	var zero R
	if m.method.contract.Kind() != capability.Mutation {
		return zero, errors.New("plugin: query must not carry a command identity")
	}
	if err := validateCommandID(id); err != nil {
		return zero, err
	}
	return m.method.callBinding(ctx, m.binding, &Params[P]{Input: input, CommandID: id})
}

func (m *BoundMethod[P, R]) ReadReceipt(ctx context.Context, id string, input P) (R, bool, error) {
	var zero R
	admitted, err := jsoninput.Encode(input)
	if err != nil {
		return zero, false, err
	}
	value, found, err := m.binding.ReadReceipt(ctx, id, admitted)
	if err != nil || !found {
		return zero, found, err
	}
	var result R
	if err := json.Unmarshal(value.JSON(), &result, json.RejectUnknownMembers(true)); err != nil {
		return zero, false, err
	}
	return result, true, nil
}

func admitCommand[P any](ctx context.Context, contract *capability.Method, wire *invokeParams) (context.Context, Command[P], error) {
	encoded, err := json.Marshal(struct {
		sdk.ParamsBase
		Input     jsontext.Value `json:"input"`
		CommandID string         `json:"commandId,omitzero"`
	}{wire.ParamsBase, wire.Input, wire.CommandID})
	if err != nil {
		return ctx, Command[P]{}, err
	}
	var params Params[P]
	if err := json.Unmarshal(encoded, &params, json.RejectUnknownMembers(true)); err != nil {
		return ctx, Command[P]{}, err
	}
	if params.inputJSON.Validate() != nil {
		return ctx, Command[P]{}, errors.New("plugin: method input is required")
	}
	if contract.Kind() == capability.Query && params.CommandID != "" {
		return ctx, Command[P]{}, errors.New("plugin: query must not carry a command identity")
	}
	if contract.Kind() == capability.Mutation {
		if err := validateCommandID(params.CommandID); err != nil {
			return ctx, Command[P]{}, err
		}
	}
	if err := contract.ValidateInput(params.inputJSON); err != nil {
		return ctx, Command[P]{}, fmt.Errorf("plugin: %s input: %w", contract.Name(), err)
	}
	ctx = context.WithValue(ctx, requestInputKey{}, params.inputJSON)
	ctx, err = bindInvocationMetadata(ctx, params.Meta, contract)
	if err != nil {
		return ctx, Command[P]{}, err
	}
	ctx, err = bindCallbackMetadata(ctx, params.Meta)
	if err != nil {
		return ctx, Command[P]{}, err
	}
	return ctx, Command[P]{ID: params.CommandID, Input: params.Input, inputJSON: params.inputJSON}, nil
}
