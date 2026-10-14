package models

import (
	"context"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/scope/core/chat"
)

// Selector chooses a proposal before the calling owner accepts its exact model
// binding. Accepted executions restore that binding without another selection.
type Selector interface {
	SelectModel(context.Context, modelref.Selection, *chat.Request) (modelref.Selection, error)
}
