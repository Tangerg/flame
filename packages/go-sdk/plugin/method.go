package plugin

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"reflect"
	"unicode/utf8"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type methodParams[T any] struct {
	sdk.ParamsBase
	Input     T      `json:"input"`
	CommandID string `json:"commandId,omitempty"`
	inputJSON jsonvalue.Value
}

// Command carries a caller's logical identity and input. Business admission and
// the atomic mutation/receipt commit remain with the business owner.
type Command[T any] struct {
	ID        string
	Input     T
	inputJSON jsonvalue.Value
}

func (c Command[T]) RequestInput() (jsonvalue.Value, error) {
	return c.inputJSON, c.inputJSON.Validate()
}

type requestInputKey struct{}

// RequestInput projects the input received at this method or Tool boundary.
// In particular, a typed numeric conversion cannot originate another identity.
func RequestInput(ctx context.Context) (jsonvalue.Value, error) {
	value, _ := ctx.Value(requestInputKey{}).(jsonvalue.Value)
	return value, value.Validate()
}

func (p methodParams[T]) MarshalJSON() ([]byte, error) {
	input := p.inputJSON
	if input.Validate() != nil {
		var err error
		input, err = jsonvalue.Encode(p.Input)
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(struct {
		sdk.ParamsBase
		Input     jsonvalue.Value `json:"input"`
		CommandID string          `json:"commandId,omitzero"`
	}{p.ParamsBase, input, p.CommandID})
}

func (p *methodParams[T]) UnmarshalJSON(data []byte) error {
	var wire struct {
		sdk.ParamsBase
		Input     jsontext.Value `json:"input"`
		CommandID string         `json:"commandId,omitzero"`
	}
	if err := json.Unmarshal(data, &wire, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	if wire.CommandID != "" {
		if err := validateCommandID(wire.CommandID); err != nil {
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

func decodeMethodInput[T any](data jsontext.Value) (T, jsonvalue.Value, error) {
	var input T
	if len(data) == 0 || string(data) == "null" {
		return input, jsonvalue.Value{}, errors.New("plugin: input is required")
	}
	admitted, err := jsonvalue.Parse(data)
	if err != nil {
		return input, jsonvalue.Value{}, err
	}
	if err := json.Unmarshal(data, &input, json.RejectUnknownMembers(true)); err != nil {
		return input, jsonvalue.Value{}, err
	}
	return input, admitted, nil
}

type methodResult[T any] struct {
	sdk.ResultBase
	Value     *T                 `json:"value,omitzero"`
	Failure   *extension.Refusal `json:"failure,omitzero"`
	valueJSON jsontext.Value
}

func (r *methodResult[T]) UnmarshalJSON(data []byte) error {
	var decoded struct {
		sdk.ResultBase
		Value   jsontext.Value     `json:"value"`
		Failure *extension.Refusal `json:"failure,omitzero"`
	}
	if err := json.Unmarshal(data, &decoded, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	var value *T
	if len(decoded.Value) != 0 {
		if string(decoded.Value) == "null" {
			switch reflect.TypeFor[T]().Kind() {
			case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Interface:
			default:
				return errors.New("plugin: result value is not nullable")
			}
		}
		value = new(T)
		if err := json.Unmarshal(decoded.Value, value, json.RejectUnknownMembers(true)); err != nil {
			return err
		}
	}
	*r = methodResult[T]{ResultBase: decoded.ResultBase, Value: value, Failure: decoded.Failure, valueJSON: decoded.Value}
	return nil
}

// Only an explicitly classified business refusal is a known outcome. Storage,
// transport and unrecognized failures remain protocol errors; encoding them as
// refusals would falsely claim that a possibly committed effect did not happen.
func reply[T any](value T, err error, classify func(error) string) (*methodResult[T], error) {
	if err != nil {
		if failure, ok := errors.AsType[*extension.Refusal](err); ok && failure.Code != "" && utf8.ValidString(failure.Code) && utf8.ValidString(failure.Message) {
			copy := *failure
			return &methodResult[T]{Failure: &copy}, nil
		}
		code := ""
		if classify != nil {
			code = classify(err)
		}
		if code == "" {
			return nil, err
		}
		if !utf8.ValidString(code) || !utf8.ValidString(err.Error()) {
			return nil, errors.Join(err, errors.New("plugin: invalid business refusal"))
		}
		return &methodResult[T]{Failure: &extension.Refusal{Code: code, Message: err.Error()}}, nil
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
		sdk.AddSendingCustomMethod[*setupParams, *methodResult[bool]](client, setupMethod),
		sdk.AddSendingCustomMethod[*protocolParams, *methodResult[bool]](client, activateMethod),
		sdk.AddSendingCustomMethod[*invokeParams, *methodResult[jsontext.Value]](client, invokeMethod),
		sdk.AddSendingCustomMethod[*invokeParams, *methodResult[string]](client, validateMethod),
		sdk.AddSendingCustomMethod[*protocolParams, *methodResult[Description]](client, describeMethod),
	)
}
func validateCommandID(id string) error {
	if id == "" || len(id) > 256 || !utf8.ValidString(id) {
		return errors.New("plugin: bounded logical command identity is required")
	}
	return nil
}
