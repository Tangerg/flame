package contract

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/Tangerg/frame/portable"
	"github.com/Tangerg/frame/value"
)

const Protocol = "io.github.tangerg.flame/plugin/1"

type Composition string

const (
	Collection Composition = "collection"
	Selection  Composition = "selection"
	Decoration Composition = "decoration"
)

func (c Composition) Validate() error {
	switch c {
	case Collection, Selection, Decoration:
		return nil
	default:
		return errors.New("plugin contract: unknown composition")
	}
}

type CapabilityConfig struct {
	Reference   string
	Composition Composition
	Scope       string
}

type CapabilityDescriptor struct {
	Protocol    string                      `json:"protocol"`
	Reference   string                      `json:"reference"`
	Composition Composition                 `json:"composition"`
	Scope       string                      `json:"scope"`
	Methods     []portable.MethodDescriptor `json:"methods"`
}

// Frame owns callable service identity. Flame's scope and selection policy are
// product metadata and cannot change that service's schema or implementation.
type Capability struct {
	service     *portable.Capability
	composition Composition
	scope       string
	identity    value.JSON
}

func NewCapability(config CapabilityConfig, methods ...portable.MethodDescriptor) (*Capability, error) {
	return AdmitCapability(CapabilityDescriptor{Protocol: Protocol, Reference: config.Reference, Composition: config.Composition, Scope: config.Scope, Methods: methods})
}

func ParseCapability(data []byte) (*Capability, error) {
	if err := portable.CheckDescriptor(data); err != nil {
		return nil, err
	}
	var descriptor CapabilityDescriptor
	if err := value.DecodeValue(data, &descriptor, "plugin contract"); err != nil {
		return nil, err
	}
	return AdmitCapability(descriptor)
}

func AdmitCapability(descriptor CapabilityDescriptor) (*Capability, error) {
	if descriptor.Protocol != Protocol || !portable.ValidToken(descriptor.Scope) {
		return nil, errors.New("plugin contract: current protocol and target scope are required")
	}
	if err := descriptor.Composition.Validate(); err != nil {
		return nil, err
	}
	for _, method := range descriptor.Methods {
		if !portable.ValidToken(method.Target) {
			return nil, errors.New("plugin contract: each method requires a concrete target scope")
		}
	}
	service, err := portable.NewCapability(descriptor.Reference, descriptor.Methods...)
	if err != nil {
		return nil, err
	}
	capability := &Capability{service: service, composition: descriptor.Composition, scope: descriptor.Scope}
	identity := capability.Descriptor()
	for i := range identity.Methods {
		identity.Methods[i].Description = ""
	}
	capability.identity, err = value.EncodeJSON(identity)
	if err != nil {
		return nil, err
	}
	if err := portable.CheckDescriptor(capability.identity.Bytes()); err != nil {
		return nil, err
	}
	return capability, nil
}

func (c *Capability) Descriptor() CapabilityDescriptor {
	service := c.service.Descriptor()
	return CapabilityDescriptor{Protocol: Protocol, Reference: service.Reference, Composition: c.composition, Scope: c.scope, Methods: service.Methods}
}

func (c *Capability) Identity() value.JSON { return c.identity }
func (c *Capability) Service() *portable.Capability {
	if c.service == nil {
		return nil
	}
	return new(*c.service)
}

func (c *Capability) Method(name string) (*portable.Method, bool) {
	if c == nil || c.service == nil {
		return nil, false
	}
	return c.service.Method(name)
}

func MethodKindFor(encoded []byte, name string) (portable.MethodKind, error) {
	capability, err := ParseCapability(encoded)
	if err != nil {
		return "", err
	}
	method, found := capability.Method(name)
	if !found {
		return "", fmt.Errorf("plugin contract: unknown method %q", name)
	}
	return method.Kind(), nil
}

var exportName = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,63}$`)

func ValidExportName(name string) bool { return exportName.MatchString(name) }
