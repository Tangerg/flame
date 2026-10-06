package sqlite_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
)

func newExecutorCheckpointStorage(t *testing.T) (*sql.DB, *persistence.ExecutorCheckpointStore) {
	t.Helper()
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db))
}

func storedExecutorCheckpoint(rootMemberID, sessionID, payload string) runs.ExecutorCheckpoint {
	cost, err := accounting.NewCost(0.25)
	if err != nil {
		panic(err)
	}
	return runs.ExecutorCheckpoint{
		RootMemberID: rootMemberID,
		SessionID:    sessionID,
		Payload:      []byte(payload),
		BuildID:      testsupport.BuildID,
		Usage: accounting.Snapshot{Models: []accounting.ModelUsage{{
			Model: "claude",
			Tokens: accounting.Tokens{
				InputTokens: 12, OutputTokens: 7, ReasoningTokens: 3,
				CacheReadTokens: 4, CacheWriteTokens: 2,
			},
			Cost:  cost,
			Calls: 1,
		}}},
	}
}

func TestExecutorCheckpointStoreReplacesOneRootOwnedAggregate(t *testing.T) {
	db, store := newExecutorCheckpointStorage(t)
	ctx := t.Context()
	first := storedExecutorCheckpoint("member_root", "session-1", `{"tree":"first"}`)
	if err := store.SaveCheckpoint(ctx, first); err != nil {
		t.Fatalf("SaveCheckpoint(first): %v", err)
	}
	replacement := storedExecutorCheckpoint("member_root", first.SessionID, `{"tree":"replacement","children":["opaque"]}`)
	replacement.Usage.Models[0].Calls = 2
	if err := store.SaveCheckpoint(ctx, replacement); err != nil {
		t.Fatalf("SaveCheckpoint(replacement): %v", err)
	}

	got, err := store.LoadCheckpoint(ctx, replacement.RootMemberID)
	if err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if !reflect.DeepEqual(got, replacement) {
		t.Fatalf("checkpoint = %+v, want %+v", got, replacement)
	}
	var rows int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM executor_checkpoints`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("checkpoint rows = %d, %v; want one aggregate", rows, err)
	}
}

func TestExecutorCheckpointStoreRejectsBuildReplacement(t *testing.T) {
	_, store := newExecutorCheckpointStorage(t)
	first := storedExecutorCheckpoint("member_root", "session-1", `{"tree":"first"}`)
	if err := store.SaveCheckpoint(t.Context(), first); err != nil {
		t.Fatalf("SaveCheckpoint(first): %v", err)
	}
	mutations := []struct {
		name   string
		mutate func(*runs.ExecutorCheckpoint)
	}{
		{name: "build", mutate: func(checkpoint *runs.ExecutorCheckpoint) { checkpoint.BuildID = testsupport.AlternateBuildID }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			replacement := first.Clone()
			replacement.Payload = []byte(`{"tree":"replacement"}`)
			mutation.mutate(&replacement)
			if err := store.SaveCheckpoint(t.Context(), replacement); !errors.Is(err, runs.ErrInvalidExecutorCheckpoint) {
				t.Fatalf("SaveCheckpoint error = %v, want ErrInvalidExecutorCheckpoint", err)
			}
			stored, err := store.LoadCheckpoint(t.Context(), first.RootMemberID)
			if err != nil {
				t.Fatalf("LoadCheckpoint: %v", err)
			}
			if !reflect.DeepEqual(stored, first) {
				t.Fatalf("checkpoint after rejected replacement = %+v, want %+v", stored, first)
			}
		})
	}
}

func TestExecutorCheckpointStoreRejectsCumulativeUsageRegression(t *testing.T) {
	_, store := newExecutorCheckpointStorage(t)
	first := storedExecutorCheckpoint("member_root", "session-1", `{"tree":"first"}`)
	first.Usage.Models[0].Calls = 2
	if err := store.SaveCheckpoint(t.Context(), first); err != nil {
		t.Fatalf("SaveCheckpoint(first): %v", err)
	}
	replacement := first.Clone()
	replacement.Payload = []byte(`{"tree":"stale"}`)
	replacement.Usage.Models[0].Calls = 1
	if err := store.SaveCheckpoint(t.Context(), replacement); !errors.Is(err, runs.ErrInvalidExecutorCheckpoint) {
		t.Fatalf("SaveCheckpoint(regression) error = %v, want ErrInvalidExecutorCheckpoint", err)
	}
	stored, err := store.LoadCheckpoint(t.Context(), first.RootMemberID)
	if err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if !reflect.DeepEqual(stored, first) {
		t.Fatalf("checkpoint after usage regression = %+v, want %+v", stored, first)
	}
}

func TestExecutorCheckpointStoreRejectsOwnerReassignment(t *testing.T) {
	_, store := newExecutorCheckpointStorage(t)
	first := storedExecutorCheckpoint("member_root", "session-1", `{"tree":"first"}`)
	if err := store.SaveCheckpoint(t.Context(), first); err != nil {
		t.Fatalf("SaveCheckpoint(first): %v", err)
	}
	replacement := storedExecutorCheckpoint(first.RootMemberID, "session-2", `{"tree":"replacement"}`)
	if err := store.SaveCheckpoint(t.Context(), replacement); !errors.Is(err, runs.ErrInvalidExecutorCheckpoint) {
		t.Fatalf("SaveCheckpoint(reassignment) error = %v, want ErrInvalidExecutorCheckpoint", err)
	}
	stored, err := store.LoadCheckpoint(t.Context(), first.RootMemberID)
	if err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if stored.SessionID != first.SessionID || !bytes.Equal(stored.Payload, first.Payload) {
		t.Fatalf("checkpoint after rejected reassignment = %+v, want original owner and payload", stored)
	}
}

func TestExecutorCheckpointStoreTreatsPayloadAsOpaqueBytes(t *testing.T) {
	_, store := newExecutorCheckpointStorage(t)
	checkpoint := storedExecutorCheckpoint("member_root", "session-1", "\x00not-json\xff")
	if err := store.SaveCheckpoint(t.Context(), checkpoint); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}
	got, err := store.LoadCheckpoint(t.Context(), checkpoint.RootMemberID)
	if err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if !bytes.Equal(got.Payload, checkpoint.Payload) {
		t.Fatalf("payload = %q, want exact opaque bytes %q", got.Payload, checkpoint.Payload)
	}
}

func TestExecutorCheckpointStoreRoundTripsApplicationEnvelope(t *testing.T) {
	_, store := newExecutorCheckpointStorage(t)
	want := storedExecutorCheckpoint("member_root", "session-1", `{"opaque":true}`)
	if err := store.SaveCheckpoint(t.Context(), want); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}
	got, err := store.LoadCheckpoint(t.Context(), want.RootMemberID)
	if err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if got.RootMemberID != want.RootMemberID ||
		got.BuildID != want.BuildID ||
		got.SessionID != want.SessionID ||
		!reflect.DeepEqual(got.Usage, want.Usage) {
		t.Fatalf("application envelope = %+v, want %+v", got, want)
	}
}

func TestExecutorCheckpointStoreMissingUsesDomainSentinel(t *testing.T) {
	_, store := newExecutorCheckpointStorage(t)
	_, err := store.LoadCheckpoint(t.Context(), "missing")
	if !errors.Is(err, runs.ErrExecutorCheckpointNotFound) {
		t.Fatalf("LoadCheckpoint error = %v, want ErrExecutorCheckpointNotFound", err)
	}
}

func TestExecutorCheckpointStoreRejectsInvalidEnvelopeBeforeMutation(t *testing.T) {
	db, store := newExecutorCheckpointStorage(t)
	checkpoint := storedExecutorCheckpoint("member_root", "session-1", `{"opaque":true}`)
	checkpoint.BuildID = ""
	if err := store.SaveCheckpoint(t.Context(), checkpoint); !errors.Is(err, runs.ErrInvalidExecutorCheckpoint) {
		t.Fatalf("SaveCheckpoint error = %v, want ErrInvalidExecutorCheckpoint", err)
	}
	var rows int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM executor_checkpoints`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("checkpoint rows after rejection = %d, %v", rows, err)
	}
}

