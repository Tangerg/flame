// Package modelconfig defines provider and auxiliary-model configuration as
// consumer-owned values. Secrets remain write-only and never enter Provider.
package modelconfig

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Tangerg/flame/cli/internal/failure"
	"github.com/Tangerg/flame/cli/internal/modelidentity"
)

type RoleKind string

const (
	UtilityRole   RoleKind = "utility"
	EmbeddingRole RoleKind = "embedding"
)

func (r RoleKind) Validate() error {
	if r != UtilityRole && r != EmbeddingRole {
		return fmt.Errorf("model role kind %q is invalid", r)
	}
	return nil
}

type roleMode uint8

const (
	inheritedRole roleMode = iota + 1
	disabledRole
	configuredRole
)

type Role struct {
	kind     RoleKind
	mode     roleMode
	provider string
	model    string
}

func InheritedUtilityRole() Role {
	return Role{kind: UtilityRole, mode: inheritedRole}
}

func DisabledEmbeddingRole() Role {
	return Role{kind: EmbeddingRole, mode: disabledRole}
}

func NewConfiguredRole(kind RoleKind, provider, model string) (Role, error) {
	role := Role{kind: kind, mode: configuredRole, provider: provider, model: model}
	if err := role.Validate(); err != nil {
		return Role{}, err
	}
	return role, nil
}

func (r Role) Validate() error {
	if err := r.kind.Validate(); err != nil {
		return err
	}
	switch r.mode {
	case inheritedRole:
		if r.kind != UtilityRole || r.provider != "" || r.model != "" {
			return errors.New("only an empty utility role can inherit the run model")
		}
	case disabledRole:
		if r.kind != EmbeddingRole || r.provider != "" || r.model != "" {
			return errors.New("only an empty embedding role can be disabled")
		}
	case configuredRole:
		if err := modelidentity.Selection(r.provider, r.model, ""); err != nil {
			return fmt.Errorf("configured model role: %w", err)
		}
	default:
		return errors.New("model role mode is unknown")
	}
	return nil
}

func (r Role) Kind() RoleKind { return r.kind }

func (r Role) Configured() bool { return r.mode == configuredRole }

func (r Role) ProviderModel() (string, string, bool) {
	return r.provider, r.model, r.mode == configuredRole
}

func (r Role) Label() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	switch r.mode {
	case inheritedRole:
		return "inherit the run model", nil
	case disabledRole:
		return "disabled", nil
	case configuredRole:
		return r.provider + "/" + r.model, nil
	default:
		panic("validated model role mode became unreachable")
	}
}

type Roles struct {
	Utility   Role
	Embedding Role
}

func (r Roles) Validate() error {
	if r.Utility.Kind() != UtilityRole || r.Embedding.Kind() != EmbeddingRole {
		return errors.New("model roles are assigned to the wrong slots")
	}
	if err := r.Utility.Validate(); err != nil {
		return fmt.Errorf("utility role: %w", err)
	}
	if err := r.Embedding.Validate(); err != nil {
		return fmt.Errorf("embedding role: %w", err)
	}
	return nil
}

type KeySource string

const (
	KeyStored           KeySource = "stored"
	KeyEnvironment      KeySource = "env"
	credentialMaskGlyph           = "*"
)

type Credential struct {
	masked string
	source KeySource
}

func NewCredential(masked string, source KeySource) (Credential, error) {
	credential := Credential{masked: masked, source: source}
	if err := credential.Validate(); err != nil {
		return Credential{}, err
	}
	return credential, nil
}

func (c Credential) Validate() error {
	if strings.TrimSpace(c.masked) == "" {
		return errors.New("provider credential mask is empty")
	}
	if c.source != KeyStored && c.source != KeyEnvironment {
		return fmt.Errorf("provider credential source %q is invalid", c.source)
	}
	return nil
}

func (c Credential) Masked() string    { return c.masked }
func (c Credential) Source() KeySource { return c.source }
func (c Credential) FromEnvironment() bool {
	return c.source == KeyEnvironment
}
func (c Credential) Stored() bool { return c.source == KeyStored }

// Exposes reports whether a returned mask reproduces a non-mask credential.
// An all-mask input is indistinguishable from a correctly redacted value and
// contains no information to expose.
func (c Credential) Exposes(raw string) bool {
	return c.masked == raw && strings.Trim(raw, credentialMaskGlyph) != ""
}

type ProviderSpec struct {
	ID                    string
	BaseURL               *string
	Credential            *Credential
	Configured            bool
	CredentialRequirement CredentialRequirement
	RequiresBaseURL       bool
	EmbeddingCapable      bool
}

type CredentialRequirement string

const (
	APIKeyRequired CredentialRequirement = "apiKeyRequired"
	APIKeyOptional CredentialRequirement = "apiKeyOptional"
)

