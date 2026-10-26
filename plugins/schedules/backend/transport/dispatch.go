package transport

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	pluginstorage "github.com/Tangerg/flame/packages/go-sdk/plugin/sqlite"
	sqlitebase "github.com/Tangerg/flame/packages/go-sdk/storage/sqlite"
	app "github.com/Tangerg/flame/plugins/schedules/backend/application"
)

type RunHost interface {
	app.RunAdmission
	Namespace(context.Context) (string, error)
	Notify(context.Context, string) error
}

type RunConnector func(context.Context, pluginsdk.CallbackRef) (RunHost, func() error, error)

func connectRuntime(ctx context.Context, db *sql.DB, connect RunConnector) (RunHost, func() error, error) {
	ref, err := pluginsdk.CallbackFromContext(ctx)
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
	return sqlitebase.RunInTx(ctx, s.db, func(ctx context.Context) error {
		request, err := s.DispatchStore.CapturedRun(ctx, id)
		if err != nil {
			return err
		}
		if err := s.DispatchStore.AcceptRun(ctx, id); err != nil {
			return err
		}
		result, err := pluginsdk.Reply(app.RunReference{SessionID: request.SessionID(), RunID: request.RunID()}, nil, nil)
		if err != nil {
			return err
		}
		reply, err := json.Marshal(result)
		if err != nil {
			return err
		}
		return s.command.Complete(ctx, reply)
	})
}
