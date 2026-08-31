package delivery

import (
	"context"
	"errors"
	"fmt"
	"time"

	modelapp "github.com/Tangerg/flame/runtime/internal/application/integration/models"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/protocol"
)

// ListModels projects the application-owned model-discovery result onto the
// protocol page. Discovery policy, remote fallback, and catalog enrichment all
// remain in application/integration/models.
func (s *Handler) ListModels(ctx context.Context, in protocol.ListModelsRequest) (*protocol.Page[protocol.Model], error) {
	models, err := s.models.ListModels(ctx, in.Provider)
	if err != nil {
		return nil, mapModelError(err)
	}
	out := make([]protocol.Model, 0, len(models))
	for _, model := range models {
		out = append(out, presentModel(model))
	}
	return protocol.NewPage(out), nil
}

// GetUtilityRole reports the (provider, model) the in-house maintenance
// services run on — empty model when unset, meaning they run on the main Run
// model (models.getUtilityRole).
func (s *Handler) GetUtilityRole(_ context.Context) (*protocol.UtilityRole, error) {
	role := s.models.UtilityRole()
	return &protocol.UtilityRole{Provider: role.Provider(), Model: role.Model()}, nil
}

// SetUtilityRole points the maintenance services at a (provider, model),
// validated and persisted by the application use case. Returns the stored role.
func (s *Handler) SetUtilityRole(ctx context.Context, in protocol.UtilityRole) (*protocol.UtilityRole, error) {
	role, err := s.models.SetUtilityRole(ctx, in.Provider, in.Model)
	if err != nil {
		return nil, mapModelError(err)
	}
	return &protocol.UtilityRole{Provider: role.Provider(), Model: role.Model()}, nil
}

// GetEmbeddingRole reports the optional (provider, model) for agent-memory ranking
// embeds with — empty model when unset (the feature is off)
// (models.getEmbeddingRole).
func (s *Handler) GetEmbeddingRole(_ context.Context) (*protocol.EmbeddingRole, error) {
	role := s.models.EmbeddingRole()
	return &protocol.EmbeddingRole{Provider: role.Provider(), Model: role.Model()}, nil
}

// SetEmbeddingRole points the index at an (embedding-capable provider, model),
// validated and persisted by the application use case. Returns the stored role.
func (s *Handler) SetEmbeddingRole(ctx context.Context, in protocol.EmbeddingRole) (*protocol.EmbeddingRole, error) {
	role, err := s.models.SetEmbeddingRole(ctx, in.Provider, in.Model)
	if err != nil {
		return nil, mapModelError(err)
	}
	return &protocol.EmbeddingRole{Provider: role.Provider(), Model: role.Model()}, nil
}

func mapModelError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, modelapp.ErrProviderUnsupported) ||
		errors.Is(err, modelapp.ErrProviderBaseURLRequired) ||
		errors.Is(err, modelapp.ErrProviderUnconfigured) ||
		errors.Is(err, modelapp.ErrProviderUpdateRequired) ||
		errors.Is(err, modelapp.ErrEmbeddingUnsupported) ||
		errors.Is(err, modelref.ErrIncomplete) {
		return fmt.Errorf("%w: %w", protocol.ErrInvalidParams, err)
	}
	return err
}

func presentModel(model modelapp.Model) protocol.Model {
	details := model.Details()
	if details == nil {
		return protocol.Model{ID: model.ID(), Provider: model.Provider()}
	}
	out := protocol.Model{
		ID:          model.ID(),
		Provider:    model.Provider(),
		DisplayName: details.DisplayName,
		Deprecated:  details.Deprecated,
		Capabilities: &protocol.ModelCapabilities{
			Reasoning:             details.Reasoning,
			ReasoningLevels:       details.ReasoningLevels,
			ReasoningDefaultLevel: details.ReasoningDefault,
			Multimodal:            details.Multimodal,
			InputModalities:       toWireModalities(details.InputModalities),
			OutputModalities:      toWireModalities(details.OutputModalities),
			ToolUse:               details.ToolUse,
			StructuredOutput:      details.StructuredOutput,
		},
	}
	if !details.TokenLimits.Unknown() {
		out.TokenLimits = &protocol.ModelTokenLimits{}
		if value, present := details.TokenLimits.ContextWindow(); present {
			out.TokenLimits.ContextWindow = &value
		}
		if value, present := details.TokenLimits.MaxInputTokens(); present {
			out.TokenLimits.MaxInputTokens = &value
		}
		if value, present := details.TokenLimits.MaxOutputTokens(); present {
			out.TokenLimits.MaxOutputTokens = &value
		}
	}
	if !details.KnowledgeCutoff.IsZero() {
		out.KnowledgeCutoff = details.KnowledgeCutoff.Format(time.DateOnly)
	}
	if details.Pricing != nil {
		out.Pricing = &protocol.ModelPricing{
			InputUSDPerMillionTokens:      details.Pricing.InputPerMillion,
			OutputUSDPerMillionTokens:     details.Pricing.OutputPerMillion,
			CacheReadUSDPerMillionTokens:  details.Pricing.CacheReadPerMillion,
			CacheWriteUSDPerMillionTokens: details.Pricing.CacheWritePerMillion,
		}
	}
	return out
}

func toWireModalities(in []string) []protocol.Modality {
	if len(in) == 0 {
		return nil
	}
	out := make([]protocol.Modality, len(in))
	for i, modality := range in {
		out[i] = protocol.Modality(modality)
	}
	return out
}
