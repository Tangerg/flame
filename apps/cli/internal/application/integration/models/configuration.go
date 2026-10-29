// Package models defines provider and auxiliary-model configuration as
// consumer-owned values. Secrets remain write-only and never enter Provider.
package models

import (
	"errors"
	"fmt"
	"strings"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
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
	runtimeDefaultRole roleMode = iota + 1
	disabledRole
	configuredRole
)

type Role struct {
	kind      RoleKind
	mode      roleMode
	selection flamemodel.Selection
}

func RuntimeDefaultUtilityRole() Role {
	return Role{kind: UtilityRole, mode: runtimeDefaultRole}
}

func DisabledEmbeddingRole() Role {
	return Role{kind: EmbeddingRole, mode: disabledRole}
}

func NewConfiguredRole(kind RoleKind, provider, model string) (Role, error) {
	selection, err := flamemodel.New(provider, model)
	if err != nil {
		return Role{}, fmt.Errorf("configured model role: %w", err)
	}
	role := Role{kind: kind, mode: configuredRole, selection: selection}
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
	case runtimeDefaultRole, disabledRole:
	case configuredRole:
		if err := r.selection.ValidateExact(); err != nil {
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
	return r.selection.Provider(), r.selection.Model(), r.mode == configuredRole
}

func (r Role) Label() (string, error) {
	if err := r.Validate(); err != nil {
		return "", err
	}
	switch r.mode {
	case runtimeDefaultRole:
		return "Runtime default", nil
	case disabledRole:
		return "disabled", nil
	case configuredRole:
		return r.selection.String(), nil
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

type Credential struct {
	masked string
	source protocol.ProviderKeySource
}

func NewCredential(masked string, source protocol.ProviderKeySource) (Credential, error) {
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
	if c.source != protocol.ProviderKeySourceStored && c.source != protocol.ProviderKeySourceEnv {
		return fmt.Errorf("provider credential source %q is invalid", c.source)
	}
	return nil
}

func (c Credential) Masked() string { return c.masked }
func (c Credential) FromEnvironment() bool {
	return c.source == protocol.ProviderKeySourceEnv
}
func (c Credential) Stored() bool { return c.source == protocol.ProviderKeySourceStored }

type ProviderSpec struct {
	ID               string
	BaseURL          *string
	Credential       *Credential
	Configured       bool
	RequiresBaseURL  bool
	EmbeddingCapable bool
}

type Provider struct {
	id               string
	baseURL          string
	credential       Credential
	configured       bool
	requiresBaseURL  bool
	embeddingCapable bool
}

func NewProvider(spec ProviderSpec) (Provider, error) {
	provider := Provider{
		id: spec.ID, configured: spec.Configured,
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
	return provider, nil
}

func (p Provider) Validate() error {
	if err := flamemodel.ValidateProviderIdentity(p.id); err != nil {
		return err
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

// Configured is Runtime's readiness verdict. Runtime alone knows every input
// to it, so the CLI reports the verdict instead of re-deriving it from the
// credential and endpoint it happens to see.
func (p Provider) Configured() bool { return p.configured }

func (p Provider) RequiresBaseURL() bool { return p.requiresBaseURL }

func (p Provider) EmbeddingCapable() bool { return p.embeddingCapable }

func (p Provider) Credential() (Credential, bool) {
	return p.credential, p.credential != (Credential{})
}

func (p Provider) BaseURL() (string, bool) { return p.baseURL, p.baseURL != "" }

type ValueChange struct {
	Kind  protocol.ProviderConfigChangeType
	Value string
}

func (v ValueChange) Validate() error {
	switch v.Kind {
	case protocol.ProviderConfigSet:
		if strings.TrimSpace(v.Value) == "" {
			return errors.New("set change value is empty")
		}
	case protocol.ProviderConfigClear:
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
	if err := flamemodel.ValidateProviderIdentity(u.Provider); err != nil {
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
