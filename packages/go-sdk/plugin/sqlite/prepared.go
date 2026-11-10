package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"time"

	framemcp "github.com/Tangerg/frame/process/mcp"
	framevalue "github.com/Tangerg/frame/value"

	"github.com/Tangerg/frame/sqlite"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
)

// PreparedCommand is a projection of a durable private command admission.
// Model or other external I/O cannot be repeated merely because no final reply
// exists. The actual mutation owner completes it in its own short transaction.
type PreparedCommand struct {
	db         *sqlite.Database
	id, method string
	input      framevalue.JSON
	fresh      bool
}

// validateNew checks only local preconditions before the first durable admission.
// Existing pending work and receipts must reconcile without acquiring new collaborators.
func AdmitCommand(ctx context.Context, db *sqlite.Database, method, id string, input framevalue.JSON, validateNew func(context.Context) error) (*PreparedCommand, []byte, error) {
	if err := validateCommandIdentity(db, method, id); err != nil {
		return nil, nil, err
	}
	if err := input.Validate(); err != nil {
		return nil, nil, err
	}
	var reply []byte
	started := false
	err := sqlite.RunInTx(ctx, db, func(ctx context.Context) error {
		var previousMethod, previousInput string
		err := sqlite.Connection(ctx, db).QueryRowContext(ctx, `SELECT method,business_input_json,reply FROM commands WHERE id=?`, id).Scan(&previousMethod, &previousInput, &reply)
		if err == nil {
			if previousMethod != method || previousInput != input.String() {
				return framemcp.ErrInvocationConflict
			}
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
		if _, err := sqlite.Connection(ctx, db).ExecContext(ctx, `INSERT INTO commands(id,method,business_input_json,reply) VALUES(?,?,?,NULL)`, id, method, input.String()); err != nil {
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

// ResumeCommand reads original private input and admission without creating work.
func ResumeCommand(ctx context.Context, db *sqlite.Database, method, id string) (*PreparedCommand, framevalue.JSON, []byte, error) {
	if err := validateCommandIdentity(db, method, id); err != nil {
		return nil, framevalue.JSON{}, nil, err
	}
	var storedMethod, raw string
	var reply []byte
	if err := db.Read(ctx, func(reader sqlite.Reader) error {
		return reader.QueryRowContext(ctx, `SELECT method,business_input_json,reply FROM commands WHERE id=?`, id).Scan(&storedMethod, &raw, &reply)
	}); err != nil {
		return nil, framevalue.JSON{}, nil, err
	}
	if storedMethod != method {
		return nil, framevalue.JSON{}, nil, framemcp.ErrInvocationConflict
	}
	input, err := framevalue.ParseJSON([]byte(raw))
	if err != nil {
		return nil, framevalue.JSON{}, nil, err
	}
	if input.String() != raw {
		return nil, framevalue.JSON{}, nil, errors.New("managed plugin: retained input is not canonical")
	}
	if len(reply) != 0 {
		return nil, input, reply, nil
	}
	return &PreparedCommand{db: db, id: id, method: method, input: input}, input, nil, nil
}

// NewlyAdmitted authorizes the first effect attempt. A pending admission requires
// owner-provided reconciliation evidence before its caller can proceed.
func (c *PreparedCommand) NewlyAdmitted() bool { return c != nil && c.fresh }

// An upstream acknowledgement survives cancellation of the request which obtained
// it. Its local write-set still has a finite lifetime if storage is unavailable.
const externalSettlementTimeout = 10 * time.Second

func (c *PreparedCommand) SettleExternal(ctx context.Context, reply []byte, record func(context.Context) error) error {
	settlement, cancel := context.WithTimeout(context.WithoutCancel(ctx), externalSettlementTimeout)
	defer cancel()
	return c.complete(settlement, reply, record, sqlite.RunIndependentTx)
}

func (c *PreparedCommand) Complete(ctx context.Context, reply []byte) error {
	return c.complete(ctx, reply, nil, sqlite.RunInTx)
}

type replyTransaction func(context.Context, *sqlite.Database, func(context.Context) error) error

func (c *PreparedCommand) complete(ctx context.Context, reply []byte, record func(context.Context) error, transact replyTransaction) error {
	if c == nil {
		return errors.New("managed plugin: prepared command is required")
	}
	if err := validateCommandIdentity(c.db, c.method, c.id); err != nil {
		return err
	}
	if _, err := pluginmcp.DecodeReply[framevalue.JSON](reply); err != nil {
		if _, known := errors.AsType[*framemcp.Refusal](err); !known {
			return err
		}
	}
	err := transact(ctx, c.db, func(ctx context.Context) error {
		original, err := c.readReply(ctx)
		if err != nil {
			return err
		}
		if len(original) != 0 {
			if !bytes.Equal(original, reply) {
				return framemcp.ErrInvocationConflict
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
	var method, input string
	var reply []byte
	if err := sqlite.Connection(ctx, c.db).QueryRowContext(ctx, `SELECT method,business_input_json,reply FROM commands WHERE id=?`, c.id).Scan(&method, &input, &reply); err != nil {
		return nil, err
	}
	if method != c.method || input != c.input.String() {
		return nil, framemcp.ErrInvocationConflict
	}
	return reply, nil
}

func (c *PreparedCommand) storeReply(ctx context.Context, reply []byte) error {
	result, err := sqlite.Connection(ctx, c.db).ExecContext(ctx, `UPDATE commands SET reply=? WHERE id=? AND reply IS NULL`, string(reply), c.id)
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

// CompleteCommand decides local publication after external work. The original
// pending admission remains the only receipt owner and prevents a second attempt.
func CompleteCommand[R any](ctx context.Context, command *PreparedCommand, classify pluginmcp.Classifier, prepare func(context.Context) (CommandPlan[R], error)) (R, error) {
	var zero R
	if command == nil || prepare == nil {
		return zero, errors.New("managed plugin: prepared command and preparation are required")
	}
	if err := validateCommandIdentity(command.db, command.method, command.id); err != nil {
		return zero, err
	}
	var reply []byte
	err := sqlite.RunIndependentTx(ctx, command.db, func(ctx context.Context) error {
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
