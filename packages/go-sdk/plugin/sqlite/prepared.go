package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"time"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"

	framecall "github.com/Tangerg/frame/call"

	framesqlite "github.com/Tangerg/frame/sqlite"
	framevalue "github.com/Tangerg/frame/value"
)

type PreparedCommand struct {
	db         framesqlite.Database
	id, method string
	input      framevalue.JSON
	fresh      bool
}

func AdmitCommand(ctx context.Context, db framesqlite.Database, method, id string, input framevalue.JSON, validateNew func(context.Context) error) (*PreparedCommand, []byte, error) {
	if err := validateCommandIdentity(db, method, id); err != nil {
		return nil, nil, err
	}
	if err := input.Validate(); err != nil {
		return nil, nil, err
	}
	var reply []byte
	started := false
	err := db.RunInTx(ctx, func(ctx context.Context) error {
		stored, err := readStoredCommand(ctx, db.Executor(ctx), id)
		if err == nil {
			if !stored.admits(method, input) {
				return framecall.ErrConflict
			}
			reply = stored.reply
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if validateNew != nil {
			if err := validateNew(ctx); err != nil {
				return err
			}
		}
		if _, err := db.Executor(ctx).ExecContext(ctx, `INSERT INTO commands(id,method,business_input_json,reply) VALUES(?,?,?,NULL)`, id, method, input.String()); err != nil {
			return err
		}
		started = true
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if !started && len(reply) != 0 {
		return nil, reply, nil
	}
	return &PreparedCommand{db: db, id: id, method: method, input: input, fresh: started}, nil, nil
}

func ResumeCommand(ctx context.Context, db framesqlite.Database, method, id string) (*PreparedCommand, framevalue.JSON, []byte, error) {
	if err := validateCommandIdentity(db, method, id); err != nil {
		return nil, framevalue.JSON{}, nil, err
	}
	var stored storedCommand
	if err := db.Read(ctx, func(reader framesqlite.Reader) error {
		var err error
		stored, err = readStoredCommand(ctx, reader, id)
		return err
	}); err != nil {
		return nil, framevalue.JSON{}, nil, err
	}
	if stored.method != method {
		return nil, framevalue.JSON{}, nil, framecall.ErrConflict
	}
	input, err := framevalue.ParseJSON([]byte(stored.input))
	if err != nil {
		return nil, framevalue.JSON{}, nil, err
	}
	if input.String() != stored.input {
		return nil, framevalue.JSON{}, nil, errors.New("managed plugin: retained input is not canonical")
	}
	if len(stored.reply) != 0 {
		return nil, input, stored.reply, nil
	}
	return &PreparedCommand{db: db, id: id, method: method, input: input}, input, nil, nil
}

func (c *PreparedCommand) NewlyAdmitted() bool { return c != nil && c.fresh }

const externalSettlementTimeout = 10 * time.Second

func (c *PreparedCommand) SettleExternal(ctx context.Context, reply []byte, record func(context.Context) error) error {
	settlement, cancel := context.WithTimeout(context.WithoutCancel(ctx), externalSettlementTimeout)
	defer cancel()
	return c.complete(settlement, reply, record, framesqlite.Database.RunIndependentTx)
}

func (c *PreparedCommand) Complete(ctx context.Context, reply []byte) error {
	return c.complete(ctx, reply, nil, framesqlite.Database.RunInTx)
}

type replyTransaction func(framesqlite.Database, context.Context, func(context.Context) error) error

func (c *PreparedCommand) complete(ctx context.Context, reply []byte, record func(context.Context) error, transact replyTransaction) error {
	if c == nil {
		return errors.New("managed plugin: prepared command is required")
	}
	if err := validateCommandIdentity(c.db, c.method, c.id); err != nil {
		return err
	}
	if _, err := pluginmcp.DecodeReply[framevalue.JSON](reply); err != nil {
		if _, known := errors.AsType[*framecall.Refusal](err); !known {
			return err
		}
	}
	err := transact(c.db, ctx, func(ctx context.Context) error {
		original, err := c.readReply(ctx)
		if err != nil {
			return err
		}
		if len(original) != 0 {
			if !bytes.Equal(original, reply) {
				return framecall.ErrConflict
			}
			return nil
		}
		if record != nil {
			if err := record(ctx); err != nil {
				return err
			}
		}
		return c.storeReply(ctx, reply)
	})
	if err != nil {
		return errors.Join(pluginmcp.ErrOutcomeUnconfirmed, err)
	}
	return nil
}

func (c *PreparedCommand) readReply(ctx context.Context) ([]byte, error) {
	stored, err := readStoredCommand(ctx, c.db.Executor(ctx), c.id)
	if err != nil {
		return nil, err
	}
	if !stored.admits(c.method, c.input) {
		return nil, framecall.ErrConflict
	}
	return stored.reply, nil
}

func (c *PreparedCommand) storeReply(ctx context.Context, reply []byte) error {
	result, err := c.db.Executor(ctx).ExecContext(ctx, `UPDATE commands SET reply=? WHERE id=? AND reply IS NULL`, string(reply), c.id)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return errors.New("managed plugin: command admission changed inside its settlement transaction")
	}
	return nil
}

func CompleteExternalCommand[R any](ctx context.Context, command *PreparedCommand, classify pluginmcp.Classifier, prepare func(context.Context) (CommandPlan[R], error)) (R, error) {
	settlement, cancel := context.WithTimeout(context.WithoutCancel(ctx), externalSettlementTimeout)
	defer cancel()
	return CompleteCommand(settlement, command, classify, prepare)
}

func CompleteCommand[R any](ctx context.Context, command *PreparedCommand, classify pluginmcp.Classifier, prepare func(context.Context) (CommandPlan[R], error)) (R, error) {
	var zero R
	if command == nil || prepare == nil {
		return zero, errors.New("managed plugin: prepared command and preparation are required")
	}
	if err := validateCommandIdentity(command.db, command.method, command.id); err != nil {
		return zero, err
	}
	var reply []byte
	err := command.db.RunIndependentTx(ctx, func(ctx context.Context) error {
		var err error
		reply, err = command.readReply(ctx)
		if err != nil || len(reply) != 0 {
			return err
		}
		reply, err = executeCommandPlan(ctx, command.db, classify, prepare)
		if err != nil {
			return err
		}
		return command.storeReply(ctx, reply)
	})
	if err != nil {
		return zero, errors.Join(pluginmcp.ErrOutcomeUnconfirmed, err)
	}
	return pluginmcp.DecodeReply[R](reply)
}
