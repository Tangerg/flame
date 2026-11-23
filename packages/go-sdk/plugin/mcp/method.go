package mcp

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"reflect"
	"unicode/utf8"

	"github.com/Tangerg/frame"
	framecall "github.com/Tangerg/frame/call"
	framevalue "github.com/Tangerg/frame/value"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/jsonschema"
	"github.com/google/uuid"
)

type methodParams[T any] struct {
	sdk.ParamsBase
	Input     T         `json:"input"`
	CommandID CommandID `json:"commandId,omitempty"`
	inputJSON framevalue.JSON
}

type Command[T any] struct {
	ID        CommandID
	Input     T
	inputJSON framevalue.JSON
}

func (c Command[T]) RequestInput() (framevalue.JSON, error) {
	return c.inputJSON, c.inputJSON.Validate()
}

type requestInputKey struct{}

func RequestInput(ctx context.Context) (framevalue.JSON, error) {
	value, _ := ctx.Value(requestInputKey{}).(framevalue.JSON)
	return value, value.Validate()
}

func (p methodParams[T]) MarshalJSON() ([]byte, error) {
	input := p.inputJSON
	if input.Validate() != nil {
		var err error
		input, err = framevalue.EncodeJSON(p.Input)
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(struct {
		sdk.ParamsBase
		Input     framevalue.JSON `json:"input"`
		CommandID CommandID       `json:"commandId,omitzero"`
	}{p.ParamsBase, input, p.CommandID})
}

func (p *methodParams[T]) UnmarshalJSON(data []byte) error {
	var wire struct {
		sdk.ParamsBase
		Input     jsontext.Value `json:"input"`
		CommandID CommandID      `json:"commandId,omitzero"`
	}
	if err := json.Unmarshal(data, &wire, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	if wire.CommandID != "" {
		if err := wire.CommandID.Validate(); err != nil {
			return err
		}
	}
	input, admitted, err := decodeMethodInput[T](wire.Input)
	if err != nil {
		return err
	}
	*p = methodParams[T]{ParamsBase: wire.ParamsBase, Input: input, CommandID: wire.CommandID, inputJSON: admitted}
	return nil
}

func decodeMethodInput[T any](data jsontext.Value) (T, framevalue.JSON, error) {
	var input T
	if len(data) == 0 || string(data) == jsonNull {
		return input, framevalue.JSON{}, errors.New("plugin: input is required")
	}
	admitted, err := framevalue.ParseJSON(data)
	if err != nil {
		return input, framevalue.JSON{}, err
	}
	if err := json.Unmarshal(data, &input, json.RejectUnknownMembers(true), framevalue.Numbers()); err != nil {
		return input, framevalue.JSON{}, err
	}
	return input, admitted, nil
}

type methodResult[T any] struct {
	sdk.ResultBase
	Value     *T                 `json:"value,omitzero"`
	Failure   *framecall.Refusal `json:"failure,omitzero"`
	valueJSON jsontext.Value
}

func (r *methodResult[T]) UnmarshalJSON(data []byte) error {
	var decoded struct {
		sdk.ResultBase
		Value   jsontext.Value     `json:"value"`
		Failure *framecall.Refusal `json:"failure,omitzero"`
	}
	if err := json.Unmarshal(data, &decoded, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	var value *T
	if len(decoded.Value) != 0 {
		if string(decoded.Value) == jsonNull {
			switch reflect.TypeFor[T]().Kind() {
			case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Interface:
			default:
				return errors.New("plugin: result value is not nullable")
			}
		}
		value = new(T)
		if err := json.Unmarshal(decoded.Value, value, json.RejectUnknownMembers(true), framevalue.Numbers()); err != nil {
			return err
		}
	}
	*r = methodResult[T]{ResultBase: decoded.ResultBase, Value: value, Failure: decoded.Failure, valueJSON: decoded.Value}
	return nil
}

type Classifier func(err error) (code string, refused bool)

var ErrOutcomeUnconfirmed = errors.New("plugin: outcome is unconfirmed")

const InvalidInputCode = "invalid_input"

// Only an explicitly classified business refusal is a known outcome; encoding
// other failures as refusals would falsely claim that a possibly committed effect
// did not happen.
func reply[T any](value T, err error, classify Classifier) (*methodResult[T], error) {
	if err != nil {
		if errors.Is(err, ErrOutcomeUnconfirmed) {
			return nil, err
		}
		if failure, ok := errors.AsType[*framecall.Refusal](err); ok && failure.Code != "" && utf8.ValidString(failure.Code) && utf8.ValidString(failure.Message) {
			cloned := *failure
			return &methodResult[T]{Failure: &cloned}, nil
		}
		code, refused := framecall.ConflictCode, errors.Is(err, framecall.ErrConflict)
		if !refused && classify != nil {
			code, refused = classify(err)
		}
		if !refused {
			return nil, err
		}
		if code == "" || !utf8.ValidString(code) || !utf8.ValidString(err.Error()) {
			return nil, errors.Join(err, errors.New("plugin: invalid business refusal"))
		}
		return &methodResult[T]{Failure: &framecall.Refusal{Code: code, Message: err.Error()}}, nil
	}
	return &methodResult[T]{Value: &value}, nil
}

func (r *methodResult[T]) value(name string) (T, error) {
	var zero T
	if r == nil {
		return zero, fmt.Errorf("%s: missing result", name)
	}
	if (r.Value == nil) == (r.Failure == nil) {
		return zero, fmt.Errorf("%s: result requires exactly one value or failure", name)
	}
	if r.Failure != nil {
		if r.Failure.Code == "" || !utf8.ValidString(r.Failure.Code) || !utf8.ValidString(r.Failure.Message) {
			return zero, fmt.Errorf("%s: invalid business refusal", name)
		}
		return zero, r.Failure
	}
	return *r.Value, nil
}

func RegisterClient(client *sdk.Client) error {
	if client == nil {
		return errors.New("plugin: client is required")
	}
	return errors.Join(
		sdk.AddSendingCustomMethod[*setupParams, *methodResult[bool]](client, SetupMethod),
		sdk.AddSendingCustomMethod[*protocolParams, *methodResult[bool]](client, ActivateMethod),
		sdk.AddSendingCustomMethod[*invokeParams, *methodResult[jsontext.Value]](client, InvokeMethod),
		sdk.AddSendingCustomMethod[*invokeParams, *methodResult[string]](client, ValidateMethod),
		sdk.AddSendingCustomMethod[*protocolParams, *methodResult[Description]](client, DescribeMethod),
	)
}

type CommandID string

func NewCommandID() CommandID { return CommandID(uuid.NewString()) }

func (id CommandID) Validate() error {
	if !frame.ValidCommandID(string(id)) {
		return errors.New("plugin: bounded logical command identity is required")
	}
	return nil
}

func (CommandID) JSONSchemaConstraints() jsonschema.Constraints {
	return jsonschema.Constraints{MinLength: new(uint64(1)), MaxLength: new(uint64(frame.MaxCommandIDCharacters))}
}
