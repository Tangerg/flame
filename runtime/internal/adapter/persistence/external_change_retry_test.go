package persistence

import (
	"bytes"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func TestExternalChangeObserverRetriesUnpublishedVersion(t *testing.T) {
	config := Config{DataDirectory: filepath.Join(t.TempDir(), "data")}
	observed, err := Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = observed.Close() })
	other, err := Open(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = other.Close() })
	var baseline int64
	if err := observed.db.QueryRowContext(t.Context(), `PRAGMA data_version`).Scan(&baseline); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previousLogger) })
	calls := 0
	observer := externalChangeObserver{db: observed.db, version: baseline, notify: func() {
		calls++
		if calls == 1 || calls == 2 || calls == 4 {
			panic("notification unavailable")
		}
	}}
	if err := other.Sessions.Insert(t.Context(), testsupport.MustRestoreSession(session.Snapshot{ID: "ses_commit"})); err != nil {
		t.Fatal(err)
	}
	for range 4 {
		if !observer.poll(t.Context()) {
			t.Fatal("observer stopped before cancellation")
		}
	}
	if calls != 3 || strings.Count(logs.String(), "external change poll panicked") != 1 {
		t.Fatalf("notification attempts = %d, want two failed attempts reported once and one successful retry", calls)
	}
	if err := other.Sessions.Insert(t.Context(), testsupport.MustRestoreSession(session.Snapshot{ID: "ses_next_commit"})); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if !observer.poll(t.Context()) {
			t.Fatal("observer stopped before cancellation")
		}
	}
	if calls != 5 || strings.Count(logs.String(), "external change poll panicked") != 2 {
		t.Fatalf("recovered observer did not report a later outage once: attempts=%d", calls)
	}
}
