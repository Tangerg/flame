package main

import (
	"context"
	"errors"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	"github.com/Tangerg/flame/plugins/schedules/internal/scheduling"

	framesqlite "github.com/Tangerg/frame/sqlite"
)

func connectRuntime(ctx context.Context, db framesqlite.Database) (remoteRuns, func() error, error) {
	ref, err := pluginmcp.CallbackFromContext(ctx)
	if err != nil {
		return remoteRuns{}, nil, err
	}
	host, release, err := connectRunHost(ctx, ref)
	if err != nil {
		return remoteRuns{}, nil, err
	}
	namespace, err := host.Namespace(ctx)
	if err == nil {
		err = pluginstorage.BindRuntimeTarget(ctx, db, namespace)
	}
	if err != nil {
		return remoteRuns{}, nil, errors.Join(err, release())
	}
	return host, release, nil
}

type dispatchCommit struct {
	scheduling.DispatchStore
	db      framesqlite.Database
	command *pluginstorage.PreparedCommand
}

func (s dispatchCommit) AcceptRun(ctx context.Context, id identity.RunID) error {
	err := s.db.RunInTx(ctx, func(ctx context.Context) error {
		request, err := s.CapturedRun(ctx, id)
		if err != nil {
			return err
		}
		if err := s.DispatchStore.AcceptRun(ctx, id); err != nil {
			return err
		}
		result, err := pluginmcp.EncodeReply(scheduling.NewRunReference(request), nil, nil)
		if err != nil {
			return err
		}
		return s.command.Complete(ctx, result.Bytes())
	})
	if err != nil {
		return errors.Join(pluginmcp.ErrOutcomeUnconfirmed, err)
	}
	return nil
}
