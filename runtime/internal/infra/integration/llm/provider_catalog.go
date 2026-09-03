package llm

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"unicode"

	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
)

type modelSource uint8

const (
	modelSourceBundled modelSource = iota + 1
	modelSourceEndpoint
)

type modelListProtocol uint8

const (
	modelListProtocolOpenAI modelListProtocol = iota + 1
	modelListProtocolAnthropic
)

type modelPolicy struct {
	source       modelSource
	defaultModel string
	listProtocol modelListProtocol
}

func bundledModels(defaultModel string) modelPolicy {
	return modelPolicy{source: modelSourceBundled, defaultModel: defaultModel}
}

func openAIEndpointModels() modelPolicy {
	return modelPolicy{source: modelSourceEndpoint, listProtocol: modelListProtocolOpenAI}
}

func anthropicEndpointModels() modelPolicy {
	return modelPolicy{source: modelSourceEndpoint, listProtocol: modelListProtocolAnthropic}
}

func (p modelPolicy) validate() error {
	switch p.source {
	case modelSourceBundled:
		if p.defaultModel == "" {
			return fmt.Errorf("bundled model policy requires a default model")
		}
		if p.listProtocol != 0 {
			return fmt.Errorf("bundled model policy cannot carry a listing protocol")
		}
		if _, err := modelref.NewModelIdentity(p.defaultModel); err != nil {
			return fmt.Errorf("bundled model policy model identity: %w", err)
		}
	case modelSourceEndpoint:
		if p.defaultModel != "" {
			return fmt.Errorf("endpoint model policy cannot carry a bundled default")
		}
		if p.listProtocol != modelListProtocolOpenAI && p.listProtocol != modelListProtocolAnthropic {
			return fmt.Errorf("endpoint model policy requires a listing protocol")
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

func (p modelPolicy) list(ctx context.Context, baseURL, apiKey string) ([]string, error) {
	if !p.discoveredAtEndpoint() {
		return nil, fmt.Errorf("bundled model policy does not support remote listing")
	}
	return listRemoteModels(ctx, baseURL, apiKey, p.listProtocol)
}

type endpointKind uint8

const (
	endpointOwnedByAdapter endpointKind = iota + 1
	endpointHasCatalogDefault
	endpointMustBeConfigured
)

type endpointPolicy struct {
	kind       endpointKind
	defaultURL string
}

func adapterEndpoint() endpointPolicy {
	return endpointPolicy{kind: endpointOwnedByAdapter}
}

func catalogEndpoint(defaultURL string) endpointPolicy {
	return endpointPolicy{kind: endpointHasCatalogDefault, defaultURL: defaultURL}
}

func configuredEndpoint() endpointPolicy {
	return endpointPolicy{kind: endpointMustBeConfigured}
}

func (p endpointPolicy) validate() error {
	switch p.kind {
	case endpointOwnedByAdapter, endpointMustBeConfigured:
		if p.defaultURL != "" {
			return fmt.Errorf("endpoint policy %d cannot carry a default URL", p.kind)
		}
	case endpointHasCatalogDefault:
		if err := validateCatalogBaseURL(p.defaultURL); err != nil {
			return fmt.Errorf("catalog default endpoint: %w", err)
		}
	default:
		return fmt.Errorf("unknown endpoint policy %d", p.kind)
	}
	return nil
}

func (p endpointPolicy) requiresConfiguration() bool {
	return p.kind == endpointMustBeConfigured
}

func (p endpointPolicy) defaultValue() (string, bool) {
	if p.kind != endpointHasCatalogDefault {
		return "", false
	}
	return p.defaultURL, true
}

func (p endpointPolicy) resolve(configured clientEndpoint) (clientEndpoint, error) {
	if configured.configured() {
		return configured, nil
	}
	switch p.kind {
	case endpointOwnedByAdapter:
		return noClientEndpoint(), nil
	case endpointHasCatalogDefault:
		return configuredClientEndpoint(p.defaultURL)
	case endpointMustBeConfigured:
		return clientEndpoint{}, fmt.Errorf("a base URL must be configured")
	default:
		return clientEndpoint{}, fmt.Errorf("unknown endpoint policy %d", p.kind)
	}
}

func validateCatalogBaseURL(raw string) error {
	if strings.TrimSpace(raw) == "" || raw != strings.TrimSpace(raw) {
		return fmt.Errorf("URL must be non-blank and canonical")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("URL must use http or https and include a host")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("URL cannot contain user info or a fragment")
	}
	return nil
}

type credentialRequirement uint8

const (
	credentialRequired credentialRequirement = iota + 1
	credentialOptional
)

type credentialPolicy struct {
	requirement credentialRequirement
	environment string
}

func requiredCredential(environment string) credentialPolicy {
	return credentialPolicy{requirement: credentialRequired, environment: environment}
}

func optionalCredential(environment string) credentialPolicy {
	return credentialPolicy{requirement: credentialOptional, environment: environment}
}

func (p credentialPolicy) validate() error {
	if p.requirement != credentialRequired && p.requirement != credentialOptional {
		return fmt.Errorf("unknown credential requirement %d", p.requirement)
	}
	if p.environment == "" {
		return fmt.Errorf("credential environment variable is empty")
	}
	for index, character := range p.environment {
		if character == '_' || unicode.IsUpper(character) || (index > 0 && unicode.IsDigit(character)) {
			continue
		}
		return fmt.Errorf("credential environment variable %q is not canonical", p.environment)
	}
	return nil
}

func (p credentialPolicy) required() bool { return p.requirement == credentialRequired }

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
		chatModels: openAIEndpointModels(), chatBuilder: builder,
	}
}

func optionalCredentialEndpointProvider(id Provider, endpoint endpointPolicy, credentialEnvironmentName string, builder buildFunc) providerProfile {
	profile := endpointProvider(id, endpoint, credentialEnvironmentName, builder)
	profile.credential = optionalCredential(credentialEnvironmentName)
	return profile
}

func (p providerProfile) withEmbedding(models modelPolicy, builder embeddingBuildFunc) providerProfile {
	profile := embeddingProviderProfile{models: models, build: builder}
	p.embedding = &profile
	return p
}

func (p providerProfile) withChatModels(models modelPolicy) providerProfile {
	p.chatModels = models
	return p
}

func (p providerProfile) validate() error {
	if _, err := modelref.NewProviderIdentity(string(p.id)); err != nil {
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
	if p.chatModels.discoveredAtEndpoint() && p.endpoint.kind == endpointOwnedByAdapter {
		return fmt.Errorf("endpoint-discovered models require a resolvable endpoint policy")
	}
	if p.chatBuilder == nil {
		return fmt.Errorf("chat builder is nil")
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
		panic("llm: invalid provider catalog: " + err.Error())
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
