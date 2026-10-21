package runtimeclient

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/dependency"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"github.com/Tangerg/flame/runtime/protocol"
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
	Contract       *capability.Contract
	Target         capability.Target
}

type PluginMethod[P, R any] struct {
	caller  PluginCaller
	binding PluginBinding
	method  *capability.Method
}

func BindPluginMethod[P, R any](caller PluginCaller, binding PluginBinding, method *pluginsdk.Method[P, R]) (*PluginMethod[P, R], error) {
	if dependency.Missing(caller) || binding.Contract == nil || method == nil {
		return nil, errors.New("runtime: plugin caller, binding and typed method are required")
	}
	required, err := admitPluginBinding(binding, method.Descriptor())
	if err != nil {
		return nil, err
	}
	return &PluginMethod[P, R]{caller: caller, binding: binding, method: required}, nil
}

func admitPluginBinding(binding PluginBinding, descriptor capability.MethodDescriptor) (*capability.Method, error) {
	if binding.Contract == nil {
		return nil, errors.New("runtime: plugin binding contract is required")
	}
	required, found := binding.Contract.Method(descriptor.Name)
	actual, err := capability.AdmitMethod(descriptor)
	if err != nil {
		return nil, err
	}
	if !found || required.Identity() != actual.Identity() {
		return nil, errors.New("runtime: typed method differs from its required capability")
	}
	if err := binding.Target.Validate(); err != nil {
		return nil, err
	}
	if binding.Target.Scope != required.Descriptor().Target {
		return nil, errors.New("runtime: plugin target differs from its method contract")
	}
	if err := protocol.ValidateWireTree(protocol.PluginReleaseRequest{InstallationID: binding.InstallationID, Digest: binding.Digest}); err != nil {
		return nil, err
	}
	if err := protocol.ValidateWireTree(protocol.SelectPluginProviderRequest{InstallationID: binding.InstallationID, Contribution: binding.Contribution, Role: binding.Contract.Descriptor().Reference}); err != nil {
		return nil, err
	}
	return required, nil
}

func (m *PluginMethod[P, R]) Call(ctx context.Context, input P, options protocol.PluginCallOptions) (R, error) {
	var zero R
	if m == nil {
		return zero, errors.New("runtime: bound plugin method is required")
	}
	if m.method.Kind() == capability.Query && (options.IdempotencyKey != "" || options.IdempotencyNamespace != "") {
		return zero, errors.New("runtime: plugin query cannot carry a command identity")
	}
	inputJSON, err := jsoninput.Encode(input)
	if err != nil {
		return zero, err
	}
	if err := m.method.ValidateInput(inputJSON); err != nil {
		return zero, err
	}
	result, err := m.caller.CallPlugin(ctx, protocol.CallPluginRequest{InstallationID: m.binding.InstallationID, Digest: m.binding.Digest, Contribution: m.binding.Contribution, Contract: string(m.binding.Contract.Identity().JSON()), Method: m.method.Name(), Input: string(inputJSON.JSON()), Target: m.binding.Target}, options)
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
		return zero, &capability.Refusal{Code: result.Failure.Code, Message: result.Failure.Message}
	}
	if err := m.method.ValidateOutput([]byte(result.Output)); err != nil {
		return zero, fmt.Errorf("runtime: plugin output: %w", errors.Join(protocol.ErrInvalidResponse, err))
	}
	var value R
	if err := json.Unmarshal([]byte(result.Output), &value, json.RejectUnknownMembers(true)); err != nil {
		return zero, errors.Join(protocol.ErrInvalidResponse, err)
	}
	return value, nil
}
