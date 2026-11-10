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
	framesqlite "github.com/Tangerg/frame/sqlite"
	framevalue "github.com/Tangerg/frame/value"

	sdk "github.com/Tangerg/go-sdk/mcp"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
)

const CommandsSchema = `CREATE TABLE IF NOT EXISTS commands (id TEXT PRIMARY KEY, method TEXT NOT NULL, business_input_json TEXT NOT NULL CHECK(json_valid(business_input_json)), reply TEXT CHECK(reply IS NULL OR json_valid(reply)))`

var ErrCommandUnconfirmed = errors.New("managed plugin: command outcome is unconfirmed")

// CommandPlan captures the admitted reply before any mutation can begin. Its
// callback only persists that decision; a write error cannot become a refusal.
type CommandPlan[R any] struct {
	reply framevalue.JSON
	apply func(context.Context) error
}

func NewCommandPlan[R any](result R, apply func(context.Context) error) (CommandPlan[R], error) {
	if err := framevalue.ValidateType(reflect.TypeFor[R]()); err != nil {
		return CommandPlan[R]{}, errors.Join(pluginmcp.ErrOutcomeUnconfirmed, err)
	}
	reply, err := pluginmcp.EncodeReply(result, nil, nil)
	if err != nil {
		return CommandPlan[R]{}, errors.Join(pluginmcp.ErrOutcomeUnconfirmed, err)
	}
	return CommandPlan[R]{reply: reply, apply: apply}, nil
}

func (p CommandPlan[R]) validateOutput(schema portable.Schema) error {
	value, err := pluginmcp.DecodeReply[framevalue.JSON](p.reply.Bytes())
	if err != nil {
		return err
	}
	return schema.Validate(value.Bytes())
}

func Command[P, R any](ctx context.Context, db *framesqlite.Database, method, id string, admitted framevalue.JSON, classify pluginmcp.Classifier, prepare func(context.Context, P) (CommandPlan[R], error)) (R, error) {
	var zero R
	if err := validateCommandIdentity(db, method, id); err != nil {
		return zero, err
	}
	if prepare == nil {
		return zero, errors.New("managed plugin: command preparation is required")
	}
	if err := admitted.Validate(); err != nil {
		return zero, err
	}
	var reply []byte
	err := framesqlite.RunIndependentTx(ctx, db, func(ctx context.Context) error {
		connection := framesqlite.Connection(ctx, db)
		var previousMethod, previousInput string
		err := connection.QueryRowContext(ctx, `SELECT method,business_input_json,reply FROM commands WHERE id=?`, id).Scan(&previousMethod, &previousInput, &reply)
		if err == nil {
			if previousMethod != method || previousInput != admitted.String() {
				conflict, err := pluginmcp.EncodeReply(zero, framemcp.ErrInvocationConflict, nil)
				reply = conflict.Bytes()
				return err
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
		reply, err = executeCommandPlan(ctx, db, classify, func(ctx context.Context) (CommandPlan[R], error) {
			return prepare(ctx, input)
		})
		if err != nil {
			return err
		}
		_, err = connection.ExecContext(ctx, `INSERT INTO commands(id,method,business_input_json,reply) VALUES(?,?,?,?)`, id, method, admitted.String(), string(reply))
		return err
	})
	if err != nil {
		if errors.Is(err, framesqlite.ErrIndeterminate) {
			return zero, errors.Join(pluginmcp.ErrOutcomeUnconfirmed, ErrCommandUnconfirmed, err)
		}
		return zero, errors.Join(pluginmcp.ErrOutcomeUnconfirmed, err)
	}
	return pluginmcp.DecodeReply[R](reply)
}

func executeCommandPlan[R any](ctx context.Context, db *framesqlite.Database, classify pluginmcp.Classifier, prepare func(context.Context) (CommandPlan[R], error)) ([]byte, error) {
	var plan CommandPlan[R]
	var decisionErr error
	if err := framesqlite.RunReadSnapshot(ctx, db, func(ctx context.Context) error {
		plan, decisionErr = prepare(ctx)
		return nil
	}); err != nil {
		return nil, err
	}
	if decisionErr != nil {
		var zero R
		outcome, err := pluginmcp.EncodeReply(zero, decisionErr, classify)
		return outcome.Bytes(), err
	}
	if err := plan.reply.Validate(); err != nil {
		return nil, fmt.Errorf("managed plugin: invalid command plan: %w", err)
	}
	if plan.apply != nil {
		if err := plan.apply(ctx); err != nil {
			return nil, err
		}
	}
	return plan.reply.Bytes(), nil
}

func RegisterCommand[P, R any](server *pluginmcp.Backend, db *framesqlite.Database, method *pluginmcp.Method[P, R], classify pluginmcp.Classifier, prepare func(context.Context, *sdk.ServerSession, P) (CommandPlan[R], error)) error {
	if method == nil || method.Descriptor().Kind != portable.Command {
		return errors.New("managed plugin: command method is required")
	}
	if server == nil || db == nil || prepare == nil {
		return errors.New("managed plugin: backend, database and command preparation are required")
	}
	output, err := portable.AdmitSchema(method.Descriptor().Output)
	if err != nil {
		return err
	}
	return method.RegisterServer(server, nil, func(ctx context.Context, session *sdk.ServerSession, command pluginmcp.Command[P]) (R, error) {
		admitted, err := command.RequestInput()
		if err != nil {
			var zero R
			return zero, err
		}
		return Command(ctx, db, method.Name(), string(command.ID), admitted, classify, func(ctx context.Context, input P) (CommandPlan[R], error) {
			plan, err := prepare(ctx, session, input)
			if err != nil {
				return CommandPlan[R]{}, err
			}
			if err := plan.validateOutput(output); err != nil {
				return CommandPlan[R]{}, errors.Join(pluginmcp.ErrOutcomeUnconfirmed, err)
			}
			return plan, nil
		})
	})
}

func validateCommandIdentity(db *framesqlite.Database, method, id string) error {
	if db == nil || !portable.ValidToken(method) {
		return errors.New("managed plugin: database and valid method identity are required")
	}
	if err := pluginmcp.CommandID(id).Validate(); err != nil {
		return fmt.Errorf("managed plugin: %w", err)
	}
	return nil
}

func RegisterReceipts(server *pluginmcp.Backend, db *framesqlite.Database) error {
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
func ReadReceipt(ctx context.Context, db *framesqlite.Database, input pluginmcp.ReceiptRequest) (pluginmcp.Receipt, error) {
	if db == nil {
		return pluginmcp.Receipt{}, errors.New("managed plugin: database is required")
	}
	if err := input.Validate(); err != nil {
		return pluginmcp.Receipt{}, err
	}
	var method, encoded string
	var reply []byte
	err := db.Read(ctx, func(reader framesqlite.Reader) error {
		return reader.QueryRowContext(ctx, `SELECT method,business_input_json,reply FROM commands WHERE id=?`, input.ID).Scan(&method, &encoded, &reply)
	})
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
