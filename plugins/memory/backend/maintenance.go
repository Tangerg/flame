package main

import (
	"context"
	"database/sql"

	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	app "github.com/Tangerg/flame/plugins/memory/internal/application"
	rpc "github.com/Tangerg/flame/plugins/memory/internal/contract"
	memory "github.com/Tangerg/flame/plugins/memory/internal/domain"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/sqlite"
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
	err := sqlite.RunInTx(ctx, s.db, func(ctx context.Context) error {
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
	result, err := qimumcp.EncodeReply(rpc.MaintenanceResult{Published: published}, nil, nil)
	if err != nil {
		return nil, err
	}
	return result.JSON(), nil
}
