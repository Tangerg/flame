package persistence

import (
	"errors"
	"testing"
	"time"
)

func TestChildRunStartReservationRetainsIdempotentConclusion(t *testing.T) {
	db, err := openDatabase(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := NewChildRunStartReservationStore(db)
	record := ChildRunStartReservationRecord{
		MemberID: "member-child", SessionID: "session-1",
		Payload: []byte(`{"run":"child"}`), CreatedAt: time.Unix(1, 2).UTC(),
	}
	if reserveErr := store.Reserve(t.Context(), record); reserveErr != nil {
		t.Fatalf("Reserve: %v", reserveErr)
	}
	if reserveErr := store.Reserve(t.Context(), record); reserveErr != nil {
		t.Fatalf("Reserve exact replay: %v", reserveErr)
	}
	if confirmed, err := store.Concluded(t.Context(), record, ChildRunStartConclusionStarted); err != nil || confirmed {
		t.Fatalf("reserved conclusion = %t/%v", confirmed, err)
	}
	changed, err := store.Conclude(
		t.Context(), record, ChildRunStartConclusionStarted,
	)
	if err != nil || !changed {
		t.Fatalf("first started conclusion = (%v, %v), want changed", changed, err)
	}
	if confirmed, err := store.Concluded(t.Context(), record, ChildRunStartConclusionStarted); err != nil || !confirmed {
		t.Fatalf("started conclusion = %t/%v", confirmed, err)
	}
	if confirmed, err := store.Concluded(t.Context(), record, ChildRunStartConclusionAborted); err != nil || confirmed {
		t.Fatalf("contradictory conclusion = %t/%v", confirmed, err)
	}
	changed, err = store.Conclude(
		t.Context(), record, ChildRunStartConclusionStarted,
	)
	if err != nil || changed {
		t.Fatalf("started replay = (%v, %v), want unchanged success", changed, err)
	}
	if _, err := store.Conclude(
		t.Context(), record, ChildRunStartConclusionAborted,
	); !errors.Is(err, ErrChildRunStartReservationConflict) {
		t.Fatalf("contradictory conclusion = %v, want conflict", err)
	}
	var state string
	if err := db.QueryRowContext(
		t.Context(),
		`SELECT state FROM child_run_start_reservations WHERE member_id = ?`,
		record.MemberID,
	).Scan(&state); err != nil || state != "started" {
		t.Fatalf("stored conclusion = %q, %v; want started", state, err)
	}
}

func TestChildRunStartReservationRejectsMissingAndChangedIdentity(t *testing.T) {
	db, err := openDatabase(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := NewChildRunStartReservationStore(db)
	record := ChildRunStartReservationRecord{
		MemberID: "member-child", SessionID: "session-1",
		Payload: []byte(`{"run":"child"}`), CreatedAt: time.Unix(1, 2).UTC(),
	}
	if _, err := store.Conclude(
		t.Context(), record, ChildRunStartConclusionAborted,
	); !errors.Is(err, ErrChildRunStartReservationConflict) {
		t.Fatalf("missing conclusion = %v, want conflict", err)
	}
	if err := store.Reserve(t.Context(), record); err != nil {
		t.Fatalf("Reserve: %v", err)
	}
	changed := record
	changed.Payload = []byte(`{"run":"other"}`)
	if err := store.Reserve(t.Context(), changed); !errors.Is(err, ErrChildRunStartReservationConflict) {
		t.Fatalf("changed replay = %v, want conflict", err)
	}
	if _, err := store.Conclude(
		t.Context(), record, ChildRunStartConclusion("unknown"),
	); err == nil {
		t.Fatal("unknown conclusion succeeded")
	}
	if _, err := db.ExecContext(t.Context(),
		`UPDATE child_run_start_reservations SET state = ? WHERE member_id = ?`,
		"corrupt", record.MemberID,
	); err == nil {
		t.Fatal("SQLite accepted a corrupt child Run start state")
	}
}

func TestChildRunStartReservationCleanupIsOwnerScopedAndBootWide(t *testing.T) {
	db, err := openDatabase(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := NewChildRunStartReservationStore(db)
	for _, record := range []ChildRunStartReservationRecord{
		{MemberID: "member-a", SessionID: "session-a", Payload: []byte(`{"run":"a"}`), CreatedAt: time.Unix(1, 0)},
		{MemberID: "member-b", SessionID: "session-b", Payload: []byte(`{"run":"b"}`), CreatedAt: time.Unix(2, 0)},
	} {
		if err := store.Reserve(t.Context(), record); err != nil {
			t.Fatalf("Reserve %q: %v", record.MemberID, err)
		}
	}
	if err := store.DeleteSession(t.Context(), "session-a"); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}
	var sessionA, sessionB int
	if err := db.QueryRowContext(t.Context(),
		`SELECT count(*) FROM child_run_start_reservations WHERE session_id = ?`, "session-a",
	).Scan(&sessionA); err != nil {
		t.Fatalf("count session-a: %v", err)
	}
	if err := db.QueryRowContext(t.Context(),
		`SELECT count(*) FROM child_run_start_reservations WHERE session_id = ?`, "session-b",
	).Scan(&sessionB); err != nil {
		t.Fatalf("count session-b: %v", err)
	}
	if sessionA != 0 || sessionB != 1 {
		t.Fatalf("owner cleanup = session-a:%d session-b:%d, want 0/1", sessionA, sessionB)
	}
}
