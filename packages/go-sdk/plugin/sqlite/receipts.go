package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"unicode/utf8"

	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/qimu/sqlite"
	sdk "github.com/Tangerg/go-sdk/mcp"
)

const CommandsSchema = `CREATE TABLE IF NOT EXISTS commands (id TEXT PRIMARY KEY, method TEXT NOT NULL, business_input_json TEXT NOT NULL CHECK(json_valid(business_input_json)), reply TEXT CHECK(reply IS NULL OR json_valid(reply)))`

var ErrCommandUnconfirmed = errors.New("managed plugin: command outcome is unconfirmed")

func Command[P, R any](ctx context.Context, db *sql.DB, method, id string, input P, admitted jsonvalue.Value, classify func(error) string, handler func(context.Context, P) (R, error)) (R, error) {
	var zero R
	if id == "" || len(id) > 256 || !utf8.ValidString(id) {
		return zero, errors.New("managed plugin: bounded command identity is required")
	}
	if err := admitted.Validate(); err != nil {
		return zero, err
	}
	var reply []byte
	err := sqlite.RunInTx(ctx, db, func(ctx context.Context) error {
		connection := sqlite.Connection(ctx, db)
		var previousMethod, previousInput string
		err := connection.QueryRowContext(ctx, `SELECT method,business_input_json,reply FROM commands WHERE id=?`, id).Scan(&previousMethod, &previousInput, &reply)
		if err == nil {
			if previousMethod != method || previousInput != admitted.String() {
				return portable.ErrInvocationConflict
			}
			if len(reply) == 0 {
				return ErrCommandUnconfirmed
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
		outcome, err := qimumcp.EncodeReply(value, operationErr, classify)
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
		reply = outcome.JSON()
		_, err = connection.ExecContext(ctx, `INSERT INTO commands(id,method,business_input_json,reply) VALUES(?,?,?,?)`, id, method, admitted.String(), string(reply))
		return err
	})
	if err != nil {
		return zero, err
	}
	return qimumcp.DecodeReply[R](reply)
}

func RegisterCommand[P, R any](server *qimumcp.Backend, db *sql.DB, method *qimumcp.Method[P, R], classify func(error) string, handler func(context.Context, *sdk.ServerSession, P) (R, error)) error {
	if method == nil || method.Descriptor().Kind != portable.Mutation {
		return errors.New("managed plugin: command method is required")
	}
	return method.RegisterServer(server, classify, func(ctx context.Context, session *sdk.ServerSession, command qimumcp.Command[P]) (R, error) {
		admitted, err := command.RequestInput()
		if err != nil {
			var zero R
			return zero, err
		}
		return Command(ctx, db, method.Name(), command.ID, command.Input, admitted, classify, func(ctx context.Context, input P) (R, error) { return handler(ctx, session, input) })
	})
}

func RegisterReceipts(server *qimumcp.Backend, db *sql.DB) error {
	method, err := qimumcp.ReceiptQuery()
	if err != nil {
		return err
	}
	return method.RegisterServer(server, func(err error) string {
		if errors.Is(err, portable.ErrInvocationConflict) {
			return "command_conflict"
		}
		return ""
	}, func(ctx context.Context, _ *sdk.ServerSession, command qimumcp.Command[qimumcp.ReceiptRequest]) (qimumcp.Receipt, error) {
		return ReadReceipt(ctx, db, command.Input)
	})
}

// ReadReceipt only projects the private owner's original method/input evidence.
func ReadReceipt(ctx context.Context, db *sql.DB, input qimumcp.ReceiptRequest) (qimumcp.Receipt, error) {
	if err := input.Validate(); err != nil {
		return qimumcp.Receipt{}, err
	}
	var method, encoded string
	var reply []byte
	err := sqlite.Connection(ctx, db).QueryRowContext(ctx, `SELECT method,business_input_json,reply FROM commands WHERE id=?`, input.ID).Scan(&method, &encoded, &reply)
	if errors.Is(err, sql.ErrNoRows) {
		return qimumcp.Receipt{}, nil
	}
	if err != nil {
		return qimumcp.Receipt{}, err
	}
	admitted, err := jsonvalue.Parse([]byte(encoded))
	if err != nil {
		return qimumcp.Receipt{}, err
	}
	if method != input.Method || admitted.Digest() != input.InputDigest {
		return qimumcp.Receipt{}, portable.ErrInvocationConflict
	}
	return qimumcp.Receipt{Found: len(reply) != 0, Reply: reply}, nil
}
