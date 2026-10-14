package recovery

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/adapter/persistence"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/run/accounting"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/scope/core/chat"
)

func endpointRecovery(t *testing.T, db *sql.DB) *runs.Recovery {
	t.Helper()
	store, err := New(Config{Sessions: sqlite.NewSessionStore(db), Runs: sqlite.NewRunStore(db), Interrupts: persistence.NewInterruptStore(sqlite.NewInterruptStore(db)), Transcript: sqlite.NewTranscriptStore(db), Messages: mustConversationStore(t, sqlite.NewMessageStore(db)), ExecutorCheckpoints: persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db)), ModelInvocations: sqlite.NewModelInvocationStore(db), ToolInvocations: sqlite.NewToolInvocationStore(db), ChildRunStarts: sqlite.NewChildRunStartReservationStore(db), Tx: func(ctx context.Context, fn func(context.Context) error) error { return sqlite.RunInTx(ctx, db, fn) }})
	if err != nil {
		t.Fatal(err)
	}
	recovery, err := newTestRecovery(store, alwaysResumable{})
	if err != nil {
		t.Fatal(err)
	}
	return recovery
}

func TestRecoveryAccruesOnlyUncommittedKnownEndpointsAcrossRestart(t *testing.T) {
	for _, test := range []struct {
		name            string
		known, reported bool
		input           int64
		price           *float64
		wantCost        *float64
	}{
		{name: "priced", known: true, reported: true, input: 12, price: new(1.25), wantCost: new(1.75)},
		{name: "unpriced", known: true, reported: true, input: 12},
		{name: "reported zero", known: true, reported: true, input: 0, price: new(0.0), wantCost: new(0.5)},
		{name: "unreported", known: true},
		{name: "unknown", wantCost: new(0.5)},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "recovery.db")
			db, err := sqlite.Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			at := time.Now().UTC().Add(-time.Minute)
			if err := sqlite.NewSessionStore(db).Insert(t.Context(), testsupport.MustRestoreSession(session.Snapshot{ID: "ses_source", CreatedAt: at, UpdatedAt: at})); err != nil {
				t.Fatal(err)
			}
			selection := testsupport.MustModelSelection("anthropic", "captured-model")
			draft := run.Draft{RunID: "run_source", SessionID: "ses_source", SegmentID: "seg_source", ModelSelection: selection, CreatedAt: at}
			runsStore := sqlite.NewRunStore(db)
			if err := runsStore.Admit(t.Context(), draft); err != nil {
				t.Fatal(err)
			}
			active, _, err := runsStore.Run(t.Context(), draft.RunID)
			if err != nil {
				t.Fatal(err)
			}
			previous := accounting.Totals{Tokens: accounting.Tokens{InputTokens: 5, OutputTokens: 2}, CostUSD: new(0.5)}
			metrics, err := run.NewMetrics(&accounting.Usage{Total: previous, ByModel: map[string]accounting.Totals{"captured-model": previous}}, 1, time.Second)
			if err != nil {
				t.Fatal(err)
			}
			active, err = active.AdvanceProgress(metrics, 5, at)
			if err != nil {
				t.Fatal(err)
			}
			if err := runsStore.UpdateProgress(t.Context(), active); err != nil {
				t.Fatal(err)
			}
			journal := sqlite.NewModelInvocationStore(db)
			if err := journal.StartModelInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "logical_pending", at); err != nil {
				t.Fatal(err)
			}
			request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("captured request")))
			invocation := modelref.EndpointInvocation{ID: "endpoint_pending", Caller: capability.Invocation{ID: "logical_pending", Origin: capability.Human, Target: capability.Target{Scope: "call", Reference: "logical_pending"}}, Provider: selection.Provider(), Model: selection.Model(), SessionID: draft.SessionID, Request: request, StartedAt: at.Add(time.Second)}
			if err := journal.StartEndpointInvocation(t.Context(), invocation); err != nil {
				t.Fatal(err)
			}
			if test.known {
				message := chat.NewAssistantMessage(chat.NewTextPart("known provider answer"))
				response := &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}}
				if test.reported {
					response.Metadata = &chat.ResponseMetadata{Model: "captured-model", Usage: &chat.Usage{InputTokens: test.input, OutputTokens: 0, CacheReadInputTokens: new(int64(0))}}
				}
				if err := journal.SettleEndpointInvocation(t.Context(), invocation, modelref.EndpointOutcome{Response: response, CostUSD: test.price, FinishedAt: at.Add(2 * time.Second)}); err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			db, err = sqlite.Open(t.Context(), path)
			if err != nil {
				t.Fatal(err)
			}
			journal = sqlite.NewModelInvocationStore(db)
			if err := journal.ReconcileEndpointInvocations(t.Context(), time.Now().UTC()); err != nil {
				t.Fatal(err)
			}
			// A later failure must roll back both logical settlement and the Run projection.
			if _, err := db.ExecContext(t.Context(), `CREATE TRIGGER refuse_recovery BEFORE UPDATE ON runs WHEN NEW.outcome='lost' BEGIN SELECT RAISE(ABORT,'refused recovery'); END`); err != nil {
				t.Fatal(err)
			}
			if _, err := endpointRecovery(t, db).Reconcile(t.Context()); err == nil {
				t.Fatal("recovery ignored rejected Run update")
			}
			unchanged, _, err := sqlite.NewRunStore(db).Run(t.Context(), draft.RunID)
			if err != nil || !unchanged.Metrics().Equal(active.Metrics()) {
				t.Fatalf("failed recovery changed usage: %v", err)
			}
			calls, err := journal.PageModelInvocations(t.Context(), draft.RunID, 0, "", 10)
			if err != nil || len(calls) != 1 || calls[0].State != "started" {
				t.Fatalf("failed recovery settled logical evidence: %+v %v", calls, err)
			}
			if _, err := db.ExecContext(t.Context(), `DROP TRIGGER refuse_recovery`); err != nil {
				t.Fatal(err)
			}
			if count, err := endpointRecovery(t, db).Reconcile(t.Context()); err != nil || count != 1 {
				t.Fatalf("known recovery failed: %d %v", count, err)
			}
			recovered, _, err := sqlite.NewRunStore(db).Run(t.Context(), draft.RunID)
			if err != nil {
				t.Fatal(err)
			}
			usage, ok := recovered.Metrics().Usage()
			wantInput := int64(5)
			wantSteps := 1
			if test.known {
				wantSteps++
				if test.reported {
					wantInput += test.input
				}
			}
			if !ok || usage.Total.InputTokens != wantInput || usage.Total.OutputTokens != 2 || recovered.Metrics().Steps() != wantSteps || recovered.State() != run.Failed {
				t.Fatalf("recovery consumption=%+v steps=%d", usage, recovered.Metrics().Steps())
			}
			if (usage.Total.CostUSD == nil) != (test.wantCost == nil) || (test.wantCost != nil && *usage.Total.CostUSD != *test.wantCost) {
				t.Fatalf("recovery repriced or guessed cost: %+v", usage.Total.CostUSD)
			}
			calls, err = journal.PageModelInvocations(t.Context(), draft.RunID, 0, "", 10)
			if err != nil {
				t.Fatal(err)
			}
			wantState := "unknown"
			if test.known {
				wantState = "failed"
			}
			if calls[0].State != wantState || (calls[0].KnownResponse != nil) != test.known || (calls[0].Usage != nil) != test.reported {
				t.Fatalf("recovery promoted or lost source evidence: %+v", calls)
			}
			if count, err := endpointRecovery(t, db).Reconcile(t.Context()); err != nil || count != 0 {
				t.Fatalf("settled recovery repeated: %d %v", count, err)
			}
			again, _, err := sqlite.NewRunStore(db).Run(t.Context(), draft.RunID)
			if err != nil || !again.Metrics().Equal(recovered.Metrics()) {
				t.Fatal("restart charged the same source twice")
			}
		})
	}
}
