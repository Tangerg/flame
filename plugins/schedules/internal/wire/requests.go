package wire

import (
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	schedule "github.com/Tangerg/flame/plugins/schedules/internal/domain"
)

const (
	List   = "flame.schedules/list.v1"
	Create = "flame.schedules/create.v1"
	Update = "flame.schedules/update.v1"
	Delete = "flame.schedules/delete.v1"
	RunNow = "flame.schedules/runNow.v1"
)

type UpdateInput struct {
	ID               string
	ExpectedRevision uint64
	Title            *string `json:"Title,omitzero"`
	Instructions     *string `json:"Instructions,omitzero"`
	CWD              *string `json:"CWD,omitzero"`
	Cron             *string `json:"Cron,omitzero"`
	Enabled          *bool   `json:"Enabled,omitzero"`
	ModelSelection   ModelPatch
}

type ListInput struct {
	Cursor string
	Limit  *int `json:"Limit,omitzero"`
}

type Page struct {
	Rows       []schedule.Snapshot
	NextCursor string
}

type ItemInput struct{ ID string }

type Ack struct{}

type ModelPatch struct {
	Provider        *string `json:"Provider,omitzero"`
	Model           *string `json:"Model,omitzero"`
	ReasoningEffort *string `json:"ReasoningEffort,omitzero"`
}

func (p ModelPatch) Patch() modelchoice.Patch {
	return modelchoice.Patch{Provider: p.Provider, Model: p.Model, ReasoningEffort: p.ReasoningEffort}
}
