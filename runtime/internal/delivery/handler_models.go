package delivery

import (
	"context"
	"errors"
	"time"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	modelapp "github.com/Tangerg/flame/runtime/internal/application/integration/models"

	"github.com/Tangerg/scope/models/catalog"
)

func (s *Handler) ListModels(ctx context.Context, in protocol.ListModelsRequest) (*protocol.Page[protocol.Model], error) {
	models, err := s.models.ListModels(ctx, in.Provider)
	if err != nil {
		return nil, mapModelError(err)
	}
	defaultSelection := s.sessions.DefaultModelSelection()
	out := make([]protocol.Model, 0, len(models))
	for _, model := range models {
		presented := presentModel(model)
		presented.Default = model.Provider() == defaultSelection.Provider() && model.ID() == defaultSelection.Model()
		out = append(out, presented)
	}
	return protocol.NewPage(out), nil
}

func (s *Handler) GetUtilityRole(_ context.Context) (*protocol.UtilityRole, error) {
	role := s.models.UtilityRole()
	return &protocol.UtilityRole{Provider: role.Provider(), Model: role.Model()}, nil
}

func (s *Handler) SetUtilityRole(ctx context.Context, in protocol.UtilityRole) (*protocol.UtilityRole, error) {
	role, err := s.models.SetUtilityRole(ctx, in.Provider, in.Model)
	if err != nil {
		return nil, mapModelError(err)
	}
	return &protocol.UtilityRole{Provider: role.Provider(), Model: role.Model()}, nil
}

func (s *Handler) GetEmbeddingRole(_ context.Context) (*protocol.EmbeddingRole, error) {
	role := s.models.EmbeddingRole()
	return &protocol.EmbeddingRole{Provider: role.Provider(), Model: role.Model()}, nil
}

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
	if errors.Is(err, modelapp.ErrModelDiscoveryFailed) {
		return protocol.NewFailure(protocol.ErrProviderError, "the provider could not supply a valid model catalog; check its configuration and availability", err)
	}
	if errors.Is(err, modelapp.ErrProviderUnsupported) ||
		errors.Is(err, modelapp.ErrProviderBaseURLRequired) ||
		errors.Is(err, flamemodel.ErrEndpointOverride) ||
		errors.Is(err, modelapp.ErrProviderUnconfigured) ||
		errors.Is(err, modelapp.ErrProviderUpdateRequired) ||
		errors.Is(err, modelapp.ErrEmbeddingUnsupported) ||
		flamemodel.IsInvalid(err) {
		return protocol.NewFailure(protocol.ErrInvalidParams, err.Error(), err)
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
			InputModalities:       presentModalities(details.InputModalities),
			OutputModalities:      presentModalities(details.OutputModalities),
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
	for _, band := range details.Pricing {
		out.Pricing = append(out.Pricing, protocol.ModelPriceBand{
			AboveInputTokens:              band.AboveInputTokens,
			InputUSDPerMillionTokens:      band.InputPerMillion,
			OutputUSDPerMillionTokens:     band.OutputPerMillion,
			CacheReadUSDPerMillionTokens:  band.CacheReadPerMillion,
			CacheWriteUSDPerMillionTokens: band.CacheWritePerMillion,
		})
	}
	return out
}

var modalities = wireVocabulary[catalog.Modality, protocol.Modality]{
	{catalog.ModalityText, protocol.ModalityText},
	{catalog.ModalityImage, protocol.ModalityImage},
	{catalog.ModalityAudio, protocol.ModalityAudio},
	{catalog.ModalityVideo, protocol.ModalityVideo},
	{catalog.ModalityPDF, protocol.ModalityPDF},
}

func presentModalities(in []catalog.Modality) []protocol.Modality {
	if len(in) == 0 {
		return nil
	}
	out := make([]protocol.Modality, len(in))
	for i, modality := range in {
		out[i] = modalities.mustWire(modality)
	}
	return out
}
