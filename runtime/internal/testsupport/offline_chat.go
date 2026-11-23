package testsupport

import (
	"context"

	modeladapter "github.com/Tangerg/flame/runtime/internal/adapter/integration/model"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/models/catalog"
)

// A scripted model stands in for an installed provider: nothing to
// release and every request admitted.
func OfflineChat(model chat.Model, counter modeladapter.InputTokenCounter, metadata *catalog.Model, pricing accounting.Pricing) (modeladapter.ResolvedChat, error) {
	return modeladapter.NewResolvedChat(model, counter, metadata, pricing,
		func(context.Context) error { return nil },
		func(context.Context, *chat.Request) (bool, error) { return true, nil },
	)
}

// No usage has a known cost.
func Unpriced() accounting.Pricing { return modeladapter.Pricing("", nil) }
