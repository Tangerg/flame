package managedmcp

import (
	"context"
	"fmt"

	sdk "github.com/Tangerg/go-sdk/mcp"
)

type Params[T any] struct {
	sdk.ParamsBase
	Input     T      `json:"input"`
	CommandID string `json:"commandId,omitempty"`
}
type Result[T any] struct {
	sdk.ResultBase
	Value   *T       `json:"value,omitempty"`
	Failure *Failure `json:"failure,omitempty"`
}
type Failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (f *Failure) Error() string { return f.Message }

func newResult[T any](value T, err error, classify func(error) string) *Result[T] {
	if err != nil {
		return &Result[T]{Failure: &Failure{Code: classify(err), Message: err.Error()}}
	}
	return &Result[T]{Value: &value}
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
		return zero, r.Failure
	}
	return *r.Value, nil
}

func RegisterClient[P, R any](client *sdk.Client, name string) error {
	return sdk.AddSendingCustomMethod[*Params[P], *Result[R]](client, name)
}
func RegisterServer[P, R any](server *sdk.Server, name string, classify func(error) string, handler func(context.Context, *sdk.ServerSession, P) (R, error)) error {
	return sdk.AddReceivingCustomMethod(server, name, func(ctx context.Context, session *sdk.ServerSession, params *Params[P]) (*Result[R], error) {
		if params == nil {
			return nil, fmt.Errorf("%s: parameters are required", name)
		}
		value, err := handler(ctx, session, params.Input)
		return newResult(value, err, classify), nil
	})
}
func Call[P, R any](ctx context.Context, session *sdk.ClientSession, name string, input P) (R, error) {
	return call[P, R](ctx, session, name, &Params[P]{Input: input})
}

func CallCommand[P, R any](ctx context.Context, session *sdk.ClientSession, name, id string, input P) (R, error) {
	var zero R
	if id == "" {
		return zero, fmt.Errorf("%s: command identity is required", name)
	}
	return call[P, R](ctx, session, name, &Params[P]{Input: input, CommandID: id})
}

func call[P, R any](ctx context.Context, session *sdk.ClientSession, name string, params *Params[P]) (R, error) {
	var zero R
	result, err := sdk.CallCustomMethod[*Params[P], *Result[R]](ctx, session, name, params)
	if err != nil {
		return zero, err
	}
	return result.value(name)
}
