package plugin

import (
	"fmt"
	"unicode/utf8"
)

// Diagnostic is one admission finding about package bytes. Findings are
// derived once from an immutable release and travel with its declaration;
// they report what admission isolated and never decide availability.
type Diagnostic struct {
	Component Component
	Code      DiagnosticCode
}

// Component locates a finding in the portable package. Kinds that address an
// authored member carry its exact authored name; the others carry none.
type Component struct {
	Kind ComponentKind
	Name string
}

type ComponentKind string

const (
	ComponentManifestField  ComponentKind = "manifestField"
	ComponentFlameExtension ComponentKind = "flameExtension"
	ComponentExtensionField ComponentKind = "extensionField"
	ComponentContribution   ComponentKind = "contribution"
	ComponentMCP            ComponentKind = "mcp"
	ComponentMCPServer      ComponentKind = "mcpServer"
	ComponentSkills         ComponentKind = "skills"
	ComponentSkill          ComponentKind = "skill"
)

// Named reports whether a component of this kind carries the exact authored
// name of the member it addresses.
func (k ComponentKind) Named() (bool, error) {
	switch k {
	case ComponentManifestField, ComponentExtensionField, ComponentContribution, ComponentMCPServer, ComponentSkill:
		return true, nil
	case ComponentFlameExtension, ComponentMCP, ComponentSkills:
		return false, nil
	default:
		return false, fmt.Errorf("%w: diagnostic component kind %q", ErrInvalid, k)
	}
}

type DiagnosticCode string

const (
	DiagnosticUnknownField            DiagnosticCode = "unknownField"
	DiagnosticInvalidDeclaration      DiagnosticCode = "invalidDeclaration"
	DiagnosticUnsupportedContribution DiagnosticCode = "unsupportedContribution"
	DiagnosticComponentLimit          DiagnosticCode = "componentLimit"
	DiagnosticInvalidDependencies     DiagnosticCode = "invalidDependencies"
	DiagnosticUnavailableComponent    DiagnosticCode = "unavailableComponent"
)

func (c DiagnosticCode) validate() error {
	switch c {
	case DiagnosticUnknownField, DiagnosticInvalidDeclaration, DiagnosticUnsupportedContribution,
		DiagnosticComponentLimit, DiagnosticInvalidDependencies, DiagnosticUnavailableComponent:
		return nil
	default:
		return fmt.Errorf("%w: diagnostic code %q", ErrInvalid, c)
	}
}

// Refusal is why a [Builder] refused one contribution, named by the
// diagnostic a package adapter reports for it. It wraps [ErrInvalid].
type Refusal struct {
	Code  DiagnosticCode
	cause error
}

func refuse(code DiagnosticCode, format string, args ...any) error {
	return &Refusal{Code: code, cause: fmt.Errorf("%w: "+format, append([]any{ErrInvalid}, args...)...)}
}

func (r *Refusal) Error() string { return r.cause.Error() }
func (r *Refusal) Unwrap() error { return r.cause }

func (d Diagnostic) Validate() error {
	if err := d.Code.validate(); err != nil {
		return err
	}
	named, err := d.Component.Kind.Named()
	if err != nil {
		return err
	}
	if !named && d.Component.Name != "" {
		return fmt.Errorf("%w: diagnostic component %q carries a name", ErrInvalid, d.Component.Kind)
	}
	if !utf8.ValidString(d.Component.Name) {
		return fmt.Errorf("%w: diagnostic component %q name encoding", ErrInvalid, d.Component.Kind)
	}
	return nil
}
