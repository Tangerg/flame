package plugin

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"unicode/utf8"

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
	if config.Name == "" || len(config.Name) > 256 || !utf8.ValidString(config.Name) || config.Target == "" || len(config.Target) > 256 || !utf8.ValidString(config.Target) || config.Description == "" || !utf8.ValidString(config.Description) {
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

func (m *Method[P, R]) RegisterClient(client *sdk.Client) error {
	return RegisterClient[P, R](client, m.config.Name)
}

// Raw input admission precedes typed business execution. Re-encoding the decoded
// value would erase missing required members by inserting their Go zero values.
func (m *Method[P, R]) RegisterServer(server *sdk.Server, classify func(error) string, handler func(context.Context, *sdk.ServerSession, Command[P]) (R, error)) error {
	if server == nil || handler == nil {
		return errors.New("plugin: server and method handler are required")
	}
	return sdk.AddReceivingCustomMethod(server, m.config.Name, func(ctx context.Context, session *sdk.ServerSession, params *Params[P]) (*Result[R], error) {
		if params == nil {
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
		return Reply(value, operationErr, classify)
	})
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
	encoded, err := json.Marshal(params.Input)
	if err != nil {
		return zero, err
	}
	if err := m.input.Validate(encoded); err != nil {
		return zero, fmt.Errorf("plugin: %s input: %w", m.config.Name, err)
	}
	result, err := sdk.CallCustomMethod[*Params[P], *Result[R]](ctx, session, m.config.Name, params)
	if err != nil {
		return zero, err
	}
	value, err := result.value(m.config.Name)
	if err != nil {
		return zero, err
	}
	if err := m.output.Validate(result.valueJSON); err != nil {
		return zero, fmt.Errorf("plugin: %s output: %w", m.config.Name, err)
	}
	return value, nil
}