func TestExecutorCheckpointStoreDeletesExactAggregates(t *testing.T) {
	_, store := newExecutorCheckpointStorage(t)
	ctx := t.Context()
	for _, checkpoint := range []runs.ExecutorCheckpoint{
		storedExecutorCheckpoint("root-a", "session-a", `{"root":"a"}`),
		storedExecutorCheckpoint("root-b", "session-a", `{"root":"b"}`),
		storedExecutorCheckpoint("root-c", "session-b", `{"root":"c"}`),
	} {
		if err := store.SaveCheckpoint(ctx, checkpoint); err != nil {
			t.Fatalf("SaveCheckpoint(%s): %v", checkpoint.RootMemberID, err)
		}
	}
	if err := store.DeleteCheckpoints(ctx, "session-a", []string{"root-b", "unknown"}); err != nil {
		t.Fatalf("DeleteCheckpoints: %v", err)
	}
	if _, err := store.LoadCheckpoint(ctx, "root-b"); !errors.Is(err, runs.ErrExecutorCheckpointNotFound) {
		t.Fatalf("deleted root-b = %v", err)
	}
	for _, rootID := range []string{"root-a", "root-c"} {
		if _, err := store.LoadCheckpoint(ctx, rootID); err != nil {
			t.Fatalf("unrelated checkpoint %q: %v", rootID, err)
		}
	}
	if err := store.DeleteCheckpoints(ctx, "session-a", nil); err == nil {
		t.Fatal("DeleteCheckpoints accepted an empty owner set")
	}
	if err := store.DeleteCheckpoints(ctx, "session-a", []string{"root-a", "root-a"}); err == nil {
		t.Fatal("DeleteCheckpoints accepted duplicate roots")
	}
}

