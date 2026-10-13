package transport

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"

	"github.com/Tangerg/flame/runtime/internal/infra/integration/managedmcp"
	sqlitedb "github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	app "github.com/Tangerg/flame/runtime/internal/plugins/schedules/application"
	"github.com/Tangerg/flame/runtime/internal/plugins/schedules/rpc"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
)

type RunHost interface {
	app.RunAdmission
	Namespace(context.Context) (string, error)
}

type RunConnector func(context.Context, pluginsdk.CallbackRef) (RunHost, func() error, error)

func captureCommand[P, R any](ctx context.Context, db *sql.DB, connect RunConnector, method string, command pluginsdk.Command[P], handler func(context.Context, P) (R, error)) (result R, err error) {
	if _, err := pluginsdk.InvocationFromContext(ctx); err != nil {
		return result, err
	}
	_, close, err := connectRuntime(ctx, db, connect)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, close()) }()
	input, err := command.RequestInput()
	if err != nil {
		return result, err
	}
	return managedmcp.Command(ctx, db, method, command.ID, command.Input, input, rpc.Classify, handler)
}

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
	if err == nil && namespace == "" {
		err = errors.New("schedules: Runtime store namespace is required")
	}
	if err == nil {
		err = bindRuntime(ctx, db, namespace)
	}
	if err != nil {
		return nil, nil, errors.Join(err, close())
	}
	return host, close, nil
}

func bindRuntime(ctx context.Context, db *sql.DB, namespace string) error {
	return sqlitedb.RunInTx(ctx, db, func(ctx context.Context) error {
		var original string
		err := sqlitedb.Connection(ctx, db).QueryRowContext(ctx, `SELECT namespace FROM runtime_target WHERE id=1`).Scan(&original)
		if errors.Is(err, sql.ErrNoRows) {
			_, err = sqlitedb.Connection(ctx, db).ExecContext(ctx, `INSERT INTO runtime_target(id,namespace) VALUES(1,?)`, namespace)
			return err
		}
		if err != nil {
			return err
		}
		if original != namespace {
			return errors.New("schedules: captured work belongs to another Runtime store")
		}
		return nil
	})
}

type dispatchCommit struct {
	app.DispatchStore
	db      *sql.DB
	command *managedmcp.PreparedCommand
}

func (s dispatchCommit) AcceptRun(ctx context.Context, id string) error {
	return sqlitedb.RunInTx(ctx, s.db, func(ctx context.Context) error {
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

func dispatchRun(ctx context.Context, db *sql.DB, store app.DispatchStore, connect RunConnector, command pluginsdk.Command[rpc.DispatchInput]) (result app.RunReference, err error) {
	input, err := command.RequestInput()
	if err != nil {
		return result, err
	}
	request, err := store.CapturedRun(ctx, command.Input.RunID)
	if err != nil {
		return result, err
	}
	invocation, err := pluginsdk.InvocationFromContext(ctx)
	if err != nil {
		return result, err
	}
	if invocation != request.Invocation() {
		return result, errors.New("schedules: dispatch caller differs from captured trigger")
	}
	prepared, original, err := managedmcp.AdmitCommand(ctx, db, rpc.Dispatch, command.ID, input, nil)
	if err != nil {
		return result, err
	}
	if prepared == nil {
		return pluginsdk.DecodeReply[app.RunReference](original)
	}
	runs, close, err := connectRuntime(ctx, db, connect)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, close()) }()
	dispatcher, err := app.NewDispatcher(dispatchCommit{DispatchStore: store, db: db, command: prepared}, runs)
	if err != nil {
		return result, err
	}
	return dispatcher.Dispatch(ctx, command.Input.RunID)
}
