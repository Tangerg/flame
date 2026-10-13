package plugin

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"reflect"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/jsoninput"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type Params[T any] struct {
	sdk.ParamsBase
	Input     T      `json:"input"`
	CommandID string `json:"commandId,omitempty"`
	inputJSON jsoninput.Input
}

// Command carries a caller's logical identity and input. Business admission and
// the atomic mutation/receipt commit remain with the business owner.
type Command[T any] struct {
	ID        string
	Input     T
	inputJSON jsoninput.Input
}

func (c Command[T]) RequestInput() (jsoninput.Input, error) {
	return c.inputJSON, c.inputJSON.Validate()
}

type requestInputKey struct{}

// RequestInput projects the input received at this method or Tool boundary.
// In particular, a typed numeric conversion cannot originate another identity.
func RequestInput(ctx context.Context) (jsoninput.Input, error) {
	value, _ := ctx.Value(requestInputKey{}).(jsoninput.Input)
	return value, value.Validate()
}

func (p Params[T]) MarshalJSON() ([]byte, error) {
	input := p.inputJSON
	if input.Validate() != nil {
		var err error
		input, err = jsoninput.Encode(p.Input)
		if err != nil {
			return nil, err
		}
	}
	return json.Marshal(struct {
		sdk.ParamsBase
		Input     jsoninput.Input `json:"input"`
		CommandID string          `json:"commandId,omitzero"`
	}{p.ParamsBase, input, p.CommandID})
}

func (p *Params[T]) UnmarshalJSON(data []byte) error {
	var wire struct {
		sdk.ParamsBase
		Input     jsontext.Value `json:"input"`
		CommandID string         `json:"commandId,omitzero"`
	}
	if err := json.Unmarshal(data, &wire, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	if len(wire.Input) == 0 || string(wire.Input) == "null" {
		return errors.New("plugin: input is required")
	}
	admitted, err := jsoninput.Parse(wire.Input)
	if err != nil {
		return err
	}
	if wire.CommandID != "" {
		if err := validateCommandID(wire.CommandID); err != nil {
			return err
		}
	}
	var input T
	if err := json.Unmarshal(wire.Input, &input, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	*p = Params[T]{ParamsBase: wire.ParamsBase, Input: input, CommandID: wire.CommandID, inputJSON: admitted}
	return nil
}

type Result[T any] struct {
	sdk.ResultBase
	Value     *T       `json:"value,omitzero"`
	Failure   *Failure `json:"failure,omitzero"`
	valueJSON jsontext.Value
}
type Failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (r *Result[T]) UnmarshalJSON(data []byte) error {
	var decoded struct {
		sdk.ResultBase
		Value   jsontext.Value `json:"value"`
		Failure *Failure       `json:"failure,omitzero"`
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
	*r = Result[T]{ResultBase: decoded.ResultBase, Value: value, Failure: decoded.Failure, valueJSON: decoded.Value}
	return nil
}

func (f *Failure) Error() string { return f.Message }

// Only an explicitly classified business refusal is a known outcome. Storage,
// transport and unrecognized failures remain protocol errors; encoding them as
// refusals would falsely claim that a possibly committed effect did not happen.
func Reply[T any](value T, err error, classify func(error) string) (*Result[T], error) {
	if err != nil {
		if failure, ok := errors.AsType[*Failure](err); ok && failure.Code != "" && utf8.ValidString(failure.Code) && utf8.ValidString(failure.Message) {
			copy := *failure
			return &Result[T]{Failure: &copy}, nil
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
		return &Result[T]{Failure: &Failure{Code: code, Message: err.Error()}}, nil
	}
	return &Result[T]{Value: &value}, nil
}

func (r *Result[T]) value(name string) (T, error) {
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
		sdk.AddSendingCustomMethod[*invokeParams, *Result[jsontext.Value]](client, invokeMethod),
		sdk.AddSendingCustomMethod[*describeParams, *Result[Description]](client, describeMethod),
	)
}
func RegisterServer[P, R any](backend *Backend, name string, classify func(error) string, handler func(context.Context, *sdk.ServerSession, P) (R, error)) error {
	if handler == nil {
		return errors.New("plugin: method handler is required")
	}
	method, err := NewMethod[P, R](MethodConfig{Name: name, Kind: capability.Query, Target: "installation", Description: name})
	if err != nil {
		return err
	}
	return method.RegisterServer(backend, classify, func(ctx context.Context, session *sdk.ServerSession, command Command[P]) (R, error) {
		return handler(ctx, session, command.Input)
	})
}
func RegisterCommand[P, R any](backend *Backend, name string, classify func(error) string, handler func(context.Context, *sdk.ServerSession, Command[P]) (R, error)) error {
	if handler == nil {
		return errors.New("plugin: command handler is required")
	}
	method, err := NewMethod[P, R](MethodConfig{Name: name, Kind: capability.Mutation, Target: "installation", Description: name})
	if err != nil {
		return err
	}
	return method.RegisterServer(backend, classify, handler)
}

func Call[P, R any](ctx context.Context, session *sdk.ClientSession, name string, input P) (R, error) {
	return call[P, R](ctx, session, name, &Params[P]{Input: input})
}

func CallCommand[P, R any](ctx context.Context, session *sdk.ClientSession, name, id string, input P) (R, error) {
	var zero R
	if err := validateCommandID(id); err != nil {
		return zero, err
	}
	return call[P, R](ctx, session, name, &Params[P]{Input: input, CommandID: id})
}

func callResult[P, R any](ctx context.Context, session *sdk.ClientSession, name string, params *Params[P]) (*Result[R], error) {
	if session == nil {
		return nil, errors.New("plugin: client session is required")
	}
	input, err := jsoninput.Encode(params.Input)
	if err != nil {
		return nil, err
	}
	binding, err := Resolve(ctx, session, name)
	if err != nil {
		return nil, err
	}
	wire, err := binding.invoke(ctx, params.CommandID, input, params.ParamsBase)
	if err != nil {
		return nil, err
	}
	return decodeBoundResult[R](wire)
}
func call[P, R any](ctx context.Context, session *sdk.ClientSession, name string, params *Params[P]) (R, error) {
	var zero R
	result, err := callResult[P, R](ctx, session, name, params)
	if err != nil {
		return zero, err
	}
	return result.value(name)
}

func validateCommandID(id string) error {
	if id == "" || len(id) > 256 || !utf8.ValidString(id) {
		return errors.New("plugin: bounded logical command identity is required")
	}
	return nil
}
