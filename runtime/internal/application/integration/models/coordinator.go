package models

import (
	"context"
	"errors"
	"fmt"
	"time"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/runtime/internal/application/invalidation"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/provider"
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

type Coordinator struct {
	providers    ProviderRegistry
	catalog      providerCatalog
	prober       providerProber
	lister       ProviderModelLister
	probeTimeout time.Duration

	utilityRole      roleStore
	utilityValidator ChatModelValidator

	embeddingRole      roleStore
	embeddingValidator embeddingModelValidator
	invalidations      invalidation.Publish
}

type Config struct {
	Providers    ProviderRegistry
	Catalog      providerCatalog
	Prober       providerProber
	Lister       ProviderModelLister
	ProbeTimeout time.Duration

	UtilityRole      roleStore
	UtilityValidator ChatModelValidator

	EmbeddingRole      roleStore
	EmbeddingValidator embeddingModelValidator

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
		{"utility role", cfg.UtilityRole},
		{"utility validator", cfg.UtilityValidator},
		{"embedding role", cfg.EmbeddingRole},
		{"embedding validator", cfg.EmbeddingValidator},
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
		utilityRole:        cfg.UtilityRole,
		utilityValidator:   cfg.UtilityValidator,
		embeddingRole:      cfg.EmbeddingRole,
		embeddingValidator: cfg.EmbeddingValidator,
		invalidations:      cfg.Invalidations,
	}, nil
}