func (r CredentialRequirement) Validate() error {
	if r != APIKeyRequired && r != APIKeyOptional {
		return fmt.Errorf("provider credential requirement %q is invalid", r)
	}
	return nil
}

type Provider struct {
	id                    string
	baseURL               string
	credential            Credential
	credentialRequirement CredentialRequirement
	requiresBaseURL       bool
	embeddingCapable      bool
}

func NewProvider(spec ProviderSpec) (Provider, error) {
	provider := Provider{
		id: spec.ID, credentialRequirement: spec.CredentialRequirement,
		requiresBaseURL: spec.RequiresBaseURL, embeddingCapable: spec.EmbeddingCapable,
	}
	if spec.BaseURL != nil {
		if strings.TrimSpace(*spec.BaseURL) == "" {
			return Provider{}, errors.New("provider base URL is empty")
		}
		provider.baseURL = *spec.BaseURL
	}
	if spec.Credential != nil {
		provider.credential = *spec.Credential
	}
	if err := provider.Validate(); err != nil {
		return Provider{}, err
	}
	if configured := provider.Configured(); configured != spec.Configured {
		return Provider{}, fmt.Errorf(
			"provider %s wire configured state %v contradicts derived readiness %v",
			provider.id,
			spec.Configured,
			configured,
		)
	}
	return provider, nil
}

func (p Provider) Validate() error {
	if err := modelidentity.Provider(p.id); err != nil {
		return err
	}
	if err := p.credentialRequirement.Validate(); err != nil {
		return fmt.Errorf("provider %s: %w", p.id, err)
	}
	if p.baseURL != "" && (p.baseURL != strings.TrimSpace(p.baseURL)) {
		return fmt.Errorf("provider %s has a non-canonical base URL", p.id)
	}
	if p.credential != (Credential{}) {
		if err := p.credential.Validate(); err != nil {
			return fmt.Errorf("provider %s: %w", p.id, err)
		}
	}
	return nil
}

func (p Provider) ID() string { return p.id }

// Configured derives readiness from the same closed credential and endpoint
// policies that validated the provider. The wire's configured flag is checked
// at construction and never becomes a second mutable truth inside the entity.
func (p Provider) Configured() bool {
	credentialReady := p.credentialRequirement == APIKeyOptional ||
		p.credentialRequirement == APIKeyRequired && p.credential != (Credential{})
	endpointReady := !p.requiresBaseURL || p.baseURL != ""
	return credentialReady && endpointReady
}

func (p Provider) RequiresAPIKey() bool { return p.credentialRequirement == APIKeyRequired }

func (p Provider) RequiresBaseURL() bool { return p.requiresBaseURL }

func (p Provider) EmbeddingCapable() bool { return p.embeddingCapable }

func (p Provider) Credential() (Credential, bool) {
	return p.credential, p.credential != (Credential{})
}

func (p Provider) BaseURL() (string, bool) { return p.baseURL, p.baseURL != "" }

type ChangeKind string

const (
	SetValue   ChangeKind = "set"
	ClearValue ChangeKind = "clear"
)

type ValueChange struct {
	Kind  ChangeKind
	Value string
}

func (v ValueChange) Validate() error {
	switch v.Kind {
	case SetValue:
		if strings.TrimSpace(v.Value) == "" {
			return errors.New("set change value is empty")
		}
	case ClearValue:
		if v.Value != "" {
			return errors.New("clear change carries a value")
		}
	default:
		return fmt.Errorf("provider change kind %q is invalid", v.Kind)
	}
	return nil
}

type UpdateProvider struct {
	Provider string
	BaseURL  *ValueChange
	APIKey   *ValueChange
}

func (u UpdateProvider) Validate() error {
	if err := modelidentity.Provider(u.Provider); err != nil {
		return fmt.Errorf("update provider: %w", err)
	}
	if u.BaseURL == nil && u.APIKey == nil {
		return errors.New("update provider has no changes")
	}
	for _, field := range []struct {
		name   string
		change *ValueChange
	}{
		{name: "base URL", change: u.BaseURL},
		{name: "API key", change: u.APIKey},
	} {
		if field.change != nil {
			if err := field.change.Validate(); err != nil {
				return fmt.Errorf("update provider %s: %w", field.name, err)
			}
		}
	}
	return nil
}

type TestResult struct {
	OK      bool
	Problem *failure.Problem
}

func (t TestResult) Validate() error {
	if t.OK == (t.Problem != nil) {
		return errors.New("provider test result must contain exactly one success or problem state")
	}
	if t.Problem != nil {
		return t.Problem.Validate()
	}
	return nil
}

type Service interface {
	Roles(context.Context) (Roles, error)
	SetRole(context.Context, Role) (Role, error)
	Providers(context.Context) ([]Provider, error)
	UpdateProvider(context.Context, UpdateProvider) (Provider, error)
	TestProvider(context.Context, string) (TestResult, error)
}
