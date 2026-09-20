package sqlite_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/run/interrupt"
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

func storedExecutorCheckpoint(rootMemberID, sessionID, payload string) run.CheckpointState {
	selection, err := modelref.NewWithReasoningEffort("anthropic", "claude", "high")
	if err != nil {
		panic(err)
	}
	cost, err := accounting.NewCost(0.25)
	if err != nil {
		panic(err)
	}
	return run.CheckpointState{
		RootMemberID: rootMemberID,
		Payload:      []byte(payload),
		BuildID:      testsupport.BuildID,
		Scope: run.ExecutionScope{
			SessionID:         sessionID,
			CWD:               "/workspace/" + sessionID,
			Isolated:          true,
			GoalIncarnationID: "lease-" + sessionID,
		},
		ModelSelection: selection,
		Capabilities: run.Capabilities{
			ChildRuns:      true,
			InterruptKinds: []interrupt.Kind{interrupt.Approval, interrupt.Question},
		},
		Usage: accounting.Snapshot{Models: []accounting.ModelUsage{{
			Model: "claude",
			TokenUsage: accounting.TokenUsage{
				PromptTokens: 12, CompletionTokens: 7, ReasoningTokens: 3,
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
	if err := store.SaveCheckpoint(ctx, testsupport.MustCheckpoint(first)); err != nil {
		t.Fatalf("SaveCheckpoint(first): %v", err)
	}
	replacement := storedExecutorCheckpoint("member_root", first.Scope.SessionID, `{"tree":"replacement","children":["opaque"]}`)
	replacement.Usage.Models[0].Calls = 2
	if err := store.SaveCheckpoint(ctx, testsupport.MustCheckpoint(replacement)); err != nil {
		t.Fatalf("SaveCheckpoint(replacement): %v", err)
	}

	got, err := store.LoadCheckpoint(ctx, replacement.RootMemberID)
	if err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if !reflect.DeepEqual(got.State(), replacement) {
		t.Fatalf("checkpoint = %+v, want %+v", got, replacement)
	}
	var rows int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM executor_checkpoints`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("checkpoint rows = %d, %v; want one aggregate", rows, err)
	}
}

func TestExecutorCheckpointStorageRejectsMissingModelIdentity(t *testing.T) {
	state := storedExecutorCheckpoint("member_root", "session-1", "opaque")
	state.ModelSelection = modelref.Selection{}
	if _, err := run.NewCheckpoint(state); !errors.Is(err, run.ErrInvalidCheckpoint) {
		t.Fatalf("missing model identity: %v", err)
	}
}

func TestExecutorCheckpointStoreRejectsImmutablePolicyReplacement(t *testing.T) {
	_, store := newExecutorCheckpointStorage(t)
	first := storedExecutorCheckpoint("member_root", "session-1", `{"tree":"first"}`)
	if err := store.SaveCheckpoint(t.Context(), testsupport.MustCheckpoint(first)); err != nil {
		t.Fatalf("SaveCheckpoint(first): %v", err)
	}
	mutations := []struct {
		name   string
		mutate func(*run.CheckpointState)
	}{
		{name: "build", mutate: func(checkpoint *run.CheckpointState) { checkpoint.BuildID = testsupport.AlternateBuildID }},
		{name: "cwd", mutate: func(checkpoint *run.CheckpointState) { checkpoint.Scope.CWD = "/other" }},
		{name: "isolation", mutate: func(checkpoint *run.CheckpointState) { checkpoint.Scope.Isolated = false }},
		{name: "goal incarnation", mutate: func(checkpoint *run.CheckpointState) { checkpoint.Scope.GoalIncarnationID = "other-lease" }},
		{name: "provider", mutate: func(checkpoint *run.CheckpointState) {
			checkpoint.ModelSelection, _ = modelref.New("openai", "claude")
		}},
		{name: "model", mutate: func(checkpoint *run.CheckpointState) {
			checkpoint.ModelSelection, _ = modelref.New("anthropic", "claude-sonnet")
		}},
		{name: "reasoning effort", mutate: func(checkpoint *run.CheckpointState) {
			checkpoint.ModelSelection, _ = modelref.NewWithReasoningEffort("anthropic", "claude", "medium")
		}},
		{name: "capabilities", mutate: func(checkpoint *run.CheckpointState) {
			checkpoint.Capabilities.ChildRuns = false
		}},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			replacement := testsupport.MustCheckpoint(first).State()
			replacement.Payload = []byte(`{"tree":"replacement"}`)
			mutation.mutate(&replacement)
			if err := store.SaveCheckpoint(t.Context(), testsupport.MustCheckpoint(replacement)); !errors.Is(err, run.ErrInvalidCheckpoint) {
				t.Fatalf("SaveCheckpoint error = %v, want ErrInvalidCheckpoint", err)
			}
			stored, err := store.LoadCheckpoint(t.Context(), first.RootMemberID)
			if err != nil {
				t.Fatalf("LoadCheckpoint: %v", err)
			}
			if !reflect.DeepEqual(stored.State(), first) {
				t.Fatalf("checkpoint after rejected replacement = %+v, want %+v", stored, first)
			}
		})
	}
}

func TestExecutorCheckpointStoreRejectsCumulativeUsageRegression(t *testing.T) {
	_, store := newExecutorCheckpointStorage(t)
	first := storedExecutorCheckpoint("member_root", "session-1", `{"tree":"first"}`)
	first.Usage.Models[0].Calls = 2
	if err := store.SaveCheckpoint(t.Context(), testsupport.MustCheckpoint(first)); err != nil {
		t.Fatalf("SaveCheckpoint(first): %v", err)
	}
	replacement := testsupport.MustCheckpoint(first).State()
	replacement.Payload = []byte(`{"tree":"stale"}`)
	replacement.Usage.Models[0].Calls = 1
	if err := store.SaveCheckpoint(t.Context(), testsupport.MustCheckpoint(replacement)); !errors.Is(err, run.ErrInvalidCheckpoint) {
		t.Fatalf("SaveCheckpoint(regression) error = %v, want ErrInvalidCheckpoint", err)
	}
	stored, err := store.LoadCheckpoint(t.Context(), first.RootMemberID)
	if err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if !reflect.DeepEqual(stored.State(), first) {
		t.Fatalf("checkpoint after usage regression = %+v, want %+v", stored, first)
	}
}

func TestExecutorCheckpointStoreRejectsOwnerReassignment(t *testing.T) {
	_, store := newExecutorCheckpointStorage(t)
	first := storedExecutorCheckpoint("member_root", "session-1", `{"tree":"first"}`)
	if err := store.SaveCheckpoint(t.Context(), testsupport.MustCheckpoint(first)); err != nil {
		t.Fatalf("SaveCheckpoint(first): %v", err)
	}
	replacement := storedExecutorCheckpoint(first.RootMemberID, "session-2", `{"tree":"replacement"}`)
	if err := store.SaveCheckpoint(t.Context(), testsupport.MustCheckpoint(replacement)); !errors.Is(err, run.ErrInvalidCheckpoint) {
		t.Fatalf("SaveCheckpoint(reassignment) error = %v, want ErrInvalidCheckpoint", err)
	}
	stored, err := store.LoadCheckpoint(t.Context(), first.RootMemberID)
	if err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if stored.Scope().SessionID != first.Scope.SessionID || !bytes.Equal(stored.Payload(), first.Payload) {
		t.Fatalf("checkpoint after rejected reassignment = %+v, want original owner and payload", stored)
	}
}

func TestExecutorCheckpointStoreTreatsPayloadAsOpaqueBytes(t *testing.T) {
	_, store := newExecutorCheckpointStorage(t)
	checkpoint := storedExecutorCheckpoint("member_root", "session-1", "\x00not-json\xff")
	if err := store.SaveCheckpoint(t.Context(), testsupport.MustCheckpoint(checkpoint)); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}
	got, err := store.LoadCheckpoint(t.Context(), checkpoint.RootMemberID)
	if err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if !bytes.Equal(got.Payload(), checkpoint.Payload) {
		t.Fatalf("payload = %q, want exact opaque bytes %q", got.Payload(), checkpoint.Payload)
	}
}

func TestExecutorCheckpointStoreRoundTripsApplicationEnvelope(t *testing.T) {
	_, store := newExecutorCheckpointStorage(t)
	want := storedExecutorCheckpoint("member_root", "session-1", `{"opaque":true}`)
	if err := store.SaveCheckpoint(t.Context(), testsupport.MustCheckpoint(want)); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}
	got, err := store.LoadCheckpoint(t.Context(), want.RootMemberID)
	if err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}
	if got.RootMemberID() != want.RootMemberID ||
		got.BuildID() != want.BuildID ||
		got.Scope() != want.Scope ||
		got.ModelSelection() != want.ModelSelection ||
		!reflect.DeepEqual(got.Capabilities(), want.Capabilities) ||
		!reflect.DeepEqual(got.Usage(), want.Usage) {
		t.Fatalf("application envelope = %+v, want %+v", got, want)
	}
}

func TestExecutorCheckpointStoreRejectsRetiredOrMalformedPolicy(t *testing.T) {
	tests := map[string]func(string) string{
		"unknown policy field": func(policy string) string {
			return `{"unexpected":true,` + policy[1:]
		},
		"retired lease field": func(policy string) string {
			return strings.Replace(policy, `"goal_incarnation_id"`, `"goal_lease_id"`, 1)
		},
		"missing capability set": func(policy string) string {
			return strings.Replace(policy, `"capabilities":{"child_runs":true,"interrupt_kinds":["approval","question"]}`, `"capabilities":null`, 1)
		},
		"noncanonical kinds": func(policy string) string {
			return strings.Replace(policy, `["approval","question"]`, `["question","approval"]`, 1)
		},
		"unknown kind": func(policy string) string {
			return strings.Replace(policy, `["approval","question"]`, `["approval","future"]`, 1)
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			db, store := newExecutorCheckpointStorage(t)
			checkpoint := storedExecutorCheckpoint("member_root", "session-1", `{"opaque":true}`)
			if err := store.SaveCheckpoint(t.Context(), testsupport.MustCheckpoint(checkpoint)); err != nil {
				t.Fatalf("SaveCheckpoint: %v", err)
			}
			var policy string
			if err := db.QueryRowContext(t.Context(),
				`SELECT policy FROM executor_checkpoints WHERE root_member_id = ?`, checkpoint.RootMemberID,
			).Scan(&policy); err != nil {
				t.Fatalf("read policy: %v", err)
			}
			corrupted := mutate(policy)
			if corrupted == policy {
				t.Fatalf("corruption %q did not change policy %s", name, policy)
			}
			if _, err := db.ExecContext(t.Context(),
				`UPDATE executor_checkpoints SET policy = ? WHERE root_member_id = ?`,
				corrupted,
				checkpoint.RootMemberID,
			); err != nil {
				t.Fatalf("corrupt policy: %v", err)
			}
			if _, err := store.LoadCheckpoint(t.Context(), checkpoint.RootMemberID); !errors.Is(err, run.ErrInvalidCheckpoint) {
				t.Fatalf("LoadCheckpoint error = %v, want ErrInvalidCheckpoint", err)
			}
		})
	}
}

func TestExecutorCheckpointStoreMissingUsesDomainSentinel(t *testing.T) {
	_, store := newExecutorCheckpointStorage(t)
	_, err := store.LoadCheckpoint(t.Context(), "missing")
	if !errors.Is(err, run.ErrCheckpointNotFound) {
		t.Fatalf("LoadCheckpoint error = %v, want ErrExecutorCheckpointNotFound", err)
	}
}

func TestExecutorCheckpointStoreRejectsInvalidEnvelopeBeforeMutation(t *testing.T) {
	db, store := newExecutorCheckpointStorage(t)
	checkpoint := run.Checkpoint{}
	if err := store.SaveCheckpoint(t.Context(), checkpoint); !errors.Is(err, run.ErrInvalidCheckpoint) {
		t.Fatalf("SaveCheckpoint error = %v, want ErrInvalidCheckpoint", err)
	}
	var rows int
	if err := db.QueryRowContext(t.Context(), `SELECT count(*) FROM executor_checkpoints`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("checkpoint rows after rejection = %d, %v", rows, err)
	}
}

func TestExecutorCheckpointStoreDeletesExactAggregates(t *testing.T) {
	_, store := newExecutorCheckpointStorage(t)
	ctx := t.Context()
	for _, checkpoint := range []run.CheckpointState{
		storedExecutorCheckpoint("root-a", "session-a", `{"root":"a"}`),
		storedExecutorCheckpoint("root-b", "session-a", `{"root":"b"}`),
		storedExecutorCheckpoint("root-c", "session-b", `{"root":"c"}`),
	} {
		if err := store.SaveCheckpoint(ctx, testsupport.MustCheckpoint(checkpoint)); err != nil {
			t.Fatalf("SaveCheckpoint(%s): %v", checkpoint.RootMemberID, err)
		}
	}
	if err := store.DeleteCheckpoints(ctx, "session-a", []string{"root-b", "unknown"}); err != nil {
		t.Fatalf("DeleteCheckpoints: %v", err)
	}
	if _, err := store.LoadCheckpoint(ctx, "root-b"); !errors.Is(err, run.ErrCheckpointNotFound) {
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
	if err := store.SaveCheckpoint(t.Context(), testsupport.MustCheckpoint(checkpoint)); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	err := store.DeleteCheckpoints(t.Context(), "session-b", []string{checkpoint.RootMemberID})
	if !errors.Is(err, run.ErrInvalidCheckpoint) {
		t.Fatalf("DeleteCheckpoints error = %v, want ErrInvalidCheckpoint", err)
	}
	if _, err := store.LoadCheckpoint(t.Context(), checkpoint.RootMemberID); err != nil {
		t.Fatalf("foreign checkpoint was deleted: %v", err)
	}
}

func TestExecutorCheckpointStoreDeletesByApplicationOwnership(t *testing.T) {
	_, store := newExecutorCheckpointStorage(t)
	ctx := context.Background()
	for _, checkpoint := range []run.CheckpointState{
		storedExecutorCheckpoint("keep", "session-a", `{"root":"keep"}`),
		storedExecutorCheckpoint("drop-session", "session-b", `{"root":"drop-session"}`),
	} {
		if err := store.SaveCheckpoint(ctx, testsupport.MustCheckpoint(checkpoint)); err != nil {
			t.Fatalf("SaveCheckpoint(%s): %v", checkpoint.RootMemberID, err)
		}
	}
	if err := store.DeleteSessionCheckpoints(ctx, "session-b"); err != nil {
		t.Fatalf("DeleteSessionCheckpoints: %v", err)
	}
	if _, err := store.LoadCheckpoint(ctx, "drop-session"); !errors.Is(err, run.ErrCheckpointNotFound) {
		t.Fatalf("stale checkpoint = %v", err)
	}
	if got, err := store.LoadCheckpoint(ctx, "keep"); err != nil || got.RootMemberID() != "keep" {
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
	want := []string{"root_member_id", "session_id", "build_id", "payload", "policy", "usage"}
	if !slices.Equal(columns, want) {
		t.Fatalf("executor checkpoint columns = %v, want %v", columns, want)
	}
}
