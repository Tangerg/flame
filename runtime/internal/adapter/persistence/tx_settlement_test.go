package persistence

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/Tangerg/flame/qimu/sqlite"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/plugin"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestTransactionFailureRetainsRollbackDiagnostic(t *testing.T) {
	failure := errors.New("write-set failed")
	retirement := errors.New("transaction rollback failed")
	faults := &transactionFaults{rollback: func() error { return retirement }}
	db := settlementDatabase(t, faults)
	err := sqlite.RunInTx(t.Context(), db, func(context.Context) error { return failure })
	if !errors.Is(err, failure) || !errors.Is(err, retirement) {
		t.Fatalf("transaction = %v, want write-set and rollback failures", err)
	}
	if faults.rollbacks.Load() != 1 || faults.commits.Load() != 0 || db.Stats().InUse != 0 {
		t.Fatalf("transaction did not settle once: rollback=%d, commit=%d, in-use=%d", faults.rollbacks.Load(), faults.commits.Load(), db.Stats().InUse)
	}
}

func TestFailedTransactionSettlementCannotRecycleItsConnection(t *testing.T) {
	for _, phase := range []string{"rollback", "commit"} {
		t.Run(phase, func(t *testing.T) {
			failure := errors.New("transaction settlement failed")
			faults := &transactionFaults{}
			body := func(context.Context) error { return nil }
			if phase == "rollback" {
				faults.rollback = func() error { return failure }
				body = func(context.Context) error { return errors.New("write-set failed") }
			} else {
				faults.commitFailure = failure
			}
			db := settlementDatabase(t, faults)
			err := sqlite.RunInTx(t.Context(), db, body)
			if !errors.Is(err, failure) || db.Stats().Idle != 0 || db.Stats().InUse != 0 {
				t.Fatalf("settlement = %v, idle=%d, in-use=%d; want retained failure and discarded connection", err, db.Stats().Idle, db.Stats().InUse)
			}
		})
	}
}

func TestTransactionCancellationJoinsOwnedRollback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		failure := errors.New("write-set failed")
		retirement := errors.New("transaction rollback failed")
		cause := errors.New("write request stopped")
		release := make(chan struct{})
		faults := &transactionFaults{rollback: func() error { <-release; return retirement }}
		db := settlementDatabase(t, faults)
		ctx, stop := context.WithCancelCause(t.Context())
		defer stop(nil)
		entered := make(chan struct{})
		finishBody := make(chan struct{})
		done := make(chan error, 1)
		go func() {
			done <- sqlite.RunInTx(ctx, db, func(context.Context) error {
				close(entered)
				<-finishBody
				return failure
			})
		}()
		<-entered
		stop(cause)
		synctest.Wait()
		close(finishBody)
		synctest.Wait()
		var result error
		select {
		case result = <-done:
			t.Error("transaction returned before rollback settled")
		default:
		}
		close(release)
		if result == nil {
			result = <-done
		}
		if !errors.Is(result, failure) || !errors.Is(result, retirement) || !errors.Is(result, context.Canceled) || !errors.Is(result, cause) {
			t.Errorf("transaction = %v, want write-set, rollback and cancellation causes", result)
		}
		if faults.rollbacks.Load() != 1 || faults.commits.Load() != 0 || db.Stats().InUse != 0 {
			t.Errorf("transaction did not settle its resource: rollback=%d, commit=%d, in-use=%d", faults.rollbacks.Load(), faults.commits.Load(), db.Stats().InUse)
		}
	})
}

func TestTransactionAcquisitionRetainsCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		db := settlementDatabase(t, &transactionFaults{})
		held, err := db.Conn(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		defer held.Close()
		ctx, stop := context.WithCancelCause(t.Context())
		defer stop(nil)
		cause := errors.New("connection acquisition stopped")
		done := make(chan error, 1)
		go func() {
			done <- sqlite.RunInTx(ctx, db, func(context.Context) error {
				t.Error("canceled acquisition started a write-set")
				return nil
			})
		}()
		synctest.Wait()
		stop(cause)
		if err := <-done; !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
			t.Errorf("acquisition = %v, want cancellation and cause", err)
		}
	})
}

