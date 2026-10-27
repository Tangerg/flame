package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"unicode/utf8"

	"github.com/Tangerg/flame/qimu/jsonvalue"
	qimumcp "github.com/Tangerg/flame/qimu/mcp"
	"github.com/Tangerg/flame/qimu/portable"
	"github.com/Tangerg/flame/qimu/sqlite"
)

// PreparedCommand is a projection of a durable private command admission.
// Model or other external I/O cannot be repeated merely because no final reply
// exists. The actual mutation owner completes it in its own short transaction.
type PreparedCommand struct {
	db         *sql.DB
	id, method string
	input      jsonvalue.Value
	fresh      bool
}

// validateNew checks only local preconditions before the first durable admission.
// Existing pending work and receipts must reconcile without acquiring new collaborators.
func AdmitCommand(ctx context.Context, db *sql.DB, method, id string, input jsonvalue.Value, validateNew func(context.Context) error) (*PreparedCommand, []byte, error) {
	if db == nil || id == "" || len(id) > 256 || !utf8.ValidString(id) || method == "" || !utf8.ValidString(method) {
		return nil, nil, errors.New("managed plugin: database, method and bounded command identity are required")
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
				return portable.ErrInvocationConflict
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
func ResumeCommand(ctx context.Context, db *sql.DB, method, id string) (*PreparedCommand, jsonvalue.Value, []byte, error) {
	var storedMethod, raw string
	var reply []byte
	if err := sqlite.Connection(ctx, db).QueryRowContext(ctx, `SELECT method,business_input_json,reply FROM commands WHERE id=?`, id).Scan(&storedMethod, &raw, &reply); err != nil {
		return nil, jsonvalue.Value{}, nil, err
	}
	if storedMethod != method {
		return nil, jsonvalue.Value{}, nil, portable.ErrInvocationConflict
	}
	input, err := jsonvalue.Parse([]byte(raw))
	if err != nil {
		return nil, jsonvalue.Value{}, nil, err
	}
	if input.String() != raw {
		return nil, jsonvalue.Value{}, nil, errors.New("managed plugin: retained input is not canonical")
	}
	if len(reply) != 0 {
		return nil, input, reply, nil
	}
	return &PreparedCommand{db: db, id: id, method: method, input: input}, input, nil, nil
}

// NewlyAdmitted authorizes the first effect attempt. A pending admission requires
// owner-provided reconciliation evidence before its caller can proceed.
func (c *PreparedCommand) NewlyAdmitted() bool { return c != nil && c.fresh }

func (c *PreparedCommand) Complete(ctx context.Context, reply []byte) error {
	if c == nil {
		return errors.New("managed plugin: prepared command is required")
	}
	if _, err := qimumcp.DecodeReply[jsonvalue.Value](reply); err != nil {
		if _, known := errors.AsType[*portable.Refusal](err); !known {
			return err
		}
	}
	return sqlite.RunInTx(ctx, c.db, func(ctx context.Context) error {
		var method, input string
		var original []byte
		if err := sqlite.Connection(ctx, c.db).QueryRowContext(ctx, `SELECT method,business_input_json,reply FROM commands WHERE id=?`, c.id).Scan(&method, &input, &original); err != nil {
			return err
		}
		if method != c.method || input != c.input.String() {
			return portable.ErrInvocationConflict
		}
		if len(original) != 0 {
			if !bytes.Equal(original, reply) {
				return portable.ErrInvocationConflict
			}
			return nil
		}
		_, err := sqlite.Connection(ctx, c.db).ExecContext(ctx, `UPDATE commands SET reply=? WHERE id=? AND reply IS NULL`, string(reply), c.id)
		return err
	})
}