func TestExecutorCheckpointStoreRejectsForeignSessionDeletion(t *testing.T) {
	_, store := newExecutorCheckpointStorage(t)
	checkpoint := storedExecutorCheckpoint("root-a", "session-a", `{"root":"a"}`)
	if err := store.SaveCheckpoint(t.Context(), checkpoint); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	err := store.DeleteCheckpoints(t.Context(), "session-b", []string{checkpoint.RootMemberID})
	if !errors.Is(err, runs.ErrInvalidExecutorCheckpoint) {
		t.Fatalf("DeleteCheckpoints error = %v, want ErrInvalidExecutorCheckpoint", err)
	}
	if _, err := store.LoadCheckpoint(t.Context(), checkpoint.RootMemberID); err != nil {
		t.Fatalf("foreign checkpoint was deleted: %v", err)
	}
}

func TestExecutorCheckpointStoreDeletesByApplicationOwnership(t *testing.T) {
	_, store := newExecutorCheckpointStorage(t)
	ctx := context.Background()
	for _, checkpoint := range []runs.ExecutorCheckpoint{
		storedExecutorCheckpoint("keep", "session-a", `{"root":"keep"}`),
		storedExecutorCheckpoint("drop-session", "session-b", `{"root":"drop-session"}`),
	} {
		if err := store.SaveCheckpoint(ctx, checkpoint); err != nil {
			t.Fatalf("SaveCheckpoint(%s): %v", checkpoint.RootMemberID, err)
		}
	}
	if err := store.DeleteSessionCheckpoints(ctx, "session-b"); err != nil {
		t.Fatalf("DeleteSessionCheckpoints: %v", err)
	}
	if _, err := store.LoadCheckpoint(ctx, "drop-session"); !errors.Is(err, runs.ErrExecutorCheckpointNotFound) {
		t.Fatalf("stale checkpoint = %v", err)
	}
	if got, err := store.LoadCheckpoint(ctx, "keep"); err != nil || got.RootMemberID != "keep" {
		t.Fatalf("preserved checkpoint = (%+v, %v)", got, err)
	}
}

func TestExecutorCheckpointSchemaContainsOnlyOwnedData(t *testing.T) {
	db, _ := newExecutorCheckpointStorage(t)
	rows, err := db.Query(`PRAGMA table_info(executor_checkpoints)`)
	if err != nil {
		t.Fatalf("table_info: %v", err)
	}
	defer func() { _ = rows.Close() }()
	var columns []string
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, dataType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		columns = append(columns, name)
	}
	want := []string{"root_member_id", "session_id", "build_id", "payload", "usage"}
	if !slices.Equal(columns, want) {
		t.Fatalf("executor checkpoint columns = %v, want %v", columns, want)
	}
}
