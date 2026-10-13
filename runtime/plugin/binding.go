package plugin

import (
	"context"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/jsoninput"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/jsonschema"
)

// Binding freezes one public operation contract against one concrete session.
// It owns structural admission, not the feature's mutation or receipt.
type Binding struct {
	session    *sdk.ClientSession
	descriptor MethodDescriptor
	contract   jsoninput.Input
	input      jsonschema.Schema
	output     jsonschema.Schema
}

// NewBinding accepts an already captured contract. The backend checks its exact
// fingerprint before invocation; restoration does not discover a replacement.
func NewBinding(session *sdk.ClientSession, descriptor MethodDescriptor) (*Binding, error) {
	if session == nil || descriptor.Name == "" || len(descriptor.Name) > 256 || !utf8.ValidString(descriptor.Name) || descriptor.Target == "" || len(descriptor.Target) > 256 || !utf8.ValidString(descriptor.Target) {
		return nil, errors.New("plugin: session, method and target are required")
	}
	if descriptor.Kind != Query && descriptor.Kind != Mutation {
		return nil, errors.New("plugin: method kind must be query or command")
	}
	input, err := jsonschema.Parse(descriptor.Input)
	if err != nil {
		return nil, fmt.Errorf("plugin: %s input contract: %w", descriptor.Name, err)
	}
	if input.Validate([]byte("null")) == nil {
		return nil, errors.New("plugin: method input must exclude null")
	}
	output, err := jsonschema.Parse(descriptor.Output)
	if err != nil {
		return nil, fmt.Errorf("plugin: %s output contract: %w", descriptor.Name, err)
	}
	contract, err := methodContract(descriptor)
	if err != nil {
		return nil, err
	}
	return &Binding{session: session, descriptor: cloneDescriptor(descriptor), contract: contract, input: input, output: output}, nil
}

func methodContract(descriptor MethodDescriptor) (jsoninput.Input, error) {
	// Prose can change without changing the callable contract. The method,
	// target, kind and exact schemas cannot.
	descriptor.Description = ""
	return jsoninput.Encode(descriptor)
}

func (b *Binding) Descriptor() MethodDescriptor { return cloneDescriptor(b.descriptor) }
func (b *Binding) Contract() jsoninput.Input    { return b.contract }

func (b *Binding) ValidateInput(input jsoninput.Input) error {
	if b == nil {
		return errors.New("plugin: operation binding is required")
	}
	if err := input.Validate(); err != nil {
		return err
	}
	return b.input.Validate(input.JSON())
}

// Resolve discovers a published export for a new call. An original command
// restores its captured descriptor with NewBinding instead.
func Resolve(ctx context.Context, session *sdk.ClientSession, name string) (*Binding, error) {
	description, err := Describe(ctx, session)
	if err != nil {
		return nil, err
	}
	var selected *MethodDescriptor
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
	if b == nil {
		return nil, errors.New("plugin: operation binding is required")
	}
	if b.descriptor.Kind == Mutation {
		if err := validateCommandID(id); err != nil {
			return nil, err
		}
	} else if id != "" {
		return nil, errors.New("plugin: query must not carry a command identity")
	}
	if err := b.ValidateInput(input); err != nil {
		return nil, fmt.Errorf("plugin: %s input: %w", b.descriptor.Name, err)
	}
	result, err := sdk.CallCustomMethod[*invokeParams, *Result[jsontext.Value]](ctx, b.session, invokeMethod, &invokeParams{ParamsBase: meta, Protocol: BackendProtocol, Method: b.descriptor.Name, Contract: b.contract.Digest(), Input: input.JSON(), CommandID: id})
	if err != nil {
		return nil, err
	}
	if _, err := result.value(b.descriptor.Name); err != nil {
		return nil, err
	}
	if err := b.output.Validate(result.valueJSON); err != nil {
		return nil, fmt.Errorf("plugin: %s output: %w", b.descriptor.Name, err)
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
	if b == nil || b.descriptor.Kind != Mutation {
		return jsoninput.Input{}, false, errors.New("plugin: command binding is required")
	}
	if err := b.ValidateInput(input); err != nil {
		return jsoninput.Input{}, false, err
	}
	value, found, err := ReadReceipt[jsontext.Value, jsontext.Value](ctx, b.session, id, b.descriptor.Name, input.JSON())
	if err != nil || !found {
		return jsoninput.Input{}, found, err
	}
	if err := b.output.Validate(value); err != nil {
		return jsoninput.Input{}, false, fmt.Errorf("plugin: %s receipt output: %w", b.descriptor.Name, err)
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
