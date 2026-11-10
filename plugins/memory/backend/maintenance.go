package main

import (
	"context"
	"errors"

	framesqlite "github.com/Tangerg/frame/sqlite"

	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	app "github.com/Tangerg/flame/plugins/memory/internal/application"
	rpc "github.com/Tangerg/flame/plugins/memory/internal/contract"
	memory "github.com/Tangerg/flame/plugins/memory/internal/domain"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
)

// Only publication is transactional here. Ledger facts survive failed model
// work; human edits remain possible while an external model call is pending.
type maintenancePublication struct {
	app.CurationStore
	db      *framesqlite.Database
	command *pluginstorage.PreparedCommand
}

func (s maintenancePublication) Reconcile(ctx context.Context, publication memory.Publication) (bool, error) {
	var published bool
	err := framesqlite.RunInTx(ctx, s.db, func(ctx context.Context) error {
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
	if err != nil {
		return false, errors.Join(pluginmcp.ErrOutcomeUnconfirmed, err)
	}
	return published, nil
}

func maintenanceReply(published bool) ([]byte, error) {
	result, err := pluginmcp.EncodeReply(rpc.MaintenanceResult{Published: published}, nil, nil)
	if err != nil {
		return nil, err
	}
	return result.Bytes(), nil
}
