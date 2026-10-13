package plugin

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/jsoninput"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

// Binding freezes one public operation contract against one concrete session.
// The pure capability owner supplies admission; the feature owns its receipt.
type Binding struct {
	session    *sdk.ClientSession
	method     *capability.Method
	export     string
	capability string
}

// NewBinding accepts an already captured contract. The backend checks its exact
// fingerprint before invocation; restoration does not discover a replacement.
func NewBinding(session *sdk.ClientSession, descriptor capability.MethodDescriptor) (*Binding, error) {
	if session == nil {
		return nil, errors.New("plugin: client session is required")
	}
	method, err := capability.AdmitMethod(descriptor)
	if err != nil {
		return nil, err
	}
	return &Binding{session: session, method: method}, nil
}

func (b *Binding) Descriptor() capability.MethodDescriptor { return b.method.Descriptor() }
func (b *Binding) Contract() jsoninput.Input               { return b.method.Identity() }

func (b *Binding) ValidateInput(input jsoninput.Input) error {
	if b == nil {
		return errors.New("plugin: operation binding is required")
	}
	return b.method.ValidateInput(input)
}

// Resolve discovers a published export for a new call. An original command
// restores its captured descriptor with NewBinding instead.
func Resolve(ctx context.Context, session *sdk.ClientSession, name string) (*Binding, error) {
	description, err := Describe(ctx, session)
	if err != nil {
		return nil, err
	}
	var selected *capability.MethodDescriptor
	seen := make(map[string]bool, len(description.Methods))
	for _, descriptor := range description.Methods {
		if seen[descriptor.Name] {
			return nil, errors.New("plugin: duplicate published method")
		}
		seen[descriptor.Name] = true
		if descriptor.Name == name {
			selected = &descriptor
		}
	}
	if selected == nil {
		return nil, errors.New("plugin: operation export is not published")
	}
	return NewBinding(session, *selected)
}

func (b *Binding) invoke(ctx context.Context, id string, input jsoninput.Input, meta sdk.ParamsBase) (*Result[jsontext.Value], error) {
	var err error
	meta, err = invocationMetadata(ctx, meta)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, errors.New("plugin: operation binding is required")
	}
	if b.method.Kind() == capability.Mutation {
		if err := validateCommandID(id); err != nil {
			return nil, err
		}
	} else if id != "" {
		return nil, errors.New("plugin: query must not carry a command identity")
	}
	if err := b.ValidateInput(input); err != nil {
		return nil, fmt.Errorf("plugin: %s input: %w", b.method.Name(), err)
	}
	result, err := sdk.CallCustomMethod[*invokeParams, *Result[jsontext.Value]](ctx, b.session, invokeMethod, &invokeParams{ParamsBase: meta, Protocol: capability.Protocol, Method: b.method.Name(), Contract: b.method.Identity().Digest(), Export: b.export, Capability: b.capability, Input: input.JSON(), CommandID: id})
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

func (b *Binding) Invoke(ctx context.Context, id string, input jsoninput.Input) (jsoninput.Input, error) {
	return b.InvokeWithMetadata(ctx, id, input, sdk.ParamsBase{})
}

// InvokeWithMetadata keeps ephemeral call collaborators outside admitted input.
func (b *Binding) InvokeWithMetadata(ctx context.Context, id string, input jsoninput.Input, metadata sdk.ParamsBase) (jsoninput.Input, error) {
	result, err := b.invoke(ctx, id, input, metadata)
	if err != nil {
		return jsoninput.Input{}, err
	}
	return jsoninput.Parse(result.valueJSON)
}

func (b *Binding) ReadReceipt(ctx context.Context, id string, input jsoninput.Input) (jsoninput.Input, bool, error) {
	if b == nil || b.method.Kind() != capability.Mutation {
		return jsoninput.Input{}, false, errors.New("plugin: command binding is required")
	}
	// Receipt lookup compares the original admitted evidence. A Tool may have
	// translated those arguments before calling this business method.
	if err := input.Validate(); err != nil {
		return jsoninput.Input{}, false, err
	}
	lookup, err := receiptBinding(b.session)
	if err != nil {
		return jsoninput.Input{}, false, err
	}
	lookup.export, lookup.capability = b.export, b.capability
	value, found, err := readReceipt(ctx, lookup, id, b.method.Name(), input)
	if err != nil || !found {
		return jsoninput.Input{}, found, err
	}
	if err := b.method.ValidateOutput(value); err != nil {
		return jsoninput.Input{}, false, fmt.Errorf("plugin: %s receipt output: %w", b.method.Name(), err)
	}
	admitted, err := jsoninput.Parse(value)
	return admitted, err == nil, err
}

func decodeBoundResult[R any](result *Result[jsontext.Value]) (*Result[R], error) {
	data, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	var typed Result[R]
	if err := json.Unmarshal(data, &typed, json.RejectUnknownMembers(true)); err != nil {
		return nil, err
	}
	return &typed, nil
}
