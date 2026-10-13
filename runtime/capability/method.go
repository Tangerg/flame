// Package capability owns immutable callable contracts without transport or
// activation. Local metadata and live bindings use the same invariants.
package capability

import (
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/jsoninput"
	"github.com/Tangerg/scope/core/jsonschema"
)

// Protocol identifies public wire behavior, independently of private releases.
// Only the current contract is admitted; a source relocation does not change it.
const Protocol = "io.github.tangerg.flame/backend/2026-10-14.1"

type MethodKind string

const (
	Query    MethodKind = "query"
	Mutation MethodKind = "command"
)

type MethodDescriptor struct {
	Name        string         `json:"name"`
	Kind        MethodKind     `json:"kind"`
	Target      string         `json:"target"`
	Description string         `json:"description"`
	Input       jsontext.Value `json:"input"`
	Output      jsontext.Value `json:"output"`
}

func (d MethodDescriptor) Clone() MethodDescriptor {
	d.Input = append(jsontext.Value(nil), d.Input...)
	d.Output = append(jsontext.Value(nil), d.Output...)
	return d
}

type Method struct {
	descriptor MethodDescriptor
	identity   jsoninput.Input
	input      jsonschema.Schema
	output     jsonschema.Schema
}

func validToken(value string) bool {
	return value != "" && len(value) <= 256 && utf8.ValidString(value) && !strings.ContainsFunc(value, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) })
}

func AdmitMethod(descriptor MethodDescriptor) (*Method, error) {
	if !validToken(descriptor.Name) || !validToken(descriptor.Target) {
		return nil, errors.New("capability: method and target are required bounded tokens")
	}
	if descriptor.Kind != Query && descriptor.Kind != Mutation {
		return nil, errors.New("capability: method kind must be query or command")
	}
	descriptor = descriptor.Clone()
	input, err := jsonschema.Parse(descriptor.Input)
	if err != nil {
		return nil, fmt.Errorf("capability: %s input contract: %w", descriptor.Name, err)
	}
	if input.Validate([]byte("null")) == nil {
		return nil, errors.New("capability: method input must exclude null")
	}
	output, err := jsonschema.Parse(descriptor.Output)
	if err != nil {
		return nil, fmt.Errorf("capability: %s output contract: %w", descriptor.Name, err)
	}
	// Presentation prose does not change callable identity. Exact schemas do.
	identity := descriptor.Clone()
	identity.Description = ""
	canonical, err := jsoninput.Encode(identity)
	if err != nil {
		return nil, err
	}
	return &Method{descriptor: descriptor, identity: canonical, input: input, output: output}, nil
}

func (m *Method) Name() string                 { return m.descriptor.Name }
func (m *Method) Kind() MethodKind             { return m.descriptor.Kind }
func (m *Method) Descriptor() MethodDescriptor { return m.descriptor.Clone() }
func (m *Method) Identity() jsoninput.Input    { return m.identity }

func (m *Method) ValidateInput(input jsoninput.Input) error {
	if err := input.Validate(); err != nil {
		return err
	}
	return m.input.Validate(input.JSON())
}

func (m *Method) ValidateOutput(output jsontext.Value) error {
	return m.output.Validate(output)
}
