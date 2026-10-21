package transport

import (
	"context"
	"database/sql"
	json "encoding/json/v2"

	pluginsdk "github.com/Tangerg/flame/packages/plugin-sdk"
	pluginstorage "github.com/Tangerg/flame/packages/plugin-sdk/sqlite"
	app "github.com/Tangerg/flame/plugins/memory/backend/application"
	memory "github.com/Tangerg/flame/plugins/memory/backend/domain"
	"github.com/Tangerg/flame/plugins/memory/backend/rpc"
	sqlitebase "github.com/Tangerg/flame/runtime/storage/sqlite"
)

// Only publication is transactional here. Ledger facts survive failed model
// work; human edits remain possible while an external model call is pending.
type maintenancePublication struct {
	app.CurationStore
	db      *sql.DB
	command *pluginstorage.PreparedCommand
}

func (s maintenancePublication) Reconcile(ctx context.Context, publication memory.Publication) (bool, error) {
	var published bool
	err := sqlitebase.RunInTx(ctx, s.db, func(ctx context.Context) error {
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
