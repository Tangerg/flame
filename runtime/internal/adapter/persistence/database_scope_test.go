package persistence

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Tangerg/frame"
	"github.com/Tangerg/frame/sqlite"
)

func frameDatabaseFixture(t *testing.T) (*database, *frame.Admission) {
	t.Helper()
	admission := frame.NewAdmission()
	store, err := sqlite.Open(t.Context(), sqlite.Config{
		Path: filepath.Join(t.TempDir(), "owner.db"), Admission: admission,
		Owners: []sqlite.OwnerSchema{{ID: "flame", Statements: []string{`CREATE TABLE entries (id TEXT PRIMARY KEY)`},
			References: func(context.Context, sqlite.Reader, frame.InstallationID) ([]frame.Dependency, error) {
				return nil, nil
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	})
	db, err := newDatabase(store, admission)
	if err != nil {
		t.Fatal(err)
	}
	return db, admission
}

func fixtureEntryCount(t *testing.T, db *database) int {
	t.Helper()
	count, err := scopedValue(t.Context(), db.read, func(ctx context.Context) (int, error) {
		var count int
		err := db.connection(ctx).QueryRowContext(ctx, `SELECT count(*) FROM entries`).Scan(&count)
		return count, err
	})
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func TestFrameWriteScopeJoinsNestedStoresAndRollsBack(t *testing.T) {
	db, _ := frameDatabaseFixture(t)
	failure := errors.New("later business write failed")
	err := db.write(t.Context(), func(ctx context.Context) error {
		if _, err := db.connection(ctx).ExecContext(ctx, `INSERT INTO entries VALUES ('first')`); err != nil {
			return err
		}
		return db.write(ctx, func(ctx context.Context) error {
			if _, err := db.connection(ctx).ExecContext(ctx, `INSERT INTO entries VALUES ('second')`); err != nil {
				return err
			}
			return failure
		})
	})
	if !errors.Is(err, failure) || fixtureEntryCount(t, db) != 0 {
		t.Fatalf("partial write survived: %v", err)
	}
	if err := db.write(t.Context(), func(ctx context.Context) error {
		_, err := db.connection(ctx).ExecContext(ctx, `INSERT INTO entries VALUES ('committed')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if fixtureEntryCount(t, db) != 1 {
		t.Fatal("successful original write did not commit")
	}
}

func TestFrameScopeCannotEscapeItsOwnerOrPromoteAReader(t *testing.T) {
	db, admission := frameDatabaseFixture(t)
	var escaped context.Context
	var executor sqlite.Executor
	if err := db.write(t.Context(), func(ctx context.Context) error {
		escaped, executor = ctx, db.connection(ctx)
		return db.read(ctx, func(reader context.Context) error {
			if err := db.write(reader, func(context.Context) error { t.Error("reader promoted to writer"); return nil }); err == nil {
				t.Error("reader promotion accepted")
			}
			return nil
		})
	}); err != nil {
		t.Fatal(err)
	}
	for _, use := range []func(context.Context, func(context.Context) error) error{db.read, db.write} {
		if err := use(escaped, func(context.Context) error { t.Error("escaped scope ran"); return nil }); !errors.Is(err, sql.ErrTxDone) {
			t.Errorf("escaped scope = %v", err)
		}
	}
	if _, err := executor.ExecContext(t.Context(), `INSERT INTO entries VALUES ('escaped')`); !errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("Frame executor outlived transaction: %v", err)
	}
	if err := admission.Admit(t.Context(), func(ctx context.Context) error {
		return db.write(ctx, func(context.Context) error { t.Error("second write entered the same admission"); return nil })
	}); err == nil {
		t.Fatal("admitted caller without its original executor opened another writer")
	}
}

func TestFrameReadRejectsSQLMutationAndReleasesItsCursor(t *testing.T) {
	db, _ := frameDatabaseFixture(t)
	var escaped *sql.Rows
	if err := db.read(t.Context(), func(ctx context.Context) error {
		if rows, err := db.connection(ctx).QueryContext(ctx, `INSERT INTO entries VALUES ('hidden write') RETURNING id`); err == nil {
			_ = rows.Close()
			t.Error("Frame read authority allowed mutation through QueryContext")
		}
		var err error
		escaped, err = db.connection(ctx).QueryContext(ctx, `SELECT 1 UNION ALL SELECT 2`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if escaped.Next() {
		t.Fatal("cursor remained usable after Frame read returned")
	}
	if fixtureEntryCount(t, db) != 0 {
		t.Fatal("read authority changed business state")
	}
}

func TestFrameWriteCancellationAndPanicReleaseOriginalTransaction(t *testing.T) {
	for _, phase := range []string{"cancel", "panic", "goroutine exit"} {
		t.Run(phase, func(t *testing.T) {
			db, _ := frameDatabaseFixture(t)
			cause := errors.New("business execution stopped")
			ctx, cancel := context.WithCancelCause(t.Context())
			defer cancel(nil)
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				defer func() {
					if recovered := recover(); phase == "panic" && recovered != cause {
						t.Errorf("panic cause changed: %v", recovered)
					} else if phase != "panic" && recovered != nil {
						t.Errorf("unexpected panic: %v", recovered)
					}
				}()
				err := db.write(ctx, func(ctx context.Context) error {
					if _, err := db.connection(ctx).ExecContext(ctx, `INSERT INTO entries VALUES ('unfinished')`); err != nil {
						return err
					}
					switch phase {
					case "cancel":
						cancel(cause)
					case "panic":
						panic(cause)
					case "goroutine exit":
						runtime.Goexit()
					}
					return nil
				})
				if phase == "cancel" && (!errors.Is(err, context.Canceled) || !errors.Is(err, cause)) {
					t.Errorf("cancellation causes were lost: %v", err)
				}
			}()
			<-finished
			if fixtureEntryCount(t, db) != 0 {
				t.Fatal("unfinished mutation survived")
			}
		})
	}
}
