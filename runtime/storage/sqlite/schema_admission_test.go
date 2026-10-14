package sqlite_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/Tangerg/flame/runtime/storage/sqlite"
)

var seededSchema = []string{
	`CREATE TABLE IF NOT EXISTS identity (id INTEGER PRIMARY KEY, value TEXT NOT NULL)`,
	`INSERT OR IGNORE INTO identity VALUES (1, 'original')`,
}

func TestCurrentSchemaAdmissionDoesNotAcquireAnotherOwnersWriteSet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "current.db")
	owner, err := sqlite.OpenSchema(t.Context(), path, seededSchema)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	write, err := owner.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer write.Rollback()
	if _, err := write.ExecContext(t.Context(), `UPDATE identity SET value='uncommitted' WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	reader, err := sqlite.OpenSchema(t.Context(), path, seededSchema)
	if err != nil {
		t.Fatalf("current schema admission required a competing write-set: %v", err)
	}
	defer reader.Close()
	var value string
	if err := reader.QueryRowContext(t.Context(), `SELECT value FROM identity WHERE id=1`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != "original" {
		t.Fatalf("schema admission read an uncommitted owner fact: %q", value)
	}
}

func TestCurrentSchemaAdmissionNeverRecreatesAnOwnersMissingFact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "current.db")
	owner, err := sqlite.OpenSchema(t.Context(), path, seededSchema)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.ExecContext(t.Context(), `DELETE FROM identity`); err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	reader, err := sqlite.OpenSchema(t.Context(), path, seededSchema)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var count int
	if err := reader.QueryRowContext(t.Context(), `SELECT count(*) FROM identity`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("schema admission recreated an owner fact from initialization SQL")
	}
}

func TestConcurrentSchemaInitializationAdmitsOneOwnerFact(t *testing.T) {
	path := filepath.Join(t.TempDir(), "current.db")
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			db, err := sqlite.OpenSchema(t.Context(), path, seededSchema)
			if err != nil {
				results <- err
				return
			}
			var count int
			err = db.QueryRowContext(t.Context(), `SELECT count(*) FROM identity`).Scan(&count)
			if err == nil && count != 1 {
				err = fmt.Errorf("initialization facts = %d, want one", count)
			}
			results <- errors.Join(err, db.Close())
		}()
	}
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Error(err)
		}
	}
}
