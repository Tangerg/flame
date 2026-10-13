package managedmcp

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"unicode/utf8"

	"github.com/Tangerg/flame/runtime/internal/idempotency"
	sqlitedb "github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/jsoninput"
	pluginsdk "github.com/Tangerg/flame/runtime/plugin"
)

// PreparedCommand is a projection of a durable private command admission.
// Model or other external I/O cannot be repeated merely because no final reply
// exists. The actual mutation owner completes it in its own short transaction.
type PreparedCommand struct {
	db         *sql.DB
	id, method string
	input      jsoninput.Input
	fresh      bool
}

func AdmitCommand(ctx context.Context, db *sql.DB, method, id string, input jsoninput.Input) (*PreparedCommand, []byte, error) {
	if db == nil || id == "" || len(id) > 256 || !utf8.ValidString(id) || method == "" || !utf8.ValidString(method) {
		return nil, nil, errors.New("managed plugin: database, method and bounded command identity are required")
	}
	if err := input.Validate(); err != nil {
		return nil, nil, err
	}
	var reply []byte
	started := false
	err := sqlitedb.RunInTx(ctx, db, func(ctx context.Context) error {
		var previousMethod, previousInput string
		err := sqlitedb.Connection(ctx, db).QueryRowContext(ctx, `SELECT method,input_json,reply FROM commands WHERE id=?`, id).Scan(&previousMethod, &previousInput, &reply)
		if err == nil {
			if previousMethod != method || previousInput != input.String() {
				return idempotency.ErrKeyConflict
			}
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := sqlitedb.Connection(ctx, db).ExecContext(ctx, `INSERT INTO commands(id,method,input_json,reply) VALUES(?,?,?,NULL)`, id, method, input.String()); err != nil {
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

// NewlyAdmitted authorizes the first effect attempt. A pending admission requires
// owner-provided reconciliation evidence before its caller can proceed.
func (c *PreparedCommand) NewlyAdmitted() bool { return c != nil && c.fresh }

func (c *PreparedCommand) Complete(ctx context.Context, reply []byte) error {
	if c == nil {
		return errors.New("managed plugin: prepared command is required")
	}
	if _, err := pluginsdk.DecodeReply[jsoninput.Input](reply); err != nil {
		if _, known := errors.AsType[*pluginsdk.Failure](err); !known {
			return err
		}
	}
	return sqlitedb.RunInTx(ctx, c.db, func(ctx context.Context) error {
		var method, input string
		var original []byte
		if err := sqlitedb.Connection(ctx, c.db).QueryRowContext(ctx, `SELECT method,input_json,reply FROM commands WHERE id=?`, c.id).Scan(&method, &input, &original); err != nil {
			return err
		}
		if method != c.method || input != c.input.String() {
			return idempotency.ErrKeyConflict
		}
		if len(original) != 0 {
			if !bytes.Equal(original, reply) {
				return idempotency.ErrKeyConflict
			}
			return nil
		}
		_, err := sqlitedb.Connection(ctx, c.db).ExecContext(ctx, `UPDATE commands SET reply=? WHERE id=? AND reply IS NULL`, string(reply), c.id)
		return err
	})
}
