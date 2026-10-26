package models

import (
	"context"

	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/scope/core/chat"
)

// Selector chooses a proposal before the calling owner accepts its exact model
// binding. Accepted executions restore that binding without another selection.
type Selector interface {
	SelectModel(context.Context, SelectionInput) (modelchoice.Selection, error)
}

type SelectionInput struct {
	Context  modelchoice.SelectionContext
	Proposed modelchoice.Selection
	Request  *chat.Request
}
