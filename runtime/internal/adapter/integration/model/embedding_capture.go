package model

import (
	"context"
	"errors"
	"strings"

	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/scope/core/embedding"
	"github.com/samber/lo"
)

type ResolvedEmbedding struct {
	model   embedding.Model
	space   string
	pricing accounting.Pricing
	close   func(context.Context) error
}

func NewResolvedEmbedding(model embedding.Model, space string, pricing accounting.Pricing, close func(context.Context) error) (ResolvedEmbedding, error) {
	if lo.IsNil(model) || strings.TrimSpace(space) == "" || close == nil {
		return ResolvedEmbedding{}, errors.New("model: embedding client, vector space and lifetime are required")
	}
	return ResolvedEmbedding{model: model, space: space, pricing: pricing, close: close}, nil
}
