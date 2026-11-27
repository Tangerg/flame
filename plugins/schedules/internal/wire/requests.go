package wire

import (
	flamemodel "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/plugins/schedules/internal/schedule"
)

const (
	List   = "flame.schedules/list.v2"
	Create = "flame.schedules/create.v2"
	Update = "flame.schedules/update.v2"
	Delete = "flame.schedules/delete.v3"
	RunNow = "flame.schedules/runNow.v2"
)

type UpdateInput struct {
	ProjectID        *string    `json:"projectId,omitzero"`
	ID               string     `json:"id"`
	ExpectedRevision uint64     `json:"expectedRevision"`
	Title            *string    `json:"title,omitzero"`
	Instructions     *string    `json:"instructions,omitzero"`
	CWD              *string    `json:"cwd,omitzero"`
	Cron             *string    `json:"cron,omitzero"`
	TimeZone         *string    `json:"timeZone,omitzero"`
	Enabled          *bool      `json:"enabled,omitzero"`
	ModelSelection   ModelPatch `json:"modelSelection"`
}

type ListInput struct {
	Cursor string `json:"cursor"`
	Limit  *int   `json:"limit,omitzero"`
}

type Page struct {
	Rows       []schedule.Snapshot `json:"rows"`
	NextCursor string              `json:"nextCursor"`
}

type ItemInput struct {
	ID string `json:"id"`
}

type DeleteInput struct {
	ID               string `json:"id"`
	ExpectedRevision uint64 `json:"expectedRevision"`
}

type DeleteResult struct {
	ID       string `json:"id"`
	Revision uint64 `json:"revision"`
}

type ModelPatch struct {
	Provider        *string `json:"provider,omitzero"`
	Model           *string `json:"model,omitzero"`
	ReasoningEffort *string `json:"reasoningEffort,omitzero"`
}

func (p ModelPatch) Patch() flamemodel.Patch {
	return flamemodel.Patch{Provider: p.Provider, Model: p.Model, ReasoningEffort: p.ReasoningEffort}
}