func TestTransactionRejectsUnstartedPhasesAfterCancellation(t *testing.T) {
	for _, phase := range []string{"after begin", "nested write-set"} {
		t.Run(phase, func(t *testing.T) {
			ctx, stop := context.WithCancelCause(t.Context())
			defer stop(nil)
			cause := errors.New("write request stopped")
			faults := &transactionFaults{}
			if phase == "after begin" {
				faults.afterBegin = func() { stop(cause) }
			}
			db := settlementDatabase(t, faults)
			err := sqlite.RunInTx(ctx, db, func(ctx context.Context) error {
				if phase == "nested write-set" {
					stop(cause)
					return sqlite.RunInTx(ctx, db, func(context.Context) error {
						t.Error("canceled transaction started a nested write-set")
						return nil
					})
				}
				t.Error("canceled transaction started its write-set")
				return nil
			})
			if !errors.Is(err, context.Canceled) || !errors.Is(err, cause) {
				t.Errorf("transaction = %v, want cancellation and cause", err)
			}
		})
	}
}

func TestTransactionBeginFailureRetainsCancellation(t *testing.T) {
	ctx, stop := context.WithCancelCause(t.Context())
	defer stop(nil)
	cause := errors.New("write request stopped")
	failure := errors.New("begin failed")
	db := settlementDatabase(t, &transactionFaults{beforeBegin: func() error { stop(cause); return failure }})
	err := sqlite.RunInTx(ctx, db, func(context.Context) error {
		t.Error("failed begin started a write-set")
		return nil
	})
	if !errors.Is(err, failure) || !errors.Is(err, context.Canceled) || !errors.Is(err, cause) || db.Stats().InUse != 0 {
		t.Fatalf("begin = %v, in-use=%d; want failure and cancellation with released connection", err, db.Stats().InUse)
	}
}

