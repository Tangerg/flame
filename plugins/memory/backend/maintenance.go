package main

import (
	"context"
	"errors"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/plugins/memory/internal/agentmemory"
	rpc "github.com/Tangerg/flame/plugins/memory/internal/contract"
	"github.com/Tangerg/flame/plugins/memory/internal/coordination"

	framesqlite "github.com/Tangerg/frame/sqlite"
)

type maintenancePublication struct {
	coordination.CurationStore
	db      framesqlite.Database
	command *pluginstorage.PreparedCommand
}

func (s maintenancePublication) Reconcile(ctx context.Context, publication agentmemory.Publication) (bool, error) {
	var published bool
	err := s.db.RunInTx(ctx, func(ctx context.Context) error {
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
