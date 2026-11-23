package models

import (
	"context"

	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/scope/core/chat"
)

type Selector interface {
	SelectModel(context.Context, SelectionInput) (flamemodel.Selection, error)
}

type SelectionInput struct {
	Context  flamemodel.SelectionContext
	Proposed flamemodel.Selection
	Request  *chat.Request
}
