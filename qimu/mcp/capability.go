package mcp

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/qimu/jsonvalue"
	"github.com/Tangerg/flame/qimu/portable"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type CapabilityExport struct {
	Name     string                        `json:"name"`
	Contract portable.CapabilityDescriptor `json:"contract"`
}

// BindCapability associates the consumer contract with existing handlers.
// The Backend remains the sole publication and invocation owner.
func (b *Backend) BindCapability(name string, contract *portable.Capability) error {
	if b == nil || contract == nil || contract.Identity().Validate() != nil || !portable.ValidExportName(name) {
		return errors.New("plugin: backend, capability and valid export name are required")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.published || b.closed {
		return errors.New("plugin: published backend cannot change exports")
	}
	if _, exists := b.capabilities[name]; exists {
		return errors.New("plugin: duplicate capability export")
	}
	for _, descriptor := range contract.Descriptor().Methods {
		required, _ := contract.Method(descriptor.Name)
		method, exists := b.methods[descriptor.Name]
		if !exists || method.contract.Identity() != required.Identity() {
			return fmt.Errorf("plugin: capability export %s requires exact method %s", name, descriptor.Name)
		}
	}
	b.capabilities[name] = contract
	return nil
}

// Called under the Backend read lock. A capability member cannot lose its
// export identity by entering through a bare-method call, including lookup.
func (b *Backend) checkCapability(params *invokeParams) error {
	member := params.Method
	if member == ReceiptMethod {
		var request ReceiptRequest
		if err := json.Unmarshal(params.Input, &request); err != nil {
			return err
		}
		member = request.Method
	}
	if params.Export == "" {
		for _, contract := range b.capabilities {
			if _, found := contract.Method(member); found {
				return errors.New("plugin: capability member requires its export binding")
			}
		}
		return nil
	}
	contract, exists := b.capabilities[params.Export]
	if !exists || contract.Identity().Digest() != params.Capability {
		return errors.New("plugin: capability export contract mismatch")
	}
	if _, found := contract.Method(member); !found {
		return errors.New("plugin: method is not in the capability export")
	}
	return nil
}

type CapabilityBinding struct {
	session  *sdk.ClientSession
	export   string
	contract *portable.Capability
}

// NewCapabilityBinding restores the originally captured contract and export.
// The backend checks both before executing; restoration never discovers a new
// provider. Installation/release retention belongs to the host routing owner.
func NewCapabilityBinding(session *sdk.ClientSession, name string, contract *portable.Capability) (*CapabilityBinding, error) {
	if session == nil || contract == nil || contract.Identity().Validate() != nil || !portable.ValidExportName(name) {
		return nil, errors.New("plugin: session, capability and valid export name are required")
	}
	return &CapabilityBinding{session: session, export: name, contract: contract}, nil
}

func ResolveCapability(ctx context.Context, session *sdk.ClientSession, name string, required *portable.Capability) (*CapabilityBinding, error) {
	binding, err := NewCapabilityBinding(session, name, required)
	if err != nil {
		return nil, err
	}
	description, err := Describe(ctx, session)
	if err != nil {
		return nil, err
	}
	published, err := admitDescription(description)
	if err != nil {
		return nil, err
	}
	if actual := published[name]; actual == nil || actual.Identity() != required.Identity() {
		return nil, errors.New("plugin: capability export contract mismatch")
	}
	return binding, nil
}

// VerifyCapabilities admits the complete declared export set before an installed
// entry's session or Tools can be published by the connection owner.
func VerifyCapabilities(ctx context.Context, session *sdk.ClientSession, required []CapabilityExport) error {
	description, err := Describe(ctx, session)
	if err != nil {
		return err
	}
	published, err := admitDescription(description)
	if err != nil {
		return err
	}
	if len(published) != len(required) {
		return errors.New("plugin: declared and published capability exports differ")
	}
	seen := make(map[string]bool, len(required))
	for _, exported := range required {
		if !portable.ValidExportName(exported.Name) || seen[exported.Name] {
			return errors.New("plugin: invalid or duplicate declared capability export")
		}
		seen[exported.Name] = true
		contract, err := portable.AdmitCapability(exported.Contract)
		if err != nil {
			return err
		}
		if actual := published[exported.Name]; actual == nil || actual.Identity() != contract.Identity() {
			return fmt.Errorf("plugin: capability export %s contract mismatch", exported.Name)
		}
	}
	return nil
}

func admitDescription(description Description) (map[string]*portable.Capability, error) {
	methods := make(map[string]jsonvalue.Value, len(description.Methods))
	for _, descriptor := range description.Methods {
		if _, exists := methods[descriptor.Name]; exists {
			return nil, errors.New("plugin: duplicate published method")
		}
		method, err := portable.AdmitMethod(descriptor)
		if err != nil {
			return nil, err
		}
		methods[descriptor.Name] = method.Identity()
	}
	exports := make(map[string]*portable.Capability, len(description.Capabilities))
	for _, published := range description.Capabilities {
		if !portable.ValidExportName(published.Name) || exports[published.Name] != nil {
			return nil, errors.New("plugin: invalid or duplicate published capability export")
		}
		actual, err := portable.AdmitCapability(published.Contract)
		if err != nil {
			return nil, err
		}
		for _, descriptor := range actual.Descriptor().Methods {
			method, _ := actual.Method(descriptor.Name)
			if methods[descriptor.Name] != method.Identity() {
				return nil, errors.New("plugin: capability method is not published with its exact contract")
			}
		}
		exports[published.Name] = actual
	}
	return exports, nil
}

func (b *CapabilityBinding) Method(name string) (*Binding, error) {
	if b == nil {
		return nil, errors.New("plugin: capability binding is required")
	}
	method, found := b.contract.Method(name)
	if !found {
		return nil, errors.New("plugin: method is not in the capability contract")
	}
	return &Binding{session: b.session, method: method, export: b.export, capability: b.contract.Identity().Digest()}, nil
}
