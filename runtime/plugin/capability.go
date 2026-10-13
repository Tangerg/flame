package plugin

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/jsoninput"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type CapabilityExport struct {
	Name     string                `json:"name"`
	Contract capability.Descriptor `json:"contract"`
}

// BindCapability associates the consumer contract with existing handlers.
// The Backend remains the sole publication and invocation owner.
func (b *Backend) BindCapability(name string, contract *capability.Contract) error {
	if b == nil || contract == nil || contract.Identity().Validate() != nil || !capability.ValidExportName(name) {
		return errors.New("plugin: backend, capability and valid export name are required")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.published {
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
	contract *capability.Contract
}

// NewCapabilityBinding restores the originally captured contract and export.
// The backend checks both before executing; restoration never discovers a new
// provider. Installation/release retention belongs to the host routing owner.
func NewCapabilityBinding(session *sdk.ClientSession, name string, contract *capability.Contract) (*CapabilityBinding, error) {
	if session == nil || contract == nil || contract.Identity().Validate() != nil || !capability.ValidExportName(name) {
		return nil, errors.New("plugin: session, capability and valid export name are required")
	}
	return &CapabilityBinding{session: session, export: name, contract: contract}, nil
}

func ResolveCapability(ctx context.Context, session *sdk.ClientSession, name string, required *capability.Contract) (*CapabilityBinding, error) {
	binding, err := NewCapabilityBinding(session, name, required)
	if err != nil {
		return nil, err
	}
	description, err := Describe(ctx, session)
	if err != nil {
		return nil, err
	}
	methods := make(map[string]jsoninput.Input, len(description.Methods))
	for _, descriptor := range description.Methods {
		if _, exists := methods[descriptor.Name]; exists {
			return nil, errors.New("plugin: duplicate published method")
		}
		method, err := capability.AdmitMethod(descriptor)
		if err != nil {
			return nil, err
		}
		methods[descriptor.Name] = method.Identity()
	}
	seen := make(map[string]bool, len(description.Capabilities))
	found := false
	for _, published := range description.Capabilities {
		if !capability.ValidExportName(published.Name) || seen[published.Name] {
			return nil, errors.New("plugin: invalid or duplicate published capability export")
		}
		seen[published.Name] = true
		if published.Name != name {
			continue
		}
		actual, err := capability.FromDescriptor(published.Contract)
		if err != nil {
			return nil, err
		}
		if actual.Identity() != required.Identity() {
			return nil, errors.New("plugin: capability contract mismatch")
		}
		for _, descriptor := range actual.Descriptor().Methods {
			method, _ := actual.Method(descriptor.Name)
			if methods[descriptor.Name] != method.Identity() {
				return nil, errors.New("plugin: capability method is not published with its exact contract")
			}
		}
		found = true
	}
	if !found {
		return nil, errors.New("plugin: capability export is not published")
	}
	return binding, nil
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
