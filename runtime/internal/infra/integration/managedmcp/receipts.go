package managedmcp

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/internal/idempotency"
	sqlitedb "github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/jsoninput"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

const CommandsSchema = `CREATE TABLE IF NOT EXISTS commands (id TEXT PRIMARY KEY, method TEXT NOT NULL, input_json TEXT NOT NULL CHECK(json_valid(input_json)), reply TEXT NOT NULL CHECK(json_valid(reply)))`

func Command[P, R any](ctx context.Context, db *sql.DB, method, id string, input P, admitted jsoninput.Input, classify func(error) string, handler func(context.Context, P) (R, error)) (R, error) {
	var zero R
	if id == "" || len(id) > 256 || !utf8.ValidString(id) {
		return zero, errors.New("managed plugin: bounded command identity is required")
	}
	if err := admitted.Validate(); err != nil {
		return zero, err
	}
	var reply []byte
	err := sqlitedb.RunInTx(ctx, db, func(ctx context.Context) error {
		connection := sqlitedb.Connection(ctx, db)
		var previousMethod, previousInput string
		err := connection.QueryRowContext(ctx, `SELECT method,input_json,reply FROM commands WHERE id=?`, id).Scan(&previousMethod, &previousInput, &reply)
		if err == nil {
			if previousMethod != method || previousInput != admitted.String() {
				return idempotency.ErrKeyConflict
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := connection.ExecContext(ctx, `SAVEPOINT command_mutation`); err != nil {
			return err
		}
		value, operationErr := handler(ctx, input)
		outcome, err := pluginsdk.Reply(value, operationErr, classify)
		if err != nil {
			return err
		}
		if operationErr != nil {
			if _, err := connection.ExecContext(ctx, `ROLLBACK TO command_mutation`); err != nil {
				return err
			}
		}
		if _, err := connection.ExecContext(ctx, `RELEASE command_mutation`); err != nil {
			return err
		}
		reply, err = json.Marshal(outcome, json.Deterministic(true))
		if err != nil {
			return err
		}
		_, err = connection.ExecContext(ctx, `INSERT INTO commands(id,method,input_json,reply) VALUES(?,?,?,?)`, id, method, admitted.String(), string(reply))
		return err
	})
	if err != nil {
		return zero, err
	}
	return pluginsdk.DecodeReply[R](reply)
}

func RegisterCommand[P, R any](server *pluginsdk.Backend, db *sql.DB, name string, classify func(error) string, handler func(context.Context, *sdk.ServerSession, P) (R, error)) error {
	return pluginsdk.RegisterCommand(server, name, classify, func(ctx context.Context, session *sdk.ServerSession, command pluginsdk.Command[P]) (R, error) {
		admitted, err := command.RequestInput()
		if err != nil {
			var zero R
			return zero, err
		}
		return Command(ctx, db, name, command.ID, command.Input, admitted, classify, func(ctx context.Context, input P) (R, error) { return handler(ctx, session, input) })
	})
}

func RegisterReceipts(server *pluginsdk.Backend, db *sql.DB) error {
	return pluginsdk.RegisterServer(server, pluginsdk.ReceiptMethod, nil, func(ctx context.Context, _ *sdk.ServerSession, input pluginsdk.ReceiptRequest) (pluginsdk.Receipt, error) {
		var method, encoded, reply string
		err := sqlitedb.Connection(ctx, db).QueryRowContext(ctx, `SELECT method,input_json,reply FROM commands WHERE id=?`, input.ID).Scan(&method, &encoded, &reply)
		if errors.Is(err, sql.ErrNoRows) {
			return pluginsdk.Receipt{}, nil
		}
		if err != nil {
			return pluginsdk.Receipt{}, err
		}
		admitted, err := jsoninput.Parse([]byte(encoded))
		if err != nil {
			return pluginsdk.Receipt{}, err
		}
		if method != input.Method || admitted.Digest() != input.InputDigest {
			return pluginsdk.Receipt{}, idempotency.ErrKeyConflict
		}
		return pluginsdk.Receipt{Found: true, Reply: []byte(reply)}, nil
	})
}
