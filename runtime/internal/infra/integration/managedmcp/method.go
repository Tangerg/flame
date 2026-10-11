// Package managedmcp encodes the closed host integrations of managed plugin
// backends over their existing MCP session. It grants no generic client tunnel.
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

func RegisterClient[P, R any](client *sdk.Client, name string) error {
	return sdk.AddSendingCustomMethod[*Params[P], *Result[R]](client, name)
}
func RegisterServer[P, R any](server *sdk.Server, name string, classify func(error) string, handler func(context.Context, *sdk.ServerSession, P) (R, error)) error {
	return sdk.AddReceivingCustomMethod(server, name, func(ctx context.Context, session *sdk.ServerSession, params *Params[P]) (*Result[R], error) {
		if params == nil {
			return nil, fmt.Errorf("%s: parameters are required", name)
		}
		value, err := handler(ctx, session, params.Input)
		result := &Result[R]{}
		if err == nil {
			result.Value = &value
		}
		if err != nil {
			result.Failure = &Failure{Code: classify(err), Message: err.Error()}
		}
		return result, nil
	})
}
func Call[P, R any](ctx context.Context, session *sdk.ClientSession, name string, input P) (R, error) {
	var zero R
	result, err := sdk.CallCustomMethod[*Params[P], *Result[R]](ctx, session, name, &Params[P]{Input: input})
	if err != nil {
		return zero, err
	}
	if result == nil {
		return zero, fmt.Errorf("%s: missing result", name)
	}
	if result.Failure != nil {
		return zero, result.Failure
	}
	if result.Value == nil {
		return zero, fmt.Errorf("%s: successful result is absent", name)
	}
	return *result.Value, nil
}

func CallCommand[P, R any](ctx context.Context, session *sdk.ClientSession, name, id string, input P) (R, error) {
	var zero R
	if id == "" {
		return zero, fmt.Errorf("%s: command identity is required", name)
	}
	result, err := sdk.CallCustomMethod[*Params[P], *Result[R]](ctx, session, name, &Params[P]{Input: input, CommandID: id})
	if err != nil {
		return zero, err
	}
	if result == nil {
		return zero, fmt.Errorf("%s: missing result", name)
	}
	if result.Failure != nil {
		return zero, result.Failure
	}
	if result.Value == nil {
		return zero, fmt.Errorf("%s: successful result is absent", name)
	}
	return *result.Value, nil
}
