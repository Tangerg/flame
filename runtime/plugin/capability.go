package plugin

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/jsoninput"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

type Composition string

const (
	Collection Composition = "collection"
	Selection  Composition = "selection"
	Decoration Composition = "decoration"
)

type CapabilityConfig struct {
	Reference   string
	Composition Composition
	Scope       string
}

// CapabilityDescriptor projects a consumer's callable contract. It contains no
// producer identity or transport location; those belong to its concrete binding.
type CapabilityDescriptor struct {
	Protocol    string             `json:"protocol"`
	Reference   string             `json:"reference"`
	Composition Composition        `json:"composition"`
	Scope       string             `json:"scope"`
	Methods     []MethodDescriptor `json:"methods"`
}

type Capability struct {
	descriptor CapabilityDescriptor
	contract   jsoninput.Input
}

func NewCapability(config CapabilityConfig, methods ...MethodDescriptor) (*Capability, error) {
	return admitCapability(CapabilityDescriptor{Protocol: BackendProtocol, Reference: config.Reference, Composition: config.Composition, Scope: config.Scope, Methods: methods})
}

// ParseCapability admits local generated metadata without loading executable
// code or fetching schemas. Only the current public protocol is interpreted.
func ParseCapability(data []byte) (*Capability, error) {
	var descriptor CapabilityDescriptor
	if err := json.Unmarshal(data, &descriptor, json.RejectUnknownMembers(true)); err != nil {
		return nil, err
	}
	return admitCapability(descriptor)
}

func contractToken(value string) bool {
	return value != "" && len(value) <= 256 && utf8.ValidString(value) && !strings.ContainsFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
}

func admitCapability(descriptor CapabilityDescriptor) (*Capability, error) {
	if descriptor.Protocol != BackendProtocol {
		return nil, errors.New("plugin: current capability protocol is required")
	}
	if !contractToken(descriptor.Reference) || !contractToken(descriptor.Scope) {
		return nil, errors.New("plugin: capability reference and scope are required bounded tokens")
	}
	if descriptor.Composition != Collection && descriptor.Composition != Selection && descriptor.Composition != Decoration {
		return nil, errors.New("plugin: unknown capability composition")
	}
	if len(descriptor.Methods) == 0 {
		return nil, errors.New("plugin: capability requires methods")
	}
	descriptor = cloneCapability(descriptor)
	slices.SortFunc(descriptor.Methods, func(a, b MethodDescriptor) int { return strings.Compare(a.Name, b.Name) })
	for i, method := range descriptor.Methods {
		if i > 0 && descriptor.Methods[i-1].Name == method.Name {
			return nil, errors.New("plugin: duplicate capability method")
		}
		if _, err := admitDescriptor(method); err != nil {
			return nil, err
		}
	}
	identity := cloneCapability(descriptor)
	for i := range identity.Methods {
		identity.Methods[i].Description = ""
	}
	contract, err := jsoninput.Encode(identity)
	if err != nil {
		return nil, err
	}
	return &Capability{descriptor: descriptor, contract: contract}, nil
}

func cloneCapability(descriptor CapabilityDescriptor) CapabilityDescriptor {
	descriptor.Methods = slices.Clone(descriptor.Methods)
	for i := range descriptor.Methods {
		descriptor.Methods[i] = cloneDescriptor(descriptor.Methods[i])
	}
	return descriptor
}

func (c *Capability) Descriptor() CapabilityDescriptor { return cloneCapability(c.descriptor) }
func (c *Capability) Contract() jsoninput.Input        { return c.contract }

type CapabilityExport struct {
	Name     string               `json:"name"`
	Contract CapabilityDescriptor `json:"contract"`
}

var exportName = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)

// BindCapability associates the generated consumer contract with existing
// handlers. The Backend remains the sole publication and invocation owner.
func (b *Backend) BindCapability(name string, capability *Capability) error {
	if b == nil || capability == nil || capability.contract.Validate() != nil || !exportName.MatchString(name) {
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
	for _, required := range capability.descriptor.Methods {
		method, exists := b.methods[required.Name]
		contract, err := methodContract(required)
		if err != nil {
			return err
		}
		if !exists || method.contract != contract.Digest() {
			return fmt.Errorf("plugin: capability export %s requires exact method %s", name, required.Name)
		}
	}
	b.capabilities[name] = capability
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
		for _, capability := range b.capabilities {
			if capability.contains(member) {
				return errors.New("plugin: capability member requires its export binding")
			}
		}
		return nil
	}
	capability, exists := b.capabilities[params.Export]
	if !exists || capability.contract.Digest() != params.Capability {
		return errors.New("plugin: capability export contract mismatch")
	}
	if !capability.contains(member) {
		return errors.New("plugin: method is not in the capability export")
	}
	return nil
}

func (c *Capability) contains(name string) bool {
	return slices.ContainsFunc(c.descriptor.Methods, func(method MethodDescriptor) bool { return method.Name == name })
}

type CapabilityBinding struct {
	session    *sdk.ClientSession
	export     string
	capability *Capability
}

// NewCapabilityBinding restores the originally captured contract and export.
// The backend checks both before executing; restoration never discovers a new
// provider. Installation/release retention belongs to the host routing owner.
func NewCapabilityBinding(session *sdk.ClientSession, name string, capability *Capability) (*CapabilityBinding, error) {
	if session == nil || capability == nil || capability.contract.Validate() != nil || !exportName.MatchString(name) {
		return nil, errors.New("plugin: session, capability and valid export name are required")
	}
	return &CapabilityBinding{session: session, export: name, capability: capability}, nil
}

func ResolveCapability(ctx context.Context, session *sdk.ClientSession, name string, required *Capability) (*CapabilityBinding, error) {
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
		method, err := admitDescriptor(descriptor)
		if err != nil {
			return nil, err
		}
		methods[descriptor.Name] = method.contract
	}
	seen := make(map[string]bool, len(description.Capabilities))
	found := false
	for _, published := range description.Capabilities {
		if !exportName.MatchString(published.Name) || seen[published.Name] {
			return nil, errors.New("plugin: invalid or duplicate published capability export")
		}
		seen[published.Name] = true
		if published.Name != name {
			continue
		}
		actual, err := admitCapability(published.Contract)
		if err != nil {
			return nil, err
		}
		if actual.contract != required.contract {
			return nil, errors.New("plugin: capability contract mismatch")
		}
		for _, method := range actual.descriptor.Methods {
			contract, err := methodContract(method)
			if err != nil {
				return nil, err
			}
			if methods[method.Name] != contract {
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
	for _, method := range b.capability.descriptor.Methods {
		if method.Name != name {
			continue
		}
		binding, err := NewBinding(b.session, method)
		if err != nil {
			return nil, err
		}
		binding.export = b.export
		binding.capability = b.capability.contract.Digest()
		return binding, nil
	}
	return nil, errors.New("plugin: method is not in the capability contract")
}
