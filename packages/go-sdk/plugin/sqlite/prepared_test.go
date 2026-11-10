package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	framemcp "github.com/Tangerg/frame/process/mcp"

	pluginmcp "github.com/Tangerg/flame/packages/go-sdk/plugin/mcp"
	"github.com/Tangerg/frame/sqlite"
)

func TestPreparedCommandRetainsUnknownAdmissionAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private.db")
	db, err := sqlite.OpenSchema(t.Context(), path, []string{CommandsSchema})
	if err != nil {
		t.Fatal(err)
	}
	input := receiptInput(t, "input")
	prepared, reply, err := AdmitCommand(t.Context(), db, "maintain", "original", input, nil)
	if err != nil || prepared == nil || len(reply) != 0 {
		t.Fatalf("admission=%v, %s, %v", prepared, reply, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sqlite.OpenSchema(t.Context(), path, []string{CommandsSchema})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if pending, reply, err := AdmitCommand(t.Context(), db, "maintain", "original", input, nil); err != nil || pending == nil || pending.NewlyAdmitted() || len(reply) != 0 {
		t.Fatalf("pending command lost its reconciliation-only admission: %v, %s, %v", pending, reply, err)
	}
	if _, _, err := AdmitCommand(t.Context(), db, "changed", "original", input, nil); !errors.Is(err, framemcp.ErrInvocationConflict) {
		t.Fatalf("changed method=%v", err)
	}
	if _, _, err := AdmitCommand(t.Context(), db, "maintain", "original", receiptInput(t, "changed"), nil); !errors.Is(err, framemcp.ErrInvocationConflict) {
		t.Fatalf("changed input=%v", err)
	}
	if _, err := Command(t.Context(), db, "maintain", "original", input, nil, func(context.Context, string) (CommandPlan[string], error) {
		t.Fatal("pending command entered atomic handler again")
		return CommandPlan[string]{}, nil
	}); !errors.Is(err, ErrCommandUnconfirmed) {
		t.Fatalf("atomic reentry=%v", err)
	}
}

func TestPreparedCompletionIsAtomicWithItsMutationAndImmutable(t *testing.T) {
	db, err := sqlite.OpenSchema(t.Context(), filepath.Join(t.TempDir(), "private.db"), []string{CommandsSchema, `CREATE TABLE counter(value INTEGER NOT NULL)`, `INSERT INTO counter VALUES(0)`, `CREATE TABLE fault(enabled INTEGER NOT NULL)`, `INSERT INTO fault VALUES(1)`, `CREATE TRIGGER abort_completion BEFORE UPDATE OF reply ON commands WHEN (SELECT enabled FROM fault)=1 BEGIN SELECT RAISE(ABORT,'receipt failure'); END`})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	input := receiptInput(t, "input")
	prepared, _, err := AdmitCommand(t.Context(), db, "maintain", "original", input, nil)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := pluginmcp.EncodeReply("saved", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	reply := outcome.Bytes()
	mutate := func(ctx context.Context) error {
		if _, err := sqlite.Connection(ctx, db).ExecContext(ctx, `UPDATE counter SET value=value+1`); err != nil {
			return err
		}
		return prepared.Complete(ctx, reply)
	}
	if err := sqlite.RunInTx(t.Context(), db, mutate); err == nil {
		t.Fatal("receipt failure committed mutation")
	}
	var count int
	var saved sql.NullString
	if err := scanDatabase(t.Context(), db, `SELECT value FROM counter`, nil, &count); err != nil || count != 0 {
		t.Fatalf("mutation survived failed receipt: %d, %v", count, err)
	}
	if err := scanDatabase(t.Context(), db, `SELECT reply FROM commands WHERE id='original'`, nil, &saved); err != nil || saved.Valid {
		t.Fatalf("failed receipt became known: %v, %v", saved, err)
	}
	if _, err := execDatabase(t.Context(), db, `UPDATE fault SET enabled=0`); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.RunInTx(t.Context(), db, mutate); err != nil {
		t.Fatal(err)
	}
	if err := prepared.Complete(t.Context(), reply); err != nil {
		t.Fatal(err)
	}
	other, err := pluginmcp.EncodeReply("different", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	changed := other.Bytes()
	if err := prepared.Complete(t.Context(), changed); !errors.Is(err, framemcp.ErrInvocationConflict) {
		t.Fatalf("completed outcome changed: %v", err)
	}
	duplicate, original, err := AdmitCommand(t.Context(), db, "maintain", "original", input, nil)
	if err != nil || duplicate != nil || string(original) != string(reply) {
		t.Fatalf("original result=%v, %s, %v", duplicate, original, err)
	}
}

func TestExternalSettlementCommitsEvidenceOnceOutsideCancelledCall(t *testing.T) {
	db, err := sqlite.OpenSchema(t.Context(), filepath.Join(t.TempDir(), "external.db"), []string{CommandsSchema, `CREATE TABLE evidence(id TEXT PRIMARY KEY)`})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	prepared, _, err := AdmitCommand(t.Context(), db, "send", "known", receiptInput(t, "input"), nil)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := pluginmcp.EncodeReply("upstream-accepted", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	call, cancel := context.WithCancel(t.Context())
	cancel()
	evidence := func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("settlement has no deadline")
		}
		_, err := sqlite.Connection(ctx, db).ExecContext(ctx, `INSERT INTO evidence VALUES('remote-id')`)
		return err
	}
	failure := errors.New("local failure")
	err = prepared.SettleExternal(call, outcome.Bytes(), func(ctx context.Context) error {
		if err := evidence(ctx); err != nil {
			return err
		}
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("settlement: %v", err)
	}
	if err := prepared.SettleExternal(call, outcome.Bytes(), evidence); err != nil {
		t.Fatal(err)
	}
	if err := prepared.SettleExternal(call, outcome.Bytes(), evidence); err != nil {
		t.Fatalf("settled evidence was replayed: %v", err)
	}
}

func TestCompleteCommandKeepsBusinessDecisionAndReceiptAtomic(t *testing.T) {
	for _, test := range []struct {
		name      string
		refuse    bool
		failWrite bool
	}{
		{name: "success"},
		{name: "read decision refuses", refuse: true},
		{name: "write error remains unconfirmed", failWrite: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, _ := commandDatabase(t)
			prepared, _, err := AdmitCommand(t.Context(), db, "publish", "original", receiptInput(t, "input"), nil)
			if err != nil {
				t.Fatal(err)
			}
			known := errors.New("publication no longer applies")
			classify := func(err error) (string, bool) { return "stale", errors.Is(err, known) }
			prepare := func(ctx context.Context) (CommandPlan[string], error) {
				if _, err := sqlite.Connection(ctx, db).ExecContext(ctx, `UPDATE counter SET value=10`); err == nil {
					t.Fatal("local publication decision could write")
				}
				if test.refuse {
					return CommandPlan[string]{}, known
				}
				return NewCommandPlan("published", func(ctx context.Context) error {
					if _, err := sqlite.Connection(ctx, db).ExecContext(ctx, `UPDATE counter SET value=1`); err != nil {
						return err
					}
					if test.failWrite {
						return known
					}
					return nil
				})
			}
			result, err := CompleteCommand(t.Context(), prepared, classify, prepare)
			if test.failWrite {
				if result != "" || !errors.Is(err, pluginmcp.ErrOutcomeUnconfirmed) {
					t.Fatalf("write failure acquired finality: %q, %v", result, err)
				}
				assertCommandState(t, db, 0, 1)
				_, _, reply, readErr := ResumeCommand(t.Context(), db, "publish", "original")
				if readErr != nil || len(reply) != 0 {
					t.Fatalf("write failure settled original admission: %s, %v", reply, readErr)
				}
				return
			}
			if test.refuse {
				if refusal, known := errors.AsType[*framemcp.Refusal](err); !known || refusal.Code != "stale" {
					t.Fatalf("decision lost original refusal: %v", err)
				}
				assertCommandState(t, db, 0, 1)
			} else {
				if err != nil || result != "published" {
					t.Fatalf("publication: %q, %v", result, err)
				}
				assertCommandState(t, db, 1, 1)
			}
			_, replayErr := CompleteCommand(t.Context(), prepared, classify, func(context.Context) (CommandPlan[string], error) {
				t.Fatal("receipt replay prepared work again")
				return CommandPlan[string]{}, nil
			})
			if test.refuse {
				if _, known := errors.AsType[*framemcp.Refusal](replayErr); !known {
					t.Fatal(replayErr)
				}
			} else if replayErr != nil {
				t.Fatal(replayErr)
			}
		})
	}
}

func TestCompleteExternalCommandRetainsAcknowledgementAfterCancellation(t *testing.T) {
	db, _ := commandDatabase(t)
	prepared, _, err := AdmitCommand(t.Context(), db, "publish", "original", receiptInput(t, "input"), nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	result, err := CompleteExternalCommand(ctx, prepared, nil, func(ctx context.Context) (CommandPlan[string], error) {
		if ctx.Err() != nil {
			t.Fatal("cancelled call erased external acknowledgement")
		}
		if _, bounded := ctx.Deadline(); !bounded {
			t.Fatal("external settlement lost its lifetime bound")
		}
		return NewCommandPlan("accepted", func(ctx context.Context) error {
			_, err := sqlite.Connection(ctx, db).ExecContext(ctx, `UPDATE counter SET value=1`)
			return err
		})
	})
	if err != nil || result != "accepted" {
		t.Fatalf("acknowledgement settlement: %q, %v", result, err)
	}
	assertCommandState(t, db, 1, 1)
}
