package sqlite

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	framemcp "github.com/Tangerg/frame/process/mcp"
	framesqlite "github.com/Tangerg/frame/sqlite"
	framevalue "github.com/Tangerg/frame/value"
)

func commandDatabase(t *testing.T) (*framesqlite.Database, string) {
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

func assertCommandState(t *testing.T, db *framesqlite.Database, counter, receipts int) {
	t.Helper()
	var actualCounter, actualReceipts int
	if err := db.Read(t.Context(), func(reader framesqlite.Reader) error {
		if err := reader.QueryRowContext(t.Context(), `SELECT value FROM counter`).Scan(&actualCounter); err != nil {
			return err
		}
		return reader.QueryRowContext(t.Context(), `SELECT count(*) FROM commands`).Scan(&actualReceipts)
	}); err != nil {
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
	value, err := Command(ctx, db, "save", "cancelled", receiptInput(t, "input"), nil, func(context.Context, string) (CommandPlan[bool], error) {
		return NewCommandPlan(true, func(ctx context.Context) error {
			_, err := framesqlite.Connection(ctx, db).ExecContext(ctx, `UPDATE counter SET value=1`)
			cancel(cause)
			return err
		})
	})
	if value || !errors.Is(err, cause) || !errors.Is(err, pluginmcp.ErrOutcomeUnconfirmed) {
		t.Fatalf("cancelled command published success or lost its cause: %t, %v", value, err)
	}
	assertCommandState(t, db, 0, 0)
}

func TestCommandPreparationCannotWriteAndItsBorrowExpires(t *testing.T) {
	db, _ := commandDatabase(t)
	var readContext context.Context
	var reader framesqlite.Executor
	var writer framesqlite.Executor
	_, err := Command(t.Context(), db, "save", "original", receiptInput(t, "input"), nil, func(ctx context.Context, _ string) (CommandPlan[bool], error) {
		readContext, reader = ctx, framesqlite.Connection(ctx, db)
		if _, err := reader.ExecContext(ctx, `UPDATE counter SET value=9`); err == nil {
			t.Fatal("preparation wrote business state")
		}
		if err := framesqlite.RunInTx(ctx, db, func(context.Context) error {
			t.Fatal("preparation promoted its read capability")
			return nil
		}); err == nil {
			t.Fatal("preparation entered a writer")
		}
		return NewCommandPlan(true, func(ctx context.Context) error {
			if _, err := reader.ExecContext(ctx, `UPDATE counter SET value=2`); err == nil {
				t.Fatal("expired preparation borrowed the write phase")
			}
			if err := framesqlite.Connection(readContext, db).QueryRowContext(ctx, `SELECT value FROM counter`).Err(); err == nil {
				t.Fatal("expired preparation context reacquired authority")
			}
			writer = framesqlite.Connection(ctx, db)
			_, err := writer.ExecContext(ctx, `UPDATE counter SET value=1`)
			return err
		})
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writer.ExecContext(t.Context(), `UPDATE counter SET value=3`); err == nil {
		t.Fatal("retained writer changed a committed command")
	}
	assertCommandState(t, db, 1, 1)
}

func TestCommandWriteFailureCannotBeClassifiedAsARefusal(t *testing.T) {
	for _, cause := range []error{framemcp.ErrInvocationConflict, &framemcp.Refusal{Code: "conflict", Message: "late business error"}} {
		t.Run(cause.Error(), func(t *testing.T) {
			db, _ := commandDatabase(t)
			classified := 0
			classify := func(error) (string, bool) { classified++; return "conflict", true }
			value, err := Command(t.Context(), db, "save", "original", receiptInput(t, "input"), classify, func(context.Context, string) (CommandPlan[bool], error) {
				return NewCommandPlan(true, func(ctx context.Context) error {
					if _, err := framesqlite.Connection(ctx, db).ExecContext(ctx, `UPDATE counter SET value=1`); err != nil {
						return err
					}
					return cause
				})
			})
			if value || !errors.Is(err, cause) || !errors.Is(err, pluginmcp.ErrOutcomeUnconfirmed) || classified != 0 {
				t.Fatalf("write failure became a decision: value=%t, classifications=%d, err=%v", value, classified, err)
			}
			if _, wireErr := pluginmcp.EncodeReply(value, err, classify); !errors.Is(wireErr, pluginmcp.ErrOutcomeUnconfirmed) || classified != 0 {
				t.Fatalf("transport reclassified a write failure: classifications=%d, err=%v", classified, wireErr)
			}
			assertCommandState(t, db, 0, 0)
		})
	}
}

func TestCommandPlanFreezesReplyBeforeMutation(t *testing.T) {
	db, _ := commandDatabase(t)
	value, err := Command(t.Context(), db, "save", "original", receiptInput(t, "input"), nil, func(context.Context, string) (CommandPlan[[]string], error) {
		result := []string{"admitted"}
		return NewCommandPlan(result, func(context.Context) error { result[0] = "changed"; return nil })
	})
	if err != nil || len(value) != 1 || value[0] != "admitted" {
		t.Fatalf("mutable result changed the admitted reply: %v, %v", value, err)
	}
	assertCommandState(t, db, 0, 1)
}

func TestCommandPlanEncodingFailureCannotBecomeABusinessRefusal(t *testing.T) {
	db, _ := commandDatabase(t)
	classified := 0
	classify := func(error) (string, bool) { classified++; return "conflict", true }
	_, err := Command(t.Context(), db, "save", "invalid-output", receiptInput(t, "input"), classify, func(context.Context, string) (CommandPlan[framevalue.JSON], error) {
		return NewCommandPlan(framevalue.JSON{}, func(ctx context.Context) error {
			_, err := framesqlite.Connection(ctx, db).ExecContext(ctx, `UPDATE counter SET value=1`)
			return err
		})
	})
	if !errors.Is(err, pluginmcp.ErrOutcomeUnconfirmed) || classified != 0 {
		t.Fatalf("invalid output became a business refusal: classifications=%d, err=%v", classified, err)
	}
	assertCommandState(t, db, 0, 0)
}

func TestZeroCommandPlanDoesNotCreateReceipt(t *testing.T) {
	db, _ := commandDatabase(t)
	_, err := Command(t.Context(), db, "save", "original", receiptInput(t, "input"), nil, func(context.Context, string) (CommandPlan[bool], error) {
		return CommandPlan[bool]{}, nil
	})
	if err == nil {
		t.Fatal("zero plan was admitted")
	}
	assertCommandState(t, db, 0, 0)
}