func TestTransactionRetainsAcceptedCommitAfterLateCancellation(t *testing.T) {
	ctx, stop := context.WithCancelCause(t.Context())
	defer stop(nil)
	cause := errors.New("write request stopped during commit")
	faults := &transactionFaults{beforeCommit: func() { stop(cause) }}
	db := settlementDatabase(t, faults)
	err := sqlite.RunInTx(ctx, db, func(ctx context.Context) error {
		_, err := sqlite.Connection(ctx, db).ExecContext(ctx, `CREATE TABLE accepted_commit (id INTEGER PRIMARY KEY)`)
		return err
	})
	if err != nil {
		t.Fatalf("accepted commit = %v, want successful durable result", err)
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO accepted_commit(id) VALUES (1)`); err != nil {
		t.Fatalf("committed write-set is absent: %v", err)
	}
	if faults.commits.Load() != 1 || faults.rollbacks.Load() != 0 || db.Stats().InUse != 0 {
		t.Fatalf("accepted commit was not consumed once: commit=%d, rollback=%d, in-use=%d", faults.commits.Load(), faults.rollbacks.Load(), db.Stats().InUse)
	}
}

func TestTransactionPanicRetainsRollbackFailure(t *testing.T) {
	failure := errors.New("write-set panicked")
	retirement := errors.New("transaction rollback failed")
	faults := &transactionFaults{rollback: func() error { return retirement }}
	db := settlementDatabase(t, faults)
	func() {
		defer func() {
			recovered, ok := recover().(error)
			if !ok || !errors.Is(recovered, failure) || !errors.Is(recovered, retirement) {
				t.Errorf("panic = %v, want write-set and rollback failures", recovered)
			}
		}()
		_ = sqlite.RunInTx(t.Context(), db, func(context.Context) error { panic(failure) })
	}()
	if faults.rollbacks.Load() != 1 || db.Stats().InUse != 0 {
		t.Fatal("panic retained its transaction resource")
	}
}

func TestTransactionPreservesPanicAfterSuccessfulRollback(t *testing.T) {
	for _, value := range []any{errors.New("write-set panicked"), "write-set panicked"} {
		faults := &transactionFaults{}
		db := settlementDatabase(t, faults)
		func() {
			defer func() {
				if recovered := recover(); recovered != value {
					t.Errorf("panic = %v, want original %v", recovered, value)
				}
			}()
			_ = sqlite.RunInTx(t.Context(), db, func(context.Context) error { panic(value) })
		}()
		if faults.rollbacks.Load() != 1 || db.Stats().InUse != 0 {
			t.Fatal("panic retained its transaction resource")
		}
	}
}

func TestTransactionSettlesBeforeGoroutineExit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		faults := &transactionFaults{}
		db := settlementDatabase(t, faults)
		transaction := make(chan *sql.Tx, 1)
		finished := make(chan struct{})
		go func() {
			defer close(finished)
			_ = sqlite.RunInTx(t.Context(), db, func(ctx context.Context) error {
				transaction <- sqlite.Connection(ctx, db).(*sql.Tx)
				runtime.Goexit()
				return nil
			})
		}()
		tx := <-transaction
		synctest.Wait()
		select {
		case <-finished:
		default:
			t.Error("goroutine exit retained an unsettled transaction")
			if err := tx.Rollback(); err != nil {
				t.Fatal(err)
			}
			<-finished
		}
		if faults.rollbacks.Load() != 1 || faults.commits.Load() != 0 || db.Stats().InUse != 0 {
			t.Fatalf("goroutine exit did not settle its transaction: rollback=%d, commit=%d, in-use=%d", faults.rollbacks.Load(), faults.commits.Load(), db.Stats().InUse)
		}
	})
}

func TestMissingInstallationReadRequiresAcceptedSettlement(t *testing.T) {
	failure := errors.New("installation read commit failed")
	db := settlementDatabase(t, &transactionFaults{commitFailure: failure})
	snapshot, found, err := newInstallationStore(newReleaseStore(db)).Get(t.Context(), testsupport.InstallationID(t))
	if found || snapshot.Installation != nil || !errors.Is(err, failure) {
		t.Fatalf("installation = (%v, %t, %v), want rejected read settlement", snapshot.Installation, found, err)
	}
}

func TestInstallationLookupSeparatesAbsentRecordFromMissingRelease(t *testing.T) {
	db := settlementDatabase(t, &transactionFaults{})
	catalog := newReleaseStore(db)
	store := newInstallationStore(catalog)
	id := testsupport.InstallationID(t)
	if _, found, err := store.Get(t.Context(), id); err != nil || found {
		t.Fatalf("absent installation = (%t, %v), want admitted absence", found, err)
	}
	release := testsupport.Release(t, "1", plugin.Declaration{Name: "package"})
	if err := catalog.Admit(t.Context(), release); err != nil {
		t.Fatal(err)
	}
	installation, err := plugin.New(id, "/package", release)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.projectRecord(t.Context(), installation.Snapshot()); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.Get(t.Context(), id); err != nil || !found {
		t.Fatalf("admitted installation = (%t, %v)", found, err)
	}
	if _, err := db.ExecContext(t.Context(), `PRAGMA foreign_keys=OFF`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `DELETE FROM plugin_releases WHERE digest=?`, release.Digest().String()); err != nil {
		t.Fatal(err)
	}
	if snapshot, found, err := store.Get(t.Context(), id); found || snapshot.Installation != nil || !errors.Is(err, plugin.ErrNotFound) || !strings.Contains(err.Error(), "selected release") {
		t.Fatalf("malformed installation = (%v, %t, %v), want rejected restoration with release cause", snapshot.Installation, found, err)
	}
}

func TestSchemaRejectionRetainsRollbackFailure(t *testing.T) {
	retirement := errors.New("schema rollback failed")
	db := settlementDatabase(t, &transactionFaults{rollback: func() error { return retirement }})
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE obsolete_schema (id INTEGER)`); err != nil {
		t.Fatal(err)
	}
	err := sqlite.InstallSchema(t.Context(), db, currentSchema())
	if !errors.Is(err, retirement) || !strings.Contains(err.Error(), "incompatible schema") {
		t.Fatalf("schema rejection = %v, want admission and rollback failures", err)
	}
}

