package main

import (
	"context"
	"database/sql"
	"errors"

	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	app "github.com/Tangerg/flame/plugins/schedules/internal/application"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/sqlite"
)

type runHost interface {
	app.RunAdmission
	Namespace(context.Context) (string, error)
	Notify(context.Context, string) error
}

type runConnector func(context.Context, qimumcp.CallbackRef) (runHost, func() error, error)

func connectRuntime(ctx context.Context, db *sql.DB, connect runConnector) (runHost, func() error, error) {
	ref, err := qimumcp.CallbackFromContext(ctx)
	if err != nil {
		return nil, nil, err
	}
	host, close, err := connect(ctx, ref)
	if err != nil {
		return nil, nil, err
	}
	namespace, err := host.Namespace(ctx)
	if err == nil {
		err = pluginstorage.BindRuntimeTarget(ctx, db, namespace)
	}
	if err != nil {
		return nil, nil, errors.Join(err, close())
	}
	return host, close, nil
}

type dispatchCommit struct {
	app.DispatchStore
	db      *sql.DB
	command *pluginstorage.PreparedCommand
}

func (s dispatchCommit) AcceptRun(ctx context.Context, id string) error {
	return sqlite.RunInTx(ctx, s.db, func(ctx context.Context) error {
		request, err := s.DispatchStore.CapturedRun(ctx, id)
		if err != nil {
			return err
		}
		if err := s.DispatchStore.AcceptRun(ctx, id); err != nil {
			return err
		}
		result, err := qimumcp.EncodeReply(app.RunReference{SessionID: request.SessionID(), RunID: request.RunID()}, nil, nil)
		if err != nil {
			return err
		}
		return s.command.Complete(ctx, result.JSON())
	})
}
