package models

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/samber/lo"
)

const defaultProviderProbeTimeout = 10 * time.Second

type providerCatalog interface {
	Profiles(context.Context) ([]ProviderProfile, error)
	Profile(context.Context, string) (ProviderProfile, error)
}

type ProviderProfile struct {
	Metadata ProviderMetadata
	Endpoint flamemodel.EndpointPolicy
	Models   []Model
}

var (
	ErrProviderUnsupported     = errors.New("models: provider is unsupported")
	ErrProviderBaseURLRequired = errors.New("models: provider base URL is required")
	ErrProviderUnconfigured    = errors.New("models: provider is not configured")
	ErrProviderUpdateRequired  = errors.New("models: provider update has no changes")
	ErrEmbeddingUnsupported    = errors.New("models: provider does not support embeddings")
)

type providerProber interface {
	Probe(ctx context.Context, entry provider.Provider) error
}

type ProviderModelLister interface {
	ListModels(ctx context.Context, entry provider.Provider) ([]string, error)
}

type ChatModelValidator interface {
	ValidateChatModel(ctx context.Context, providerID, model string) error
}

type embeddingModelValidator interface {
	ValidateEmbeddingModel(ctx context.Context, providerID, model string) error
}

type roleSaver interface {
	Save(ctx context.Context, role modelref.Role) error
}

type Coordinator struct {
	providers    ProviderRegistry
	catalog      providerCatalog
	prober       providerProber
	lister       ProviderModelLister
	probeTimeout time.Duration

	utilityRoleState *RoleState
	utilityValidator ChatModelValidator
	utilityStore     roleSaver
	utilityMu        sync.Mutex

	embeddingRoleState *RoleState
	embeddingValidator embeddingModelValidator
	embeddingStore     roleSaver
	embeddingMu        sync.Mutex
	invalidations      invalidation.Publish
}

type Config struct {
	Providers    ProviderRegistry
	Catalog      providerCatalog
	Prober       providerProber
	Lister       ProviderModelLister
	ProbeTimeout time.Duration

	UtilityRoleState *RoleState
	UtilityValidator ChatModelValidator
	UtilityStore     roleSaver

	EmbeddingRoleState *RoleState
	EmbeddingValidator embeddingModelValidator
	EmbeddingStore     roleSaver

	Invalidations invalidation.Publish
}

func New(cfg Config) (*Coordinator, error) {
	for _, required := range []struct {
		name  string
		value any
	}{
		{"providers", cfg.Providers},
		{"catalog", cfg.Catalog},
		{"prober", cfg.Prober},
		{"lister", cfg.Lister},
		{"utility role state", cfg.UtilityRoleState},
		{"utility validator", cfg.UtilityValidator},
		{"utility store", cfg.UtilityStore},
		{"embedding role state", cfg.EmbeddingRoleState},
		{"embedding validator", cfg.EmbeddingValidator},
		{"embedding store", cfg.EmbeddingStore},
		{"invalidations", cfg.Invalidations},
	} {
		if lo.IsNil(required.value) {
			return nil, fmt.Errorf("models: %s is required", required.name)
		}
	}
	if cfg.ProbeTimeout <= 0 {
		cfg.ProbeTimeout = defaultProviderProbeTimeout
	}
	return &Coordinator{
		providers:          cfg.Providers,
		catalog:            cfg.Catalog,
		prober:             cfg.Prober,
		lister:             cfg.Lister,
		probeTimeout:       cfg.ProbeTimeout,
		utilityRoleState:   cfg.UtilityRoleState,
		utilityValidator:   cfg.UtilityValidator,
		utilityStore:       cfg.UtilityStore,
		embeddingRoleState: cfg.EmbeddingRoleState,
		embeddingValidator: cfg.EmbeddingValidator,
		embeddingStore:     cfg.EmbeddingStore,
		invalidations:      cfg.Invalidations,
	}, nil
}
