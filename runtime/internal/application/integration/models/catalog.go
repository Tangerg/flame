package models

import (
	"fmt"
	"slices"
	"time"

	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
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

// EmbeddingCapability is a closed provider capability. A default model can
// only exist for a provider that actually has an embedding implementation.
type EmbeddingCapability struct {
	kind         embeddingCapabilityKind
	defaultModel modelref.ModelIdentity
}

func NoEmbeddingCapability() EmbeddingCapability {
	return EmbeddingCapability{kind: embeddingUnsupported}
}

func EmbeddingCapabilityWithoutDefault() EmbeddingCapability {
	return EmbeddingCapability{kind: embeddingSupportedWithoutDefault}
}

func EmbeddingCapabilityWithDefault(model string) (EmbeddingCapability, error) {
	identity, err := modelref.NewModelIdentity(model)
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
		_, err := modelref.NewModelIdentity(c.defaultModel.String())
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

// ProviderMetadata is immutable static reference data used by provider/model
// use cases. Closed endpoint, discovery, and embedding policies prevent the
// contradictory combinations possible in the former public field bag.
type ProviderMetadata struct {
	id             modelref.ProviderIdentity
	authentication ProviderAuthenticationPolicy
	endpoint       ProviderEndpointPolicy
	models         ProviderModelSource
	embedding      EmbeddingCapability
}

func NewProviderMetadata(id string, authentication ProviderAuthenticationPolicy, endpoint ProviderEndpointPolicy, models ProviderModelSource, embedding EmbeddingCapability) (ProviderMetadata, error) {
	identity, err := modelref.NewProviderIdentity(id)
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

// Validate rechecks static catalog metadata returned through the application
// port. Production metadata is constructor-built, but the use case must not
// publish a zero or contradictory value from an alternate implementation.
func (p ProviderMetadata) Validate() error {
	_, err := NewProviderMetadata(p.ID(), p.authentication, p.endpoint, p.models, p.embedding)
	return err
}

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

// ConfigurationSatisfied applies static provider policy to one registry entry.
// A provider with optional authentication and a built-in endpoint is usable
// without manufacturing a credential row.
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

// Model is the application-facing catalog record used by model selection. It
// carries only provider capability facts needed by model-selection use cases.
type Model struct {
	selection modelref.Selection
	details   *Details
}

// NewModel constructs one immutable application catalog record. Endpoint and
// bundled catalog sources therefore cross the same admission boundary before
// a model identity can reach a picker or generated response.
func NewModel(providerID, modelID string, details *Details) (Model, error) {
	selection, err := modelref.New(providerID, modelID)
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

// Details is the static capability and commercial metadata known for a
// model. A nil Details means a provider endpoint reported an otherwise unknown
// model id, so callers can still select it without inventing capabilities.
type Details struct {
	DisplayName      string
	TokenLimits      modelref.TokenLimits
	KnowledgeCutoff  time.Time
	Deprecated       bool
	Reasoning        bool
	ReasoningLevels  []string
	ReasoningDefault string
	Multimodal       bool
	InputModalities  []string
	OutputModalities []string
	ToolUse          bool
	StructuredOutput bool
	Pricing          *Pricing
}

// Pricing is the primary per-million-token rate the runtime displays for a
// model. Zero-valued cache rates mean the provider does not price them
// separately.
type Pricing struct {
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
	if details.Pricing != nil {
		pricing := *details.Pricing
		clone.Pricing = &pricing
	}
	return &clone
}

func validateDetails(details *Details) error {
	if details == nil {
		return nil
	}
	if !details.Reasoning && (len(details.ReasoningLevels) != 0 || details.ReasoningDefault != "") {
		return fmt.Errorf("models: non-reasoning model carries reasoning identities")
	}
	levels := make(map[string]struct{}, len(details.ReasoningLevels))
	for _, level := range details.ReasoningLevels {
		identity, err := modelref.NewReasoningEffortIdentity(level)
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
		identity, err := modelref.NewReasoningEffortIdentity(details.ReasoningDefault)
		if err != nil {
			return fmt.Errorf("models: default reasoning level: %w", err)
		}
		if _, offered := levels[identity.String()]; !offered {
			return fmt.Errorf("models: default reasoning level %q is not offered", details.ReasoningDefault)
		}
	}
	return nil
}
