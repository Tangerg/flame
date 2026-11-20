package client

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"

	"github.com/Tangerg/frame"
	framecall "github.com/Tangerg/frame/call"

	framevalue "github.com/Tangerg/frame/value"
	"github.com/samber/lo"
)

type PluginCaller interface {
	CallPlugin(context.Context, protocol.CallPluginRequest, protocol.PluginCallOptions) (*protocol.CallPluginResponse, error)
}

// PluginBinding retains one concrete installation/release/contribution for a
// consumer contract. Changing a selection does not change an existing binding.
type PluginBinding struct {
	InstallationID string
	Digest         string
	Contribution   string
	Contract       *plugincontract.Capability
	Target         framecall.Target
}

type PluginMethod[P, R any] struct {
	boundMethod
	caller PluginCaller
}

func BindPluginMethod[P, R any](caller PluginCaller, binding PluginBinding, method *pluginmcp.Method[P, R]) (*PluginMethod[P, R], error) {
	if lo.IsNil(caller) || method == nil {
		return nil, errors.New("runtime: plugin caller, binding and typed method are required")
	}
	bound, err := bindMethod(binding, method.Descriptor())
	if err != nil {
		return nil, err
	}
	return &PluginMethod[P, R]{boundMethod: bound, caller: caller}, nil
}

// boundMethod is one admitted method of a captured plugin binding. Calls and
// streams share its request admission and output decoding.
type boundMethod struct {
	binding PluginBinding
	method  *frame.Method
}

func bindMethod(binding PluginBinding, descriptor frame.MethodDescriptor) (boundMethod, error) {
	if binding.Contract == nil {
		return boundMethod{}, errors.New("runtime: plugin binding contract is required")
	}
	binding.Contract = new(*binding.Contract)
	required, found := binding.Contract.Method(descriptor.Name)
	actual, err := frame.AdmitMethod(descriptor)
	if err != nil {
		return boundMethod{}, err
	}
	if !found || required.Identity() != actual.Identity() {
		return boundMethod{}, errors.New("runtime: typed method differs from its required capability")
	}
	if err := binding.Target.Validate(); err != nil {
		return boundMethod{}, err
	}
	if binding.Target.Scope != required.Descriptor().Target {
		return boundMethod{}, errors.New("runtime: plugin target differs from its method contract")
	}
	if err := protocol.ValidateWireTree(protocol.PluginReleaseRequest{InstallationID: binding.InstallationID, Digest: binding.Digest}); err != nil {
		return boundMethod{}, err
	}
	if err := protocol.ValidateWireTree(protocol.SelectPluginProviderRequest{InstallationID: binding.InstallationID, Contribution: binding.Contribution, Role: binding.Contract.Descriptor().Reference}); err != nil {
		return boundMethod{}, err
	}
	return boundMethod{binding: binding, method: required}, nil
}

// request admits one invocation's input against the method contract.
func (b boundMethod) request(input any, options protocol.PluginCallOptions) (protocol.CallPluginRequest, error) {
	if b.method.Kind() == frame.Query && (options.IdempotencyKey != "" || options.IdempotencyNamespace != "") {
		return protocol.CallPluginRequest{}, errors.New("runtime: plugin query cannot carry a command identity")
	}
	encoded, err := framevalue.EncodeJSON(input)
	if err != nil {
		return protocol.CallPluginRequest{}, err
	}
	if err := b.method.ValidateInput(encoded); err != nil {
		return protocol.CallPluginRequest{}, err
	}
	return protocol.CallPluginRequest{
		InstallationID: b.binding.InstallationID, Digest: b.binding.Digest, Contribution: b.binding.Contribution,
		Contract: string(b.binding.Contract.Identity().Bytes()), Method: b.method.Name(), Input: string(encoded.Bytes()), Target: b.binding.Target,
	}, nil
}

// contractBreach is a response that broke the method contract. A command's
// acknowledgement is then unknown, because its effect may already be durable.
func (b boundMethod) contractBreach(err error) error {
	err = errors.Join(protocol.ErrInvalidResponse, err)
	if b.method.Kind() == frame.Command {
		return errors.Join(ErrAcknowledgementUnknown, err)
	}
	return err
}

func decodeOutput[T any](validate func(jsontext.Value) error, raw string) (T, error) {
	var value T
	if err := validate(jsontext.Value(raw)); err != nil {
		return value, err
	}
	err := json.Unmarshal([]byte(raw), &value, json.RejectUnknownMembers(true), framevalue.Numbers())
	return value, err
}

func pluginRefusal(failure *protocol.PluginCallFailure) error {
	return &framecall.Refusal{Code: failure.Code, Message: failure.Message}
}

func (m *PluginMethod[P, R]) Call(ctx context.Context, input P, options protocol.PluginCallOptions) (R, error) {
	var zero R
	request, err := m.request(input, options)
	if err != nil {
		return zero, err
	}
	result, err := m.caller.CallPlugin(ctx, request, options)
	if err != nil {
		return zero, err
	}
	if result == nil {
		return zero, protocol.ErrInvalidResponse
	}
	if err := protocol.ValidateWireTree(*result); err != nil {
		return zero, errors.Join(protocol.ErrInvalidResponse, err)
	}
	if result.Failure != nil {
		return zero, pluginRefusal(result.Failure)
	}
	value, err := decodeOutput[R](m.method.ValidateOutput, result.Output)
	if err != nil {
		return zero, fmt.Errorf("runtime: plugin output: %w", errors.Join(protocol.ErrInvalidResponse, err))
	}
	return value, nil
}
