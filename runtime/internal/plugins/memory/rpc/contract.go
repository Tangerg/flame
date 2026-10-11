package rpc

import (
	"errors"

	"github.com/Tangerg/flame/runtime/internal/application/pagination"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/idempotency"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/managedmcp"
	memory "github.com/Tangerg/flame/runtime/internal/plugins/memory/domain"
	sdk "github.com/Tangerg/go-sdk/mcp"
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
	Limit       *int
}
type Page struct {
	Rows       []memory.ItemSnapshot
	NextCursor string
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
	Content *string
	Pinned  *bool
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
	Project, Query        string
	TopK                  int
	Grant, EmbeddingSpace string
}
type Conversation struct {
	MessageCount int
	Transcript   string
}
type MaintenanceInput struct {
	SessionID, Project, Grant string
	Compacted                 bool
	Conversation              Conversation
}
type MaintenanceResult struct{ Published bool }
type Ack struct{}

func RegisterClient(client *sdk.Client) error {
	return errors.Join(
		managedmcp.RegisterClient[ListInput, Page](client, List),
		managedmcp.RegisterClient[AddInput, AddResult](client, Add),
		managedmcp.RegisterClient[UpdateInput, memory.ItemSnapshot](client, Update),
		managedmcp.RegisterClient[ItemInput, Ack](client, Delete),
		managedmcp.RegisterClient[ReviewInput, Ack](client, Review),
		managedmcp.RegisterClient[ItemsInput, []memory.ItemSnapshot](client, Items),
		managedmcp.RegisterClient[SearchInput, []memory.ItemSnapshot](client, Search),
		managedmcp.RegisterClient[MaintenanceInput, MaintenanceResult](client, Maintain),
	)
}

var errorCodes = map[string]error{"workspace": workspaceapp.ErrCWDUnavailable, "command_conflict": idempotency.ErrKeyConflict,
	"not_found": memory.ErrNotFound, "not_visible": memory.ErrNotVisible, "not_pending": memory.ErrNotPending,
	"target_full": memory.ErrTargetFull, "cursor": pagination.ErrInvalidCursor, "limit": pagination.ErrInvalidLimit,
}

func Classify(err error) string {
	for code, cause := range errorCodes {
		if errors.Is(err, cause) {
			return code
		}
	}
	return "internal"
}
func DecodeError(err error) error {
	var failure *managedmcp.Failure
	if errors.As(err, &failure) {
		if cause := errorCodes[failure.Code]; cause != nil {
			return errors.Join(cause, err)
		}
	}
	return err
}
