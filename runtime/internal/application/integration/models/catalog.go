package models

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"

	"github.com/Tangerg/scope/models/catalog"
)

type ProviderAuthenticationPolicy uint8

const (
	ProviderAPIKeyRequired ProviderAuthenticationPolicy = iota + 1
	ProviderAPIKeyOptional
)

func (p ProviderAuthenticationPolicy) valid() bool {
	return p == ProviderAPIKeyRequired || p == ProviderAPIKeyOptional
}

type ProviderEndpointPolicy uint8

const (
	ProviderEndpointOptional ProviderEndpointPolicy = iota + 1
	ProviderEndpointRequired
)

func (p ProviderEndpointPolicy) valid() bool {
	return p == ProviderEndpointOptional || p == ProviderEndpointRequired
}

type ProviderModelSource uint8

const (
	ProviderModelsBundled ProviderModelSource = iota + 1
	ProviderModelsEndpoint
)

func (s ProviderModelSource) valid() bool {
	return s == ProviderModelsBundled || s == ProviderModelsEndpoint
}

type embeddingCapabilityKind uint8

const (
	embeddingUnsupported embeddingCapabilityKind = iota + 1
	embeddingSupportedWithoutDefault
	embeddingSupportedWithDefault
)

type EmbeddingCapability struct {
	kind         embeddingCapabilityKind
	defaultModel flamemodel.ModelIdentity
}

func NoEmbeddingCapability() EmbeddingCapability {
	return EmbeddingCapability{kind: embeddingUnsupported}
}

func EmbeddingCapabilityWithoutDefault() EmbeddingCapability {
	return EmbeddingCapability{kind: embeddingSupportedWithoutDefault}
}

func EmbeddingCapabilityWithDefault(model string) (EmbeddingCapability, error) {
	identity, err := flamemodel.NewModelIdentity(model)
	if err != nil {
		return EmbeddingCapability{}, fmt.Errorf("models: default embedding model: %w", err)
	}
	return EmbeddingCapability{kind: embeddingSupportedWithDefault, defaultModel: identity}, nil
}

func (c EmbeddingCapability) valid() bool {
	switch c.kind {
	case embeddingUnsupported, embeddingSupportedWithoutDefault:
		return c.defaultModel.String() == ""
	case embeddingSupportedWithDefault:
		_, err := flamemodel.NewModelIdentity(c.defaultModel.String())
		return err == nil
	default:
		return false
	}
}

func (c EmbeddingCapability) Supported() bool {
	return c.kind == embeddingSupportedWithoutDefault || c.kind == embeddingSupportedWithDefault
}

func (c EmbeddingCapability) DefaultModel() (string, bool) {
	if c.kind != embeddingSupportedWithDefault {
		return "", false
	}
	return c.defaultModel.String(), true
}

type ProviderMetadata struct {
	id             flamemodel.ProviderIdentity
	authentication ProviderAuthenticationPolicy
	endpoint       ProviderEndpointPolicy
	models         ProviderModelSource
	embedding      EmbeddingCapability
}

func NewProviderMetadata(id string, authentication ProviderAuthenticationPolicy, endpoint ProviderEndpointPolicy, models ProviderModelSource, embedding EmbeddingCapability) (ProviderMetadata, error) {
	identity, err := flamemodel.NewProviderIdentity(id)
	if err != nil {
		return ProviderMetadata{}, fmt.Errorf("models: provider identity: %w", err)
	}
	if !authentication.valid() {
		return ProviderMetadata{}, fmt.Errorf("models: provider %q has an invalid authentication policy", id)
	}
	if !endpoint.valid() {
		return ProviderMetadata{}, fmt.Errorf("models: provider %q has an invalid endpoint policy", id)
	}
	if !models.valid() {
		return ProviderMetadata{}, fmt.Errorf("models: provider %q has an invalid model source", id)
	}
	if !embedding.valid() {
		return ProviderMetadata{}, fmt.Errorf("models: provider %q has an invalid embedding capability", id)
	}
	return ProviderMetadata{id: identity, authentication: authentication, endpoint: endpoint, models: models, embedding: embedding}, nil
}

func (p ProviderMetadata) ID() string { return p.id.String() }

func (p ProviderMetadata) RequiresAPIKey() bool {
	return p.authentication == ProviderAPIKeyRequired
}