func TestWorkspaceIntentRejectionRetainsRollbackFailure(t *testing.T) {
	retirement := errors.New("workspace intent rollback failed")
	db := settlementDatabase(t, &transactionFaults{rollback: func() error { return retirement }})
	created, err := newWorkspaceMutationStore(db).recordMutation(t.Context(), workspaceMutationRecord{SessionID: "ses_missing", CWD: t.TempDir(), ToRunID: "run_missing"})
	if created || !errors.Is(err, retirement) || !strings.Contains(err.Error(), "no matching Run boundary") {
		t.Fatalf("intent = (%t, %v), want rejection and rollback failure", created, err)
	}
}

func TestAdoptedWorkspaceIntentRequiresTransactionSettlement(t *testing.T) {
	faults := &transactionFaults{}
	db := settlementDatabase(t, faults)
	seedWorkspaceMutationSession(t, db, "ses_intent", "/repo")
	seedWorkspaceMutationRun(t, db, "ses_intent", "run_intent")
	store := newWorkspaceMutationStore(db)
	intent := workspaceMutationRecord{SessionID: "ses_intent", CWD: "/repo", ToRunID: "run_intent"}
	if created, err := store.recordMutation(t.Context(), intent); !created || err != nil {
		t.Fatalf("initial intent = (%t, %v)", created, err)
	}
	failure := errors.New("intent read transaction commit failed")
	faults.commitFailure = failure
	if created, err := store.recordMutation(t.Context(), intent); created || !errors.Is(err, failure) {
		t.Fatalf("adopted intent = (%t, %v), want retained settlement failure", created, err)
	}
}

type transactionFaults struct {
	beforeBegin   func() error
	afterBegin    func()
	beforeCommit  func()
	commitFailure error
	rollback      func() error
	commits       atomic.Int32
	rollbacks     atomic.Int32
}

func settlementDatabase(t *testing.T, faults *transactionFaults) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "flame.db")
	base, err := openDatabase(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := base.Close(); err != nil {
			t.Error(err)
		}
	})
	db := sql.OpenDB(settlementConnector{base: base.Driver(), dsn: "file:" + url.PathEscape(filepath.ToSlash(path)) + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)", faults: faults})
	db.SetMaxOpenConns(1)
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	return db
}

type settlementConnector struct {
	base   driver.Driver
	dsn    string
	faults *transactionFaults
}

func (c settlementConnector) Connect(context.Context) (driver.Conn, error) {
	connection, err := c.base.Open(c.dsn)
	if err != nil {
		return nil, err
	}
	return &settlementConnection{Conn: connection, faults: c.faults}, nil
}

func (c settlementConnector) Driver() driver.Driver { return c.base }

type settlementConnection struct {
	driver.Conn
	faults *transactionFaults
}

func (c *settlementConnection) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *settlementConnection) BeginTx(ctx context.Context, options driver.TxOptions) (driver.Tx, error) {
	if c.faults.beforeBegin != nil {
		if err := c.faults.beforeBegin(); err != nil {
			return nil, err
		}
	}
	transaction, err := c.Conn.(driver.ConnBeginTx).BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}
	if c.faults.afterBegin != nil {
		c.faults.afterBegin()
	}
	return settlementTransaction{Tx: transaction, faults: c.faults}, nil
}

type settlementTransaction struct {
	driver.Tx
	faults *transactionFaults
}

func (t settlementTransaction) Commit() error {
	t.faults.commits.Add(1)
	if t.faults.beforeCommit != nil {
		t.faults.beforeCommit()
	}
	return errors.Join(t.Tx.Commit(), t.faults.commitFailure)
}

func (t settlementTransaction) Rollback() error {
	t.faults.rollbacks.Add(1)
	err := t.Tx.Rollback()
	if t.faults.rollback != nil {
		err = errors.Join(err, t.faults.rollback())
	}
	return err
}
