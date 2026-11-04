package sqlite

import (
	"context"
	"database/sql"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"reflect"

	"github.com/Tangerg/frame/portable"
	framemcp "github.com/Tangerg/frame/process/mcp"
	framevalue "github.com/Tangerg/frame/value"

	sdk "github.com/Tangerg/go-sdk/mcp"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
)

const CommandsSchema = `CREATE TABLE IF NOT EXISTS commands (id TEXT PRIMARY KEY, method TEXT NOT NULL, business_input_json TEXT NOT NULL CHECK(json_valid(business_input_json)), reply TEXT CHECK(reply IS NULL OR json_valid(reply)))`

var ErrCommandUnconfirmed = errors.New("managed plugin: command outcome is unconfirmed")

func Command[P, R any](ctx context.Context, db *sql.DB, method, id string, admitted framevalue.JSON, classify pluginmcp.Classifier, handler func(context.Context, P) (R, error)) (R, error) {
	var zero R
	if err := validateCommandIdentity(db, method, id); err != nil {
		return zero, err
	}
	if handler == nil {
		return zero, errors.New("managed plugin: command handler is required")
	}
	if err := admitted.Validate(); err != nil {
		return zero, err
	}
	var reply []byte
	err := runCommandTransaction(ctx, db, func(ctx context.Context, transaction *sql.Tx) error {
		connection := Connection(ctx, db)
		var previousMethod, previousInput string
		err := connection.QueryRowContext(ctx, `SELECT method,business_input_json,reply FROM commands WHERE id=?`, id).Scan(&previousMethod, &previousInput, &reply)
		if err == nil {
			if previousMethod != method || previousInput != admitted.String() {
				return framemcp.ErrInvocationConflict
			}
			if len(reply) == 0 {
				return ErrCommandUnconfirmed
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		// The retained request is the only input authority; accepting a separate
		// typed argument would allow effects that disagree with their receipt.
		var input P
		if err := framevalue.ValidateType(reflect.TypeFor[P]()); err != nil {
			return err
		}
		if err := json.Unmarshal(admitted.Bytes(), &input, json.RejectUnknownMembers(true), framevalue.Numbers()); err != nil {
			return err
		}
		if _, err := transaction.ExecContext(ctx, `SAVEPOINT command_mutation`); err != nil {
			return err
		}
		value, operationErr := handler(ctx, input)
		outcome, err := pluginmcp.EncodeReply(value, operationErr, classify)
		if err != nil {
			return err
		}
		if operationErr != nil {
			if _, err := transaction.ExecContext(ctx, `ROLLBACK TO command_mutation`); err != nil {
				return err
			}
		}
		if _, err := transaction.ExecContext(ctx, `RELEASE command_mutation`); err != nil {
			return err
		}
		reply = outcome.Bytes()
		_, err = connection.ExecContext(ctx, `INSERT INTO commands(id,method,business_input_json,reply) VALUES(?,?,?,?)`, id, method, admitted.String(), string(reply))
		return err
	})
	if err != nil {
		return zero, err
	}
	return pluginmcp.DecodeReply[R](reply)
}

func RegisterCommand[P, R any](server *pluginmcp.Backend, db *sql.DB, method *pluginmcp.Method[P, R], classify pluginmcp.Classifier, handler func(context.Context, *sdk.ServerSession, P) (R, error)) error {
	if method == nil || method.Descriptor().Kind != portable.Command {
		return errors.New("managed plugin: command method is required")
	}
	if server == nil || db == nil || handler == nil {
		return errors.New("managed plugin: backend, database and command handler are required")
	}
	return method.RegisterServer(server, classify, func(ctx context.Context, session *sdk.ServerSession, command pluginmcp.Command[P]) (R, error) {
		admitted, err := command.RequestInput()
		if err != nil {
			var zero R
			return zero, err
		}
		return Command(ctx, db, method.Name(), string(command.ID), admitted, classify, func(ctx context.Context, input P) (R, error) { return handler(ctx, session, input) })
	})
}

func validateCommandIdentity(db *sql.DB, method, id string) error {
	if db == nil || !portable.ValidToken(method) {
		return errors.New("managed plugin: database and valid method identity are required")
	}
	if err := pluginmcp.CommandID(id).Validate(); err != nil {
		return fmt.Errorf("managed plugin: %w", err)
	}
	return nil
}

func RegisterReceipts(server *pluginmcp.Backend, db *sql.DB) error {
	if server == nil || db == nil {
		return errors.New("managed plugin: backend and database are required")
	}
	method, err := pluginmcp.ReceiptQuery()
	if err != nil {
		return err
	}
	return method.RegisterServer(server, nil, func(ctx context.Context, _ *sdk.ServerSession, command pluginmcp.Command[pluginmcp.ReceiptRequest]) (pluginmcp.Receipt, error) {
		return ReadReceipt(ctx, db, command.Input)
	})
}

// ReadReceipt only projects the private owner's original method/input evidence.
func ReadReceipt(ctx context.Context, db *sql.DB, input pluginmcp.ReceiptRequest) (pluginmcp.Receipt, error) {
	if db == nil {
		return pluginmcp.Receipt{}, errors.New("managed plugin: database is required")
	}
	if err := input.Validate(); err != nil {
		return pluginmcp.Receipt{}, err
	}
	var method, encoded string
	var reply []byte
	err := Connection(ctx, db).QueryRowContext(ctx, `SELECT method,business_input_json,reply FROM commands WHERE id=?`, input.ID).Scan(&method, &encoded, &reply)
	if errors.Is(err, sql.ErrNoRows) {
		return pluginmcp.Receipt{}, nil
	}
	if err != nil {
		return pluginmcp.Receipt{}, err
	}
	admitted, err := framevalue.ParseJSON([]byte(encoded))
	if err != nil {
		return pluginmcp.Receipt{}, err
	}
	if method != input.Method || admitted.Digest() != input.InputDigest {
		return pluginmcp.Receipt{}, framemcp.ErrInvocationConflict
	}
	return pluginmcp.Receipt{Found: len(reply) != 0, Reply: reply}, nil
}
