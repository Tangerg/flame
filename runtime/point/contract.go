// Package point owns consumer-defined contribution values and composition.
// Callable interfaces remain with capability; a value may reference one.
package point

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/jsoninput"
	"github.com/Tangerg/scope/core/jsonschema"
)

type Ordering string

const (
	CanonicalKeys Ordering = "canonicalKeys"
	ExplicitList  Ordering = "explicitList"
)

func (Ordering) JSONSchemaEnum() []string {
	return []string{string(CanonicalKeys), string(ExplicitList)}
}

type Definition struct {
	Protocol      string                 `json:"protocol"`
	Reference     string                 `json:"reference"`
	Composition   capability.Composition `json:"composition"`
	Scope         string                 `json:"scope"`
	Optional      bool                   `json:"optional"`
	Ordering      Ordering               `json:"ordering"`
	Value         jsontext.Value         `json:"value"`
	Applicability jsontext.Value         `json:"applicability"`
}

func (d Definition) Clone() Definition {
	d.Value = slices.Clone(d.Value)
	d.Applicability = slices.Clone(d.Applicability)
	return d
}

type Contract struct {
	descriptor    Definition
	identity      jsoninput.Input
	value         jsonschema.Schema
	applicability jsonschema.Schema
}

func Parse(data []byte) (*Contract, error) {
	var descriptor Definition
	if err := json.Unmarshal(data, &descriptor, json.RejectUnknownMembers(true)); err != nil {
		return nil, err
	}
	return Admit(descriptor)
}

func Admit(descriptor Definition) (*Contract, error) {
	valid := func(value string) bool {
		return value != "" && len(value) <= 256 && utf8.ValidString(value) && !strings.ContainsFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
	}
	if descriptor.Protocol != capability.Protocol || !valid(descriptor.Reference) || !valid(descriptor.Scope) {
		return nil, errors.New("point: current protocol, reference and scope are required")
	}
	if descriptor.Composition != capability.Collection && descriptor.Composition != capability.Selection && descriptor.Composition != capability.Decoration {
		return nil, errors.New("point: unknown composition")
	}
	if descriptor.Ordering != CanonicalKeys && descriptor.Ordering != ExplicitList || descriptor.Composition != capability.Collection && descriptor.Ordering != ExplicitList {
		return nil, errors.New("point: selection and decoration require explicit composition")
	}
	descriptor = descriptor.Clone()
	value, err := jsonschema.Parse(descriptor.Value)
	if err != nil {
		return nil, fmt.Errorf("point: value schema: %w", err)
	}
	applicability, err := jsonschema.Parse(descriptor.Applicability)
	if err != nil {
		return nil, fmt.Errorf("point: applicability schema: %w", err)
	}
	identity, err := jsoninput.Encode(descriptor)
	if err != nil {
		return nil, err
	}
	return &Contract{descriptor: descriptor, identity: identity, value: value, applicability: applicability}, nil
}

func (c *Contract) Definition() Definition    { return c.descriptor.Clone() }
func (c *Contract) Identity() jsoninput.Input { return c.identity }
func (c *Contract) ValidateValue(value jsoninput.Input) error {
	if err := value.Validate(); err != nil {
		return err
	}
	return c.value.Validate(value.JSON())
}

// Context is a frozen caller projection. Applicability cannot discover or mutate
// a screen, workspace or business owner while resolving contributions.
type Context struct {
	Target     capability.Target `json:"target"`
	Attributes jsontext.Value    `json:"attributes"`
}

func (c *Contract) Applicable(context Context) (bool, error) {
	if err := context.Target.Validate(); err != nil {
		return false, err
	}
	if context.Target.Scope != c.descriptor.Scope {
		return false, errors.New("point: target scope differs from its contract")
	}
	attributes, err := jsoninput.Parse(context.Attributes)
	if err != nil {
		return false, err
	}
	context.Attributes = attributes.JSON()
	input, err := jsoninput.Encode(context)
	if err != nil {
		return false, err
	}
	return c.applicability.Validate(input.JSON()) == nil, nil
}

type Value struct {
	Key  string
	Data jsoninput.Input
}

// Compose returns a detached, frozen value projection. Explicit decoration
// order is outermost first; it never implies that wrappers have executed.
func (c *Contract) Compose(values []Value, order []string) ([]Value, error) {
	byKey := make(map[string]Value, len(values))
	for _, value := range values {
		if value.Key == "" || len(value.Key) > 512 {
			return nil, errors.New("point: canonical contribution key is required")
		}
		if _, exists := byKey[value.Key]; exists {
			return nil, fmt.Errorf("point: duplicate contribution %s", value.Key)
		}
		if err := c.ValidateValue(value.Data); err != nil {
			return nil, fmt.Errorf("point: contribution %s: %w", value.Key, err)
		}
		byKey[value.Key] = value
	}
	if len(values) == 0 && !c.descriptor.Optional {
		return nil, errors.New("point: required point has no applicable contributions")
	}
	if c.descriptor.Ordering == CanonicalKeys {
		if len(order) > 0 {
			return nil, errors.New("point: canonical collection rejects an explicit order")
		}
		result := slices.Clone(values)
		slices.SortFunc(result, func(a, b Value) int { return strings.Compare(a.Key, b.Key) })
		return result, nil
	}
	if c.descriptor.Composition == capability.Selection && len(order) > 1 {
		return nil, errors.New("point: selection requires one explicit contribution")
	}
	result := make([]Value, 0, len(order))
	seen := make(map[string]bool, len(order))
	for _, key := range order {
		value, found := byKey[key]
		if !found || seen[key] {
			return nil, fmt.Errorf("point: unknown or duplicate ordered contribution %s", key)
		}
		seen[key] = true
		result = append(result, value)
	}
	if c.descriptor.Composition == capability.Collection && len(result) != len(values) {
		return nil, errors.New("point: explicit order omits an applicable contribution")
	}
	if len(result) == 0 && !c.descriptor.Optional {
		return nil, errors.New("point: required point needs an explicit composition")
	}
	return result, nil
}
