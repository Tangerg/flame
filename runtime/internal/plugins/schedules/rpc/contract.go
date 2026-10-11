package rpc

import (
	"errors"
	"time"

	"github.com/Tangerg/flame/runtime/internal/application/pagination"
	workspaceapp "github.com/Tangerg/flame/runtime/internal/application/workspace"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/idempotency"
	"github.com/Tangerg/flame/runtime/internal/infra/integration/managedmcp"
	schedule "github.com/Tangerg/flame/runtime/internal/plugins/schedules/domain"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

const (
	List          = "flame.schedules/list.v1"
	Create        = "flame.schedules/create.v1"
	Update        = "flame.schedules/update.v1"
	Delete        = "flame.schedules/delete.v1"
	PrepareManual = "flame.schedules/prepareManual.v1"
	Scan          = "flame.schedules/scan.v1"
	Accept        = "flame.schedules/accept.v1"
)

type UpdateInput struct {
	ID                             string
	ExpectedRevision               uint64
	Title, Instructions, CWD, Cron *string
	Enabled                        *bool
	ModelSelection                 modelref.Patch
}
type ListInput struct {
	Cursor string
	Limit  *int
}
type Page struct {
	Rows       []schedule.Snapshot
	NextCursor string
}
type ItemInput struct{ ID string }
type ScanInput struct{ Now time.Time }
type AcceptInput struct{ RunID string }
type Ack struct{}

func RegisterClient(client *sdk.Client) error {
	return errors.Join(
		managedmcp.RegisterClient[ListInput, Page](client, List),
		managedmcp.RegisterClient[schedule.Draft, schedule.Snapshot](client, Create),
		managedmcp.RegisterClient[UpdateInput, schedule.Snapshot](client, Update),
		managedmcp.RegisterClient[ItemInput, Ack](client, Delete),
		managedmcp.RegisterClient[ItemInput, schedule.RunRequestSnapshot](client, PrepareManual),
		managedmcp.RegisterClient[ScanInput, []schedule.RunRequestSnapshot](client, Scan),
		managedmcp.RegisterClient[AcceptInput, Ack](client, Accept),
	)
}

var errorCodes = map[string]error{"workspace": workspaceapp.ErrCWDUnavailable, "command_conflict": idempotency.ErrKeyConflict, "not_found": schedule.ErrNotFound, "revision": schedule.ErrRevisionConflict, "cursor": pagination.ErrInvalidCursor, "limit": pagination.ErrInvalidLimit, "invalid_cron": schedule.ErrInvalidCron, "model": modelref.ErrUnsupported, "instructions": schedule.ErrInstructionsRequired, "revision_required": schedule.ErrRevisionRequired}

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
