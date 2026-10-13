package plugin

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/jsoninput"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/jsonschema"
)

type MethodKind string

const (
	Query    MethodKind = "query"
	Mutation MethodKind = "command"
)

type MethodConfig struct {
	Name        string
	Kind        MethodKind
	Target      string
	Description string
}

// MethodDescriptor is a build/discovery projection. It cannot change the
// immutable method that generated it or supply a different admission schema.
type MethodDescriptor struct {
	Name        string         `json:"name"`
	Kind        MethodKind     `json:"kind"`
	Target      string         `json:"target"`
	Description string         `json:"description"`
	Input       jsontext.Value `json:"input"`
	Output      jsontext.Value `json:"output"`
}

type Method[P, R any] struct {
	config MethodConfig
	input  jsonschema.Schema
	output jsonschema.Schema
}

func NewMethod[P, R any](config MethodConfig) (*Method[P, R], error) {
	if config.Kind != Query && config.Kind != Mutation {
		return nil, errors.New("plugin: method kind must be query or command")
	}
	if !contractToken(config.Name) || !contractToken(config.Target) || config.Description == "" || !utf8.ValidString(config.Description) {
		return nil, errors.New("plugin: method identity, target and description are required")
	}
	input, err := jsonschema.For[P]()
	if err != nil {
		return nil, fmt.Errorf("plugin: %s input: %w", config.Name, err)
	}
	if input.Validate([]byte("null")) == nil {
		return nil, errors.New("plugin: method input must exclude null; wrap optional values in a request object")
	}
	output, err := jsonschema.For[R]()
	if err != nil {
		return nil, fmt.Errorf("plugin: %s output: %w", config.Name, err)
	}
	return &Method[P, R]{config: config, input: input, output: output}, nil
}

func (m *Method[P, R]) Name() string { return m.config.Name }

func (m *Method[P, R]) Descriptor() MethodDescriptor {
	return MethodDescriptor{Name: m.config.Name, Kind: m.config.Kind, Target: m.config.Target, Description: m.config.Description, Input: m.input.JSON(), Output: m.output.JSON()}
}

// Raw input admission precedes typed business execution. Re-encoding the decoded
// value would erase missing required members by inserting their Go zero values.
func (m *Method[P, R]) RegisterServer(backend *Backend, classify func(error) string, handler func(context.Context, *sdk.ServerSession, Command[P]) (R, error)) error {
	if backend == nil || handler == nil {
		return errors.New("plugin: backend and method handler are required")
	}
	return backend.register(m.bind(classify, handler))
}

func (m *Method[P, R]) bind(classify func(error) string, handler func(context.Context, *sdk.ServerSession, Command[P]) (R, error)) boundMethod {
	return boundMethod{descriptor: m.Descriptor(), invoke: func(ctx context.Context, session *sdk.ServerSession, wire *invokeParams) (*Result[jsontext.Value], error) {
		encoded, err := json.Marshal(struct {
			sdk.ParamsBase
			Input     jsontext.Value `json:"input"`
			CommandID string         `json:"commandId,omitzero"`
		}{wire.ParamsBase, wire.Input, wire.CommandID})
		if err != nil {
			return nil, err
		}
		var params Params[P]
		if err := json.Unmarshal(encoded, &params, json.RejectUnknownMembers(true)); err != nil {
			return nil, err
		}
		if params.inputJSON.Validate() != nil {
			return nil, errors.New("plugin: method input is required")
		}
		if m.config.Kind == Query && params.CommandID != "" {
			return nil, errors.New("plugin: query must not carry a command identity")
		}
		if m.config.Kind == Mutation {
			if err := validateCommandID(params.CommandID); err != nil {
				return nil, err
			}
		}
		if err := m.input.Validate(params.inputJSON.JSON()); err != nil {
			return nil, fmt.Errorf("plugin: %s input: %w", m.config.Name, err)
		}
		ctx = context.WithValue(ctx, requestInputKey{}, params.inputJSON)
		ctx, err = bindCallbackMetadata(ctx, params.Meta)
		if err != nil {
			return nil, err
		}
		value, operationErr := handler(ctx, session, Command[P]{ID: params.CommandID, Input: params.Input, inputJSON: params.inputJSON})
		if operationErr == nil {
			encoded, err := json.Marshal(value)
			if err != nil {
				return nil, err
			}
			if err := m.output.Validate(encoded); err != nil {
				return nil, fmt.Errorf("plugin: %s output: %w", m.config.Name, err)
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
	if m.config.Kind != Query {
		return zero, errors.New("plugin: command requires a logical identity")
	}
	return m.call(ctx, session, &Params[P]{Input: input})
}

func (m *Method[P, R]) CallCommand(ctx context.Context, session *sdk.ClientSession, id string, input P) (R, error) {
	var zero R
	if m.config.Kind != Mutation {
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
	binding, err := NewBinding(session, m.Descriptor())
	if err != nil {
		return zero, err
	}
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
	return result.value(m.config.Name)
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
	contract, err := methodContract(m.Descriptor())
	if err != nil {
		return nil, err
	}
	if contract != binding.contract {
		return nil, errors.New("plugin: typed method differs from capability contract")
	}
	return &BoundMethod[P, R]{method: m, binding: binding}, nil
}

func (m *BoundMethod[P, R]) Call(ctx context.Context, input P) (R, error) {
	var zero R
	if m.method.config.Kind != Query {
		return zero, errors.New("plugin: command requires a logical identity")
	}
	return m.method.callBinding(ctx, m.binding, &Params[P]{Input: input})
}

func (m *BoundMethod[P, R]) CallCommand(ctx context.Context, id string, input P) (R, error) {
	var zero R
	if m.method.config.Kind != Mutation {
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
