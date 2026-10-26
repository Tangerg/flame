package model

import (
	"errors"
	"fmt"
	"slices"
	"unicode"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	providerdomain "github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
)

type modelSource uint8

const (
	modelSourceBundled modelSource = iota + 1
	modelSourceEndpoint
)

type modelPolicy struct {
	source       modelSource
	defaultModel string
}

func bundledModels(defaultModel string) modelPolicy {
	return modelPolicy{source: modelSourceBundled, defaultModel: defaultModel}
}

func endpointModels() modelPolicy { return modelPolicy{source: modelSourceEndpoint} }

func (p modelPolicy) validate() error {
	switch p.source {
	case modelSourceBundled:
		if p.defaultModel == "" {
			return errors.New("bundled model policy requires a default model")
		}
		if _, err := modelchoice.NewModelIdentity(p.defaultModel); err != nil {
			return fmt.Errorf("bundled model policy model identity: %w", err)
		}
	case modelSourceEndpoint:
		if p.defaultModel != "" {
			return errors.New("endpoint model policy cannot carry a bundled default")
		}
	default:
		return fmt.Errorf("unknown model source %d", p.source)
	}
	return nil
}

func (p modelPolicy) defaultValue() (string, bool) {
	if p.source != modelSourceBundled {
		return "", false
	}
	return p.defaultModel, true
}

func (p modelPolicy) discoveredAtEndpoint() bool {
	return p.source == modelSourceEndpoint
}

type endpointKind uint8

const (
	endpointOwnedByAdapter endpointKind = iota + 1
	endpointMustBeConfigured
)

type endpointPolicy struct {
	kind endpointKind
}

func adapterEndpoint() endpointPolicy {
	return endpointPolicy{kind: endpointOwnedByAdapter}
}

func configuredEndpoint() endpointPolicy {
	return endpointPolicy{kind: endpointMustBeConfigured}
}

func (p endpointPolicy) validate() error {
	switch p.kind {
	case endpointOwnedByAdapter, endpointMustBeConfigured:
	default:
		return fmt.Errorf("unknown endpoint policy %d", p.kind)
	}
	return nil
}

func (p endpointPolicy) requiresConfiguration() bool {
	return p.kind == endpointMustBeConfigured
}

func (p endpointPolicy) resolve(configured providerdomain.BaseURL) (providerdomain.BaseURL, error) {
	if configured.Present() {
		return configured, nil
	}
	switch p.kind {
	case endpointOwnedByAdapter:
		return providerdomain.BaseURL{}, nil
	case endpointMustBeConfigured:
		return providerdomain.BaseURL{}, errors.New("a base URL must be configured")
	default:
		return providerdomain.BaseURL{}, fmt.Errorf("unknown endpoint policy %d", p.kind)
	}
}

type credentialPolicy struct {
	environment string
}

func requiredCredential(environment string) credentialPolicy {
	return credentialPolicy{environment: environment}
}

func (p credentialPolicy) validate() error {
	if p.environment == "" {
		return errors.New("credential environment variable is empty")
	}
	for index, character := range p.environment {
		if character == '_' || unicode.IsUpper(character) || (index > 0 && unicode.IsDigit(character)) {
			continue
		}
		return fmt.Errorf("credential environment variable %q is not canonical", p.environment)
	}
	return nil
}

type providerProfile struct {
	id          Provider
	credential  credentialPolicy
	endpoint    endpointPolicy
	chatModels  modelPolicy
	chatBuilder buildFunc
	embedding   *embeddingProviderProfile
}

func bundledProvider(id Provider, defaultModel, credentialEnvironmentName string, builder buildFunc) providerProfile {
	return providerProfile{
		id: id, credential: requiredCredential(credentialEnvironmentName), endpoint: adapterEndpoint(),
		chatModels: bundledModels(defaultModel), chatBuilder: builder,
	}
}

func endpointProvider(id Provider, endpoint endpointPolicy, credentialEnvironmentName string, builder buildFunc) providerProfile {
	return providerProfile{
		id: id, credential: requiredCredential(credentialEnvironmentName), endpoint: endpoint,
		chatModels: endpointModels(), chatBuilder: builder,
	}
}

func (p providerProfile) withEmbedding(models modelPolicy, builder embeddingBuildFunc) providerProfile {
	profile := embeddingProviderProfile{models: models, build: builder}
	p.embedding = &profile
	return p
}

func (p providerProfile) validate() error {
	if _, err := modelchoice.NewProviderIdentity(string(p.id)); err != nil {
		return fmt.Errorf("provider identity: %w", err)
	}
	if err := p.credential.validate(); err != nil {
		return err
	}
	if err := p.endpoint.validate(); err != nil {
		return err
	}
	if err := p.chatModels.validate(); err != nil {
		return fmt.Errorf("chat models: %w", err)
	}
	if p.chatBuilder == nil {
		return errors.New("chat builder is nil")
	}
	if p.embedding != nil {
		if err := p.embedding.validate(); err != nil {
			return fmt.Errorf("embedding: %w", err)
		}
	}
	return nil
}

type providerCatalog struct {
	profiles map[Provider]providerProfile
	ids      []Provider
}

func newProviderCatalog(profiles ...providerProfile) (providerCatalog, error) {
	catalog := providerCatalog{profiles: make(map[Provider]providerProfile, len(profiles)), ids: make([]Provider, 0, len(profiles))}
	for _, profile := range profiles {
		if err := profile.validate(); err != nil {
			return providerCatalog{}, fmt.Errorf("provider %q: %w", profile.id, err)
		}
		if _, exists := catalog.profiles[profile.id]; exists {
			return providerCatalog{}, fmt.Errorf("provider %q is registered more than once", profile.id)
		}
		catalog.profiles[profile.id] = profile
		catalog.ids = append(catalog.ids, profile.id)
	}
	return catalog, nil
}

func mustProviderCatalog(profiles ...providerProfile) providerCatalog {
	catalog, err := newProviderCatalog(profiles...)
	if err != nil {
		panic("model: invalid provider catalog: " + err.Error())
	}
	return catalog
}

func (c providerCatalog) lookup(id Provider) (providerProfile, bool) {
	profile, found := c.profiles[id]
	return profile, found
}

func (c providerCatalog) supported() []Provider {
	return slices.Clone(c.ids)
}
