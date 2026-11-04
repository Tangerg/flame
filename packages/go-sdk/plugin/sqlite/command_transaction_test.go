package sqlite

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"

	framemcp "github.com/Tangerg/frame/process/mcp"
	framesqlite "github.com/Tangerg/frame/sqlite"
)

func commandDatabase(t *testing.T) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "command.db")
	db, err := framesqlite.OpenSchema(t.Context(), path, []string{
		CommandsSchema, `CREATE TABLE counter (value INTEGER NOT NULL)`, `INSERT INTO counter VALUES(0)`,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, path
}

func assertCommandState(t *testing.T, db *sql.DB, counter, receipts int) {
	t.Helper()
	var actualCounter, actualReceipts int
	if err := db.QueryRowContext(t.Context(), `SELECT value FROM counter`).Scan(&actualCounter); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM commands`).Scan(&actualReceipts); err != nil {
		t.Fatal(err)
	}
	if actualCounter != counter || actualReceipts != receipts {
		t.Fatalf("business/receipt state = %d/%d, want %d/%d", actualCounter, actualReceipts, counter, receipts)
	}
}

func TestCommandCancellationRollsBackOriginalWriteSetAndReceipt(t *testing.T) {
	db, _ := commandDatabase(t)
	ctx, cancel := context.WithCancelCause(t.Context())
	cause := errors.New("caller withdrew command")
	value, err := Command(ctx, db, "save", "cancelled", receiptInput(t, "input"), nil, func(ctx context.Context, _ string) (bool, error) {
		err := WithinTransaction(ctx, db, func(ctx context.Context) error {
			_, err := Connection(ctx, db).ExecContext(ctx, `UPDATE counter SET value=1`)
			return err
		})
		cancel(cause)
		return true, err
	})
	if value || err == nil {
		t.Fatalf("cancelled command published success: %t, %v", value, err)
	}
	if _, known := errors.AsType[*framemcp.Refusal](err); known {
		t.Fatalf("cancelled commit became a known business refusal: %v", err)
	}
	assertCommandState(t, db, 0, 0)
}

func TestCommandBorrowCannotOutliveOrCrossItsOriginalTransaction(t *testing.T) {
	db, _ := commandDatabase(t)
	other, _ := commandDatabase(t)
	var retained context.Context
	var borrowed commandConnection
	_, err := Command(t.Context(), db, "save", "original", receiptInput(t, "input"), nil, func(ctx context.Context, _ string) (bool, error) {
		retained, borrowed = ctx, Connection(ctx, db)
		if _, err := Connection(ctx, other).ExecContext(ctx, `UPDATE counter SET value=9`); err == nil {
			t.Fatal("command borrowed a different database")
		}
		if err := WithinTransaction(ctx, other, func(context.Context) error {
			t.Fatal("foreign database admitted repository work")
			return nil
		}); err == nil {
			t.Fatal("foreign transaction was admitted")
		}
		if _, err := Command(ctx, other, "save", "nested", receiptInput(t, "input"), nil, func(context.Context, string) (bool, error) {
			t.Fatal("a nested command reached its handler")
			return true, nil
		}); err == nil {
			t.Fatal("command entered an independent receipt transaction")
		}
		_, err := borrowed.ExecContext(ctx, `UPDATE counter SET value=1`)
		return true, err
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := borrowed.ExecContext(t.Context(), `UPDATE counter SET value=2`); err == nil {
		t.Fatal("retained executor changed a committed command")
	}
	if err := Connection(retained, db).QueryRowContext(t.Context(), `SELECT value FROM counter`).Err(); err == nil {
		t.Fatal("retained context reacquired command SQL authority")
	}
	if err := WithinTransaction(retained, db, func(context.Context) error {
		t.Fatal("expired command context entered repository work")
		return nil
	}); err == nil {
		t.Fatal("expired command scope was admitted")
	}
	assertCommandState(t, db, 1, 1)
	assertCommandState(t, other, 0, 0)
}

// These faults occur at the actual database/sql transaction boundary, after
// the original handler has written its business row.
type commandFaultConnector struct {
	driver    driver.Driver
	path      string
	committed bool
	rollback  error
	closed    atomic.Int32
}

func (c *commandFaultConnector) Driver() driver.Driver { return c.driver }

func (c *commandFaultConnector) Connect(context.Context) (driver.Conn, error) {
	connection, err := c.driver.Open(c.path)
	if err != nil {
		return nil, err
	}
	return &commandFaultConnection{Conn: connection, owner: c}, nil
}

type commandFaultConnection struct {
	driver.Conn
	owner *commandFaultConnector
}

func (c *commandFaultConnection) Close() error {
	c.owner.closed.Add(1)
	return c.Conn.Close()
}

func (c *commandFaultConnection) Begin() (driver.Tx, error) {
	tx, err := c.Conn.Begin()
	if err != nil {
		return nil, err
	}
	return commandFaultTransaction{Tx: tx, committed: c.owner.committed, rollback: c.owner.rollback}, nil
}

type commandFaultTransaction struct {
	driver.Tx
	committed bool
	rollback  error
}

var errCommitAcknowledgement = errors.New("injected commit acknowledgement loss")

func (tx commandFaultTransaction) Commit() error {
	if tx.committed {
		if err := tx.Tx.Commit(); err != nil {
			return err
		}
	}
	return errCommitAcknowledgement
}

func (tx commandFaultTransaction) Rollback() error {
	if tx.rollback != nil {
		return tx.rollback
	}
	return tx.Tx.Rollback()
}

func TestCommandRollbackFailureDiscardsOriginalConnection(t *testing.T) {
	original, path := commandDatabase(t)
	rollbackFailure := errors.New("injected rollback failure")
	connector := &commandFaultConnector{driver: original.Driver(), path: path, rollback: rollbackFailure}
	if err := original.Close(); err != nil {
		t.Fatal(err)
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	operationFailure := errors.New("original mutation failed")
	value, err := Command(t.Context(), db, "save", "original", receiptInput(t, "input"), nil, func(ctx context.Context, _ string) (bool, error) {
		if _, err := Connection(ctx, db).ExecContext(ctx, `UPDATE counter SET value=1`); err != nil {
			return false, err
		}
		return false, operationFailure
	})
	if value || !errors.Is(err, operationFailure) || !errors.Is(err, rollbackFailure) {
		t.Fatalf("failed rollback lost its original causes: %t, %v", value, err)
	}
	if _, known := errors.AsType[*framemcp.Refusal](err); known {
		t.Fatalf("failed rollback published a known business refusal: %v", err)
	}
	if connector.closed.Load() != 1 {
		t.Fatal("failed rollback returned the original write-set to the pool")
	}
	assertCommandState(t, db, 0, 0)
}

func TestCommandCommitFailureNeverPublishesPreparedOutcome(t *testing.T) {
	for _, test := range []struct {
		name      string
		committed bool
		refused   bool
	}{
		{name: "success committed but acknowledgement lost", committed: true},
		{name: "refusal committed but acknowledgement lost", committed: true, refused: true},
		{name: "original transaction never committed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			original, path := commandDatabase(t)
			connector := &commandFaultConnector{driver: original.Driver(), path: path, committed: test.committed}
			if err := original.Close(); err != nil {
				t.Fatal(err)
			}
			db := sql.OpenDB(connector)
			t.Cleanup(func() { _ = db.Close() })
			refusal := errors.New("business refusal")
			classify := func(err error) (string, bool) { return "conflict", errors.Is(err, refusal) }
			value, err := Command(t.Context(), db, "save", "original", receiptInput(t, "input"), classify, func(ctx context.Context, _ string) (bool, error) {
				if _, err := Connection(ctx, db).ExecContext(ctx, `UPDATE counter SET value=1`); err != nil {
					return false, err
				}
				if test.refused {
					return false, refusal
				}
				return true, nil
			})
			if value || !errors.Is(err, ErrCommandUnconfirmed) || !errors.Is(err, errCommitAcknowledgement) {
				t.Fatalf("uncertain commit published a prepared result: %t, %v", value, err)
			}
			if _, known := errors.AsType[*framemcp.Refusal](err); known {
				t.Fatalf("unconfirmed refusal was published as known: %v", err)
			}
			if connector.closed.Load() != 1 {
				t.Fatal("uncertain original connection returned to the pool")
			}
			reopened, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			var counter, receipts int
			if test.committed {
				receipts = 1
				if !test.refused {
					counter = 1
				}
			}
			assertCommandState(t, reopened, counter, receipts)
			if test.committed {
				value, err := Command(t.Context(), reopened, "save", "original", receiptInput(t, "input"), classify, func(context.Context, string) (bool, error) {
					t.Fatal("reconciliation re-executed the original effect")
					return false, nil
				})
				if test.refused {
					if _, known := errors.AsType[*framemcp.Refusal](err); !known {
						t.Fatalf("original committed refusal disappeared: %v", err)
					}
				} else if err != nil || !value {
					t.Fatalf("original committed reply disappeared: %t, %v", value, err)
				}
			}
		})
	}
}
