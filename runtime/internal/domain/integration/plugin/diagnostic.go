package plugin

import (
	"fmt"
	"unicode/utf8"
)

type Diagnostic struct {
	Component Component
	Code      DiagnosticCode
}

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
