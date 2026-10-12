package plugin

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"reflect"
	"unicode/utf8"

	sdk "github.com/Tangerg/go-sdk/mcp"
)

type Params[T any] struct {
	sdk.ParamsBase
	Input     T      `json:"input"`
	CommandID string `json:"commandId,omitempty"`
	inputJSON jsontext.Value
}

// Command carries a caller's logical identity and input. Business admission and
// the atomic mutation/receipt commit remain with the business owner.
type Command[T any] struct {
	ID    string
	Input T
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
	if wire.CommandID != "" {
		if err := validateCommandID(wire.CommandID); err != nil {
			return err
		}
	}
	var input T
	if err := json.Unmarshal(wire.Input, &input, json.RejectUnknownMembers(true)); err != nil {
		return err
	}
	*p = Params[T]{ParamsBase: wire.ParamsBase, Input: input, CommandID: wire.CommandID, inputJSON: wire.Input}
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

func RegisterClient[P, R any](client *sdk.Client, name string) error {
	if client == nil || name == "" || !utf8.ValidString(name) {
		return errors.New("plugin: client and method name are required")
	}
	return sdk.AddSendingCustomMethod[*Params[P], *Result[R]](client, name)
}
func RegisterServer[P, R any](server *sdk.Server, name string, classify func(error) string, handler func(context.Context, *sdk.ServerSession, P) (R, error)) error {
	if server == nil || handler == nil || name == "" || !utf8.ValidString(name) {
		return errors.New("plugin: server, method name and handler are required")
	}
	return sdk.AddReceivingCustomMethod(server, name, func(ctx context.Context, session *sdk.ServerSession, params *Params[P]) (*Result[R], error) {
		if params == nil || params.CommandID != "" {
			return nil, fmt.Errorf("%s: unkeyed method parameters are required", name)
		}
		value, err := handler(ctx, session, params.Input)
		return Reply(value, err, classify)
	})
}

func RegisterCommand[P, R any](server *sdk.Server, name string, classify func(error) string, handler func(context.Context, *sdk.ServerSession, Command[P]) (R, error)) error {
	if server == nil || handler == nil || name == "" || !utf8.ValidString(name) {
		return errors.New("plugin: server, method name and command handler are required")
	}
	return sdk.AddReceivingCustomMethod(server, name, func(ctx context.Context, session *sdk.ServerSession, params *Params[P]) (*Result[R], error) {
		if params == nil {
			return nil, fmt.Errorf("%s: command parameters are required", name)
		}
		if err := validateCommandID(params.CommandID); err != nil {
			return nil, err
		}
		value, err := handler(ctx, session, Command[P]{ID: params.CommandID, Input: params.Input})
		return Reply(value, err, classify)
	})
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

func call[P, R any](ctx context.Context, session *sdk.ClientSession, name string, params *Params[P]) (R, error) {
	var zero R
	if session == nil {
		return zero, errors.New("plugin: client session is required")
	}
	result, err := sdk.CallCustomMethod[*Params[P], *Result[R]](ctx, session, name, params)
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
