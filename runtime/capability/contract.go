package capability

import (
	json "encoding/json/v2"
	"errors"
	"slices"
	"strings"

	"github.com/Tangerg/flame/runtime/jsoninput"
)

type Composition string

const (
	Collection Composition = "collection"
	Selection  Composition = "selection"
	Decoration Composition = "decoration"
)

type Config struct {
	Reference   string
	Composition Composition
	Scope       string
}

type Descriptor struct {
	Protocol    string             `json:"protocol"`
	Reference   string             `json:"reference"`
	Composition Composition        `json:"composition"`
	Scope       string             `json:"scope"`
	Methods     []MethodDescriptor `json:"methods"`
}

// Contract owns the consumer's required semantics. Producer identity, execution
// location and availability belong to the concrete contribution and binding.
type Contract struct {
	config   Config
	methods  []*Method
	identity jsoninput.Input
}

func New(config Config, methods ...MethodDescriptor) (*Contract, error) {
	return FromDescriptor(Descriptor{Protocol: Protocol, Reference: config.Reference, Composition: config.Composition, Scope: config.Scope, Methods: methods})
}

func Parse(data []byte) (*Contract, error) {
	var descriptor Descriptor
	if err := json.Unmarshal(data, &descriptor, json.RejectUnknownMembers(true)); err != nil {
		return nil, err
	}
	return FromDescriptor(descriptor)
}

func FromDescriptor(descriptor Descriptor) (*Contract, error) {
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
	contract := &Contract{config: Config{Reference: descriptor.Reference, Composition: descriptor.Composition, Scope: descriptor.Scope}}
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
	canonical, err := jsoninput.Encode(identity)
	if err != nil {
		return nil, err
	}
	contract.identity = canonical
	return contract, nil
}

func (c *Contract) Descriptor() Descriptor {
	descriptor := Descriptor{Protocol: Protocol, Reference: c.config.Reference, Composition: c.config.Composition, Scope: c.config.Scope, Methods: make([]MethodDescriptor, len(c.methods))}
	for i, method := range c.methods {
		descriptor.Methods[i] = method.Descriptor()
	}
	return descriptor
}

func (c *Contract) Identity() jsoninput.Input { return c.identity }

func (c *Contract) Method(name string) (*Method, bool) {
	for _, method := range c.methods {
		if method.Name() == name {
			return method, true
		}
	}
	return nil, false
}
