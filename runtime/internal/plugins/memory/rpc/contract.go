package rpc

import (
	"errors"
	"strconv"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/invopop/jsonschema"

	"github.com/Tangerg/flame/runtime/idempotency"
	"github.com/Tangerg/flame/runtime/internal/application/pagination"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/exactjson"
	memory "github.com/Tangerg/flame/runtime/internal/plugins/memory/domain"
)

const (
	List     = "flame.memory/list.v1"
	Add      = "flame.memory/add.v1"
	Update   = "flame.memory/update.v1"
	Delete   = "flame.memory/delete.v1"
	Review   = "flame.memory/review.v1"
	Items    = "flame.memory/items.v1"
	Search   = "flame.memory/search.v1"
	Maintain = "flame.memory/maintain.v1"
)

type ListInput struct {
	Scope       memory.Scope
	CWD, Cursor string
	Limit       *int `json:"Limit,omitzero"`
}

func (ListInput) JSONSchemaExtend(schema *jsonschema.Schema) {
	limit, ok := schema.Properties.Get("Limit")
	if ok {
		limit.Minimum = exactjson.Number("1")
		limit.Maximum = exactjson.Number(strconv.Itoa(memory.MaxManagementPageItems))
	}
}

type Page struct {
	Rows       []memory.ItemSnapshot
	NextCursor string
	Capacity   int
}

func (Page) JSONSchemaExtend(schema *jsonschema.Schema) {
	rows, ok := schema.Properties.Get("Rows")
	if ok {
		rows.MaxItems = new(uint64(memory.MaxManagementPageItems))
	}
	capacity, ok := schema.Properties.Get("Capacity")
	if ok {
		capacity.Const = memory.MaxVisiblePerTarget
	}
}

type AddInput struct {
	Scope        memory.Scope
	CWD, Content string
}
type AddResult struct {
	Item    memory.ItemSnapshot
	Changed bool
}
type UpdateInput struct {
	ID      string
	Content *string `json:"Content,omitzero"`
	Pinned  *bool   `json:"Pinned,omitzero"`
}
type ItemInput struct{ ID string }
type ReviewInput struct {
	ID       string
	Decision memory.ReviewDecision
}
type ItemsInput struct {
	Scope   memory.Scope
	Project string
}
type SearchInput struct {
	Project, Query string
	TopK           int
	EmbeddingSpace string
}
type Conversation struct {
	MessageCount int
	Transcript   string
}
type MaintenanceInput struct {
	SessionID, Project string
	Compacted          bool
	Conversation       Conversation
}
type MaintenanceResult struct{ Published bool }
type Ack struct{}

var errorCodes = map[string]error{"workspace": workspaceapp.ErrCWDUnavailable, "command_conflict": idempotency.ErrKeyConflict,
	"not_found": memory.ErrNotFound, "not_visible": memory.ErrNotVisible, "not_pending": memory.ErrNotPending,
	"target_full": memory.ErrTargetFull, "cursor": pagination.ErrInvalidCursor, "limit": pagination.ErrInvalidLimit,
}

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
