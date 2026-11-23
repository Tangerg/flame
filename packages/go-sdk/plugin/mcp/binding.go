package mcp

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"

	plugincontract "github.com/Tangerg/flame/packages/go-sdk/plugin/contract"

	"github.com/Tangerg/frame"
	framevalue "github.com/Tangerg/frame/value"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type Binding struct {
	session    *sdk.ClientSession
	method     *frame.Method
	export     string
	capability framevalue.Digest
}

func NewBinding(session *sdk.ClientSession, descriptor frame.MethodDescriptor) (*Binding, error) {
	if session == nil {
		return nil, errClientSessionRequired
	}
	method, err := frame.AdmitMethod(descriptor)
	if err != nil {
		return nil, err
	}
	return &Binding{session: session, method: method}, nil
}

func (b *Binding) Descriptor() frame.MethodDescriptor { return b.method.Descriptor() }
func (b *Binding) Contract() framevalue.JSON          { return b.method.Identity() }

var ErrInvalidInput = errors.New("plugin: input is not structurally admitted")

func (b *Binding) ValidateInput(ctx context.Context, input framevalue.JSON) error {
	if b == nil {
		return errors.New("plugin: operation binding is required")
	}
	if err := b.method.ValidateInput(input); err != nil {
		return errors.Join(ErrInvalidInput, err)
	}
	result, err := sdk.CallCustomMethod[*invokeParams, *methodResult[string]](ctx, b.session, ValidateMethod, &invokeParams{Protocol: plugincontract.Protocol, Method: b.method.Name(), Contract: b.method.Identity().Digest(), Export: b.export, Capability: b.capability, Input: input.Bytes()})
	if err != nil {
		return err
	}
	problem, err := result.value(ValidateMethod)
	if err != nil {
		return err
	}
	if problem != "" {
		return fmt.Errorf("%w: %s", ErrInvalidInput, problem)
	}
	return nil
}

func (b *Binding) invoke(ctx context.Context, id CommandID, input framevalue.JSON, meta sdk.ParamsBase) (*methodResult[jsontext.Value], error) {
	if b != nil && len(b.method.Descriptor().Stream) != 0 {
		return nil, errors.New("plugin: streaming method requires streaming delivery")
	}
	return b.invokeMode(ctx, id, input, meta)
}

func (b *Binding) invokeMode(ctx context.Context, id CommandID, input framevalue.JSON, meta sdk.ParamsBase) (*methodResult[jsontext.Value], error) {
	params, err := b.invocationParams(ctx, id, input, meta)
	if err != nil {
		return nil, err
	}
	result, err := sdk.CallCustomMethod[*invokeParams, *methodResult[jsontext.Value]](ctx, b.session, InvokeMethod, params)
	if err != nil {
		return nil, err
	}
	if _, err := result.value(b.method.Name()); err != nil {
		return nil, err
	}
	if err := b.method.ValidateOutput(result.valueJSON); err != nil {
		return nil, fmt.Errorf("plugin: %s output: %w", b.method.Name(), err)
	}
	return result, nil
}

func (b *Binding) invocationParams(ctx context.Context, id CommandID, input framevalue.JSON, meta sdk.ParamsBase) (*invokeParams, error) {
	var err error
	meta, err = invocationMetadata(ctx, meta)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, errors.New("plugin: operation binding is required")
	}
	if b.method.Kind() == frame.Command {
		if err := id.Validate(); err != nil {
			return nil, err
		}
	} else if id != "" {
		return nil, errQueryCommandIdentity
	}
	if err := b.method.ValidateInput(input); err != nil {
		return nil, fmt.Errorf("plugin: %s input: %w", b.method.Name(), err)
	}
	return &invokeParams{ParamsBase: meta, Protocol: plugincontract.Protocol, Method: b.method.Name(), Contract: b.method.Identity().Digest(), Export: b.export, Capability: b.capability, Input: input.Bytes(), CommandID: id}, nil
}

func (b *Binding) Invoke(ctx context.Context, id CommandID, input framevalue.JSON) (framevalue.JSON, error) {
	return b.InvokeWithMetadata(ctx, id, input, sdk.ParamsBase{})
}

func (b *Binding) InvokeWithMetadata(ctx context.Context, id CommandID, input framevalue.JSON, metadata sdk.ParamsBase) (framevalue.JSON, error) {
	if b != nil && len(b.method.Descriptor().Stream) != 0 {
		return framevalue.JSON{}, errors.New("plugin: streaming method requires streaming delivery")
	}
	result, err := b.invoke(ctx, id, input, metadata)
	if err != nil {
		return framevalue.JSON{}, err
	}
	return framevalue.ParseJSON(result.valueJSON)
}

func (b *Binding) ReadReceipt(ctx context.Context, id CommandID, input framevalue.JSON) (framevalue.JSON, bool, error) {
	if b == nil || b.method.Kind() != frame.Command {
		return framevalue.JSON{}, false, errors.New("plugin: command binding is required")
	}
	// Receipt lookup compares the original admitted evidence. A Tool may have
	// translated those arguments before calling this business method.
	if err := input.Validate(); err != nil {
		return framevalue.JSON{}, false, err
	}
	lookup, err := receiptBinding(b.session)
	if err != nil {
		return framevalue.JSON{}, false, err
	}
	lookup.export, lookup.capability = b.export, b.capability
	value, found, err := readReceipt(ctx, lookup, id, b.method.Name(), input)
	if err != nil || !found {
		return framevalue.JSON{}, found, err
	}
	if err := b.method.ValidateOutput(value); err != nil {
		return framevalue.JSON{}, false, fmt.Errorf("plugin: %s receipt output: %w", b.method.Name(), err)
	}
	admitted, err := framevalue.ParseJSON(value)
	return admitted, err == nil, err
}

func decodeBoundResult[R any](result *methodResult[jsontext.Value]) (*methodResult[R], error) {
	data, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	var typed methodResult[R]
	if err := json.Unmarshal(data, &typed, json.RejectUnknownMembers(true)); err != nil {
		return nil, err
	}
	return &typed, nil
}
