package transport

import (
	"context"
	"database/sql"
	json "encoding/json/v2"

	"github.com/Tangerg/flame/runtime/internal/infra/integration/managedmcp"
	sqlitedb "github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	app "github.com/Tangerg/flame/runtime/internal/plugins/memory/application"
	memory "github.com/Tangerg/flame/runtime/internal/plugins/memory/domain"
	"github.com/Tangerg/flame/runtime/internal/plugins/memory/rpc"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
)

// Only publication is transactional here. Ledger facts survive failed model
// work; human edits remain possible while an external model call is pending.
type maintenancePublication struct {
	app.CurationStore
	db      *sql.DB
	command *managedmcp.PreparedCommand
}

func (s maintenancePublication) Reconcile(ctx context.Context, publication memory.Publication) (bool, error) {
	var published bool
	err := sqlitedb.RunInTx(ctx, s.db, func(ctx context.Context) error {
		var err error
		published, err = s.CurationStore.Reconcile(ctx, publication)
		if err != nil {
			return err
		}
		reply, err := maintenanceReply(published)
		if err != nil {
			return err
		}
		return s.command.Complete(ctx, reply)
	})
	return published, err
}

func maintenanceReply(published bool) ([]byte, error) {
	result, err := pluginsdk.Reply(rpc.MaintenanceResult{Published: published}, nil, nil)
	if err != nil {
		return nil, err
	}
	return json.Marshal(result)
}
