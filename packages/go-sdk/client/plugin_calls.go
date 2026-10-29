package client

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
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
	Contract       *portable.Capability
	Target         portable.Target
}

type PluginMethod[P, R any] struct {
	caller  PluginCaller
	binding PluginBinding
	method  *portable.Method
}

func BindPluginMethod[P, R any](caller PluginCaller, binding PluginBinding, method *qimumcp.Method[P, R]) (*PluginMethod[P, R], error) {
	if lo.IsNil(caller) || binding.Contract == nil || method == nil {
		return nil, errors.New("runtime: plugin caller, binding and typed method are required")
	}
	required, err := admitPluginBinding(binding, method.Descriptor())
	if err != nil {
		return nil, err
	}
	return &PluginMethod[P, R]{caller: caller, binding: binding, method: required}, nil
}

func admitPluginBinding(binding PluginBinding, descriptor portable.MethodDescriptor) (*portable.Method, error) {
	if binding.Contract == nil {
		return nil, errors.New("runtime: plugin binding contract is required")
	}
	required, found := binding.Contract.Method(descriptor.Name)
	actual, err := portable.AdmitMethod(descriptor)
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
	if m == nil || m.method == nil {
		return zero, errors.New("runtime: bound plugin method is required")
	}
	if m.method.Kind() == portable.Query && (options.IdempotencyKey != "" || options.IdempotencyNamespace != "") {
		return zero, errors.New("runtime: plugin query cannot carry a command identity")
	}
	inputJSON, err := jsonvalue.Encode(input)
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
		return zero, &portable.Refusal{Code: result.Failure.Code, Message: result.Failure.Message}
	}
	if err := m.method.ValidateOutput([]byte(result.Output)); err != nil {
		return zero, fmt.Errorf("runtime: plugin output: %w", errors.Join(protocol.ErrInvalidResponse, err))
	}
	var value R
	if err := json.Unmarshal([]byte(result.Output), &value, json.RejectUnknownMembers(true), jsonvalue.Numbers()); err != nil {
		return zero, errors.Join(protocol.ErrInvalidResponse, err)
	}
	return value, nil
}
