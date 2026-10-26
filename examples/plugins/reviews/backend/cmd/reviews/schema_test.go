package main

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestFormerTypedArgumentsDirectoryIsRefusedWithoutChangingPrivateFacts(t *testing.T) {
	directory := t.TempDir()
	current, err := openStore(t.Context(), directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := current.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(directory, "reviews.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `DROP TABLE invocations;
CREATE TABLE invocations(id TEXT PRIMARY KEY,arguments BLOB NOT NULL,result BLOB NOT NULL);
UPDATE reviews SET title='retained',status='resolved',revision=2;
INSERT INTO invocations VALUES('original','{"id":"example-review","expectedRevision":1,"status":"resolved"}','{"type":"updated"}');`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if store, err := openStore(t.Context(), directory); err == nil {
		_ = store.Close()
		t.Fatal("former private format was opened or converted")
	}
	db, err = sql.Open("sqlite", filepath.Join(directory, "reviews.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var title, arguments string
	var revision int
	if err := db.QueryRowContext(t.Context(), `SELECT title,revision FROM reviews`).Scan(&title, &revision); err != nil || title != "retained" || revision != 2 {
		t.Fatalf("schema refusal changed review: %s, %d, %v", title, revision, err)
	}
	if err := db.QueryRowContext(t.Context(), `SELECT arguments FROM invocations WHERE id='original'`).Scan(&arguments); err != nil || arguments != `{"id":"example-review","expectedRevision":1,"status":"resolved"}` {
		t.Fatalf("schema refusal changed receipt: %s, %v", arguments, err)
	}
}
