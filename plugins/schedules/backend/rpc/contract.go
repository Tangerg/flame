package rpc

import (
	"errors"

	workspacepaths "github.com/Tangerg/flame/packages/go-sdk/workspace"

	"github.com/Tangerg/flame/packages/go-sdk/capability"
	"github.com/Tangerg/flame/packages/go-sdk/idempotency"
	modelchoice "github.com/Tangerg/flame/packages/go-sdk/model"
	"github.com/Tangerg/flame/packages/go-sdk/pagination"
	schedule "github.com/Tangerg/flame/plugins/schedules/backend/domain"
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

var errorCodes = map[string]error{"workspace": workspacepaths.ErrCWDUnavailable, "command_conflict": idempotency.ErrKeyConflict, "not_found": schedule.ErrNotFound, "revision": schedule.ErrRevisionConflict, "cursor": pagination.ErrInvalidCursor, "limit": pagination.ErrInvalidLimit, "invalid_cron": schedule.ErrInvalidCron, "model": modelchoice.ErrUnsupported, "instructions": schedule.ErrInstructionsRequired, "revision_required": schedule.ErrRevisionRequired}

func Classify(err error) string {
	if failure, ok := errors.AsType[*capability.Refusal](err); ok {
		if errorCodes[failure.Code] != nil {
			return failure.Code
		}
	}
	for code, cause := range errorCodes {
		if errors.Is(err, cause) {
			return code
		}
	}
	return ""
}
func DecodeError(err error) error {
	var failure *capability.Refusal
	if errors.As(err, &failure) {
		if cause := errorCodes[failure.Code]; cause != nil {
			return errors.Join(cause, err)
		}
	}
	return err
}

type ModelPatch struct {
	Provider        *string `json:"Provider,omitzero"`
	Model           *string `json:"Model,omitzero"`
	ReasoningEffort *string `json:"ReasoningEffort,omitzero"`
}

func EncodeModelPatch(value modelchoice.Patch) ModelPatch {
	return ModelPatch{Provider: value.Provider, Model: value.Model, ReasoningEffort: value.ReasoningEffort}
}
func (p ModelPatch) Patch() modelchoice.Patch {
	return modelchoice.Patch{Provider: p.Provider, Model: p.Model, ReasoningEffort: p.ReasoningEffort}
}