func (p ProviderMetadata) RequiresConfiguredEndpoint() bool {
	return p.endpoint == ProviderEndpointRequired
}

func (p ProviderMetadata) DiscoversModelsAtEndpoint() bool {
	return p.models == ProviderModelsEndpoint
}

func (p ProviderMetadata) Embedding() EmbeddingCapability { return p.embedding }

func (p ProviderMetadata) ConfigurationSatisfied(entry provider.Provider) bool {
	if p.RequiresAPIKey() {
		if _, configured := entry.Credential(); !configured {
			return false
		}
	}
	if p.RequiresConfiguredEndpoint() {
		if _, configured := entry.BaseURL(); !configured {
			return false
		}
	}
	return true
}

type Model struct {
	selection flamemodel.Selection
	details   *Details
}

func NewModel(providerID, modelID string, details *Details) (Model, error) {
	selection, err := flamemodel.New(providerID, modelID)
	if err != nil {
		return Model{}, err
	}
	if err := validateDetails(details); err != nil {
		return Model{}, err
	}
	return Model{selection: selection, details: cloneDetails(details)}, nil
}

func (m Model) ID() string        { return m.selection.Model() }
func (m Model) Provider() string  { return m.selection.Provider() }
func (m Model) Details() *Details { return cloneDetails(m.details) }

type Details struct {
	DisplayName      string
	TokenLimits      modelref.TokenLimits
	KnowledgeCutoff  time.Time
	Deprecated       bool
	Reasoning        bool
	ReasoningLevels  []string
	ReasoningDefault string
	Multimodal       bool
	InputModalities  []catalog.Modality
	OutputModalities []catalog.Modality
	ToolUse          bool
	StructuredOutput bool
	Pricing          []PriceBand
}

type PriceBand struct {
	AboveInputTokens     int64
	InputPerMillion      float64
	OutputPerMillion     float64
	CacheReadPerMillion  float64
	CacheWritePerMillion float64
}

func cloneDetails(details *Details) *Details {
	if details == nil {
		return nil
	}
	clone := *details
	clone.ReasoningLevels = slices.Clone(details.ReasoningLevels)
	clone.InputModalities = slices.Clone(details.InputModalities)
	clone.OutputModalities = slices.Clone(details.OutputModalities)
	clone.Pricing = slices.Clone(details.Pricing)
	return &clone
}

func validateDetails(details *Details) error {
	if details == nil {
		return nil
	}
	if err := validateReasoning(details); err != nil {
		return err
	}
	return validatePricing(details.Pricing)
}

func validateReasoning(details *Details) error {
	if !details.Reasoning && (len(details.ReasoningLevels) != 0 || details.ReasoningDefault != "") {
		return errors.New("models: non-reasoning model carries reasoning identities")
	}
	levels := make(map[string]struct{}, len(details.ReasoningLevels))
	for _, level := range details.ReasoningLevels {
		identity, err := flamemodel.NewReasoningEffortIdentity(level)
		if err != nil {
			return fmt.Errorf("models: reasoning level: %w", err)
		}
		canonical := identity.String()
		if _, duplicate := levels[canonical]; duplicate {
			return fmt.Errorf("models: reasoning level %q is duplicated", canonical)
		}
		levels[canonical] = struct{}{}
	}
	if details.ReasoningDefault != "" {
		identity, err := flamemodel.NewReasoningEffortIdentity(details.ReasoningDefault)
		if err != nil {
			return fmt.Errorf("models: default reasoning level: %w", err)
		}
		if _, offered := levels[identity.String()]; !offered {
			return fmt.Errorf("models: default reasoning level %q is not offered", details.ReasoningDefault)
		}
	}
	return nil
}

func validatePricing(bands []PriceBand) error {
	for index, band := range bands {
		if index == 0 && band.AboveInputTokens != 0 {
			return errors.New("models: the first price band must start at zero input tokens")
		}
		if index > 0 && band.AboveInputTokens <= bands[index-1].AboveInputTokens {
			return errors.New("models: price bands must rise by input tokens")
		}
		for _, rate := range []float64{band.InputPerMillion, band.OutputPerMillion, band.CacheReadPerMillion, band.CacheWritePerMillion} {
			if rate < 0 || math.IsInf(rate, 0) || math.IsNaN(rate) {
				return fmt.Errorf("models: price band rate %v is not a finite non-negative amount", rate)
			}
		}
	}
	return nil
}
