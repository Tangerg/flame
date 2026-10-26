package extension

import (
	json "encoding/json/v2"
	"errors"
	"slices"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/jsonvalue"
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

type CapabilityDescriptor struct {
	Protocol    string             `json:"protocol"`
	Reference   string             `json:"reference"`
	Composition Composition        `json:"composition"`
	Scope       string             `json:"scope"`
	Methods     []MethodDescriptor `json:"methods"`
}

// Producer identity, execution location and availability cannot change the
// consumer's required semantics; those facts belong to the concrete binding.
type Capability struct {
	config   CapabilityConfig
	methods  []*Method
	identity jsonvalue.Value
}

func NewCapability(config CapabilityConfig, methods ...MethodDescriptor) (*Capability, error) {
	return AdmitCapability(CapabilityDescriptor{Protocol: Protocol, Reference: config.Reference, Composition: config.Composition, Scope: config.Scope, Methods: methods})
}

func ParseCapability(data []byte) (*Capability, error) {
	var descriptor CapabilityDescriptor
	if err := json.Unmarshal(data, &descriptor, json.RejectUnknownMembers(true)); err != nil {
		return nil, err
	}
	return AdmitCapability(descriptor)
}

func AdmitCapability(descriptor CapabilityDescriptor) (*Capability, error) {
	if descriptor.Protocol != Protocol {
		return nil, errors.New("capability: current protocol is required")
	}
	if !validToken(descriptor.Reference) || !validToken(descriptor.Scope) {
		return nil, errors.New("capability: reference and scope are required bounded tokens")
	}
	if descriptor.Composition != Collection && descriptor.Composition != Selection && descriptor.Composition != Decoration {
		return nil, errors.New("capability: unknown composition")
	}
	if len(descriptor.Methods) == 0 {
		return nil, errors.New("capability: required methods are missing")
	}
	contract := &Capability{config: CapabilityConfig{Reference: descriptor.Reference, Composition: descriptor.Composition, Scope: descriptor.Scope}}
	for _, descriptor := range descriptor.Methods {
		method, err := AdmitMethod(descriptor)
		if err != nil {
			return nil, err
		}
		contract.methods = append(contract.methods, method)
	}
	slices.SortFunc(contract.methods, func(a, b *Method) int { return strings.Compare(a.Name(), b.Name()) })
	for i := 1; i < len(contract.methods); i++ {
		if contract.methods[i-1].Name() == contract.methods[i].Name() {
			return nil, errors.New("capability: duplicate required method")
		}
	}
	identity := contract.Descriptor()
	for i := range identity.Methods {
		identity.Methods[i].Description = ""
	}
	canonical, err := jsonvalue.Encode(identity)
	if err != nil {
		return nil, err
	}
	contract.identity = canonical
	return contract, nil
}

func (c *Capability) Descriptor() CapabilityDescriptor {
	descriptor := CapabilityDescriptor{Protocol: Protocol, Reference: c.config.Reference, Composition: c.config.Composition, Scope: c.config.Scope, Methods: make([]MethodDescriptor, len(c.methods))}
	for i, method := range c.methods {
		descriptor.Methods[i] = method.Descriptor()
	}
	return descriptor
}

func (c *Capability) Identity() jsonvalue.Value { return c.identity }

func (c *Capability) Method(name string) (*Method, bool) {
	for _, method := range c.methods {
		if method.Name() == name {
			return method, true
		}
	}
	return nil, false
}
