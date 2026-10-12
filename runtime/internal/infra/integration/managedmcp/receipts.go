package managedmcp

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/internal/idempotency"
	sqlitedb "github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

const CommandsSchema = `CREATE TABLE IF NOT EXISTS commands (id TEXT PRIMARY KEY, method TEXT NOT NULL, input_digest TEXT NOT NULL, reply TEXT NOT NULL CHECK(json_valid(reply)))`

func Command[P, R any](ctx context.Context, db *sql.DB, method, id string, input P, classify func(error) string, handler func(context.Context, P) (R, error)) (R, error) {
	var zero R
	if id == "" || len(id) > 256 || !utf8.ValidString(id) {
		return zero, errors.New("managed plugin: bounded command identity is required")
	}
	digest, err := pluginsdk.InputDigest(input)
	if err != nil {
		return zero, err
	}
	var reply []byte
	err = sqlitedb.RunInTx(ctx, db, func(ctx context.Context) error {
		connection := sqlitedb.Connection(ctx, db)
		var previousMethod, previousDigest string
		err := connection.QueryRowContext(ctx, `SELECT method,input_digest,reply FROM commands WHERE id=?`, id).Scan(&previousMethod, &previousDigest, &reply)
		if err == nil {
			if previousMethod != method || previousDigest != digest {
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
		_, err = connection.ExecContext(ctx, `INSERT INTO commands(id,method,input_digest,reply) VALUES(?,?,?,?)`, id, method, digest, string(reply))
		return err
	})
	if err != nil {
		return zero, err
	}
	return pluginsdk.DecodeReply[R](reply)
}

func RegisterCommand[P, R any](server *sdk.Server, db *sql.DB, name string, classify func(error) string, handler func(context.Context, *sdk.ServerSession, P) (R, error)) error {
	return pluginsdk.RegisterCommand(server, name, classify, func(ctx context.Context, session *sdk.ServerSession, command pluginsdk.Command[P]) (R, error) {
		return Command(ctx, db, name, command.ID, command.Input, classify, func(ctx context.Context, input P) (R, error) { return handler(ctx, session, input) })
	})
}

func RegisterReceipts(server *sdk.Server, db *sql.DB) error {
	return pluginsdk.RegisterServer(server, pluginsdk.ReceiptMethod, nil, func(ctx context.Context, _ *sdk.ServerSession, input pluginsdk.ReceiptRequest) (pluginsdk.Receipt, error) {
		var method, digest, reply string
		err := sqlitedb.Connection(ctx, db).QueryRowContext(ctx, `SELECT method,input_digest,reply FROM commands WHERE id=?`, input.ID).Scan(&method, &digest, &reply)
		if errors.Is(err, sql.ErrNoRows) {
			return pluginsdk.Receipt{}, nil
		}
		if err != nil {
			return pluginsdk.Receipt{}, err
		}
		if method != input.Method || digest != input.InputDigest {
			return pluginsdk.Receipt{}, idempotency.ErrKeyConflict
		}
		return pluginsdk.Receipt{Found: true, Reply: []byte(reply)}, nil
	})
}
