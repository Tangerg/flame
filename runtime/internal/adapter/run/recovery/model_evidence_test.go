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
	"github.com/Tangerg/flame/runtime/internal/domain/automation/goal"
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
	store, err := New(Config{Sessions: sqlite.NewSessionStore(db), Runs: sqlite.NewRunStore(db), Interrupts: persistence.NewInterruptStore(sqlite.NewInterruptStore(db)), Transcript: sqlite.NewTranscriptStore(db), Messages: mustConversationStore(t, sqlite.NewMessageStore(db)), GoalRuns: mustGoalRuns(t, sqlite.NewGoalStore(db)), ExecutorCheckpoints: persistence.NewExecutorCheckpointStore(sqlite.NewExecutorCheckpointStore(db)), ModelInvocations: sqlite.NewModelInvocationStore(db), ToolInvocations: sqlite.NewToolInvocationStore(db), ChildRunStarts: sqlite.NewChildRunStartReservationStore(db), Tx: func(ctx context.Context, fn func(context.Context) error) error { return sqlite.RunInTx(ctx, db, fn) }})
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
		independent     bool
		input           int64
		price           *float64
		wantCost        *float64
	}{
		{name: "priced", known: true, reported: true, input: 12, price: new(1.25), wantCost: new(1.75)},
		{name: "unpriced", known: true, reported: true, input: 12},
		{name: "reported zero", known: true, reported: true, input: 0, price: new(0.0), wantCost: new(0.5)},
		{name: "unreported", known: true},
		{name: "unknown", wantCost: new(0.5)},
		{name: "independent route", independent: true, known: true, reported: true, input: 999, price: new(.25), wantCost: new(.75)},
		{name: "independent unreported", independent: true, known: true},
		{name: "independent unknown", independent: true},
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
			purpose := run.ModelConversation
			if test.independent {
				purpose = run.ModelIndependent
			}
			if err := journal.StartModelInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "logical_pending", at, purpose); err != nil {
				t.Fatal(err)
			}
			request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("captured request")))
			invocation := modelref.EndpointInvocation{ID: "endpoint_pending", Kind: modelref.EndpointChat, Caller: capability.Invocation{ID: "logical_pending", Origin: capability.Human, Target: capability.Target{Scope: "call", Reference: "logical_pending"}}, Provider: selection.Provider(), Model: selection.Model(), SessionID: draft.SessionID, Request: request, StartedAt: at.Add(time.Second)}
			if test.independent {
				invocation.Provider = "openai"
				invocation.Model = "independent-model"
			}
			if err := journal.StartEndpointInvocation(t.Context(), invocation, "logical_pending"); err != nil {
				t.Fatal(err)
			}
			if test.known {
				message := chat.NewAssistantMessage(chat.NewTextPart("known provider answer"))
				response := &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}}
				if test.reported {
					response.Metadata = &chat.ResponseMetadata{Model: invocation.Model, Usage: &chat.Usage{InputTokens: test.input, OutputTokens: 0, CacheReadInputTokens: new(int64(0))}}
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
			if test.known || test.independent {
				wantSteps++
				if test.reported {
					wantInput += test.input
				}
			}
			if !ok || usage.Total.InputTokens != wantInput || usage.Total.OutputTokens != 2 || recovered.Metrics().Steps() != wantSteps || recovered.State() != run.Failed {
				t.Fatalf("recovery consumption=%+v steps=%d", usage, recovered.Metrics().Steps())
			}
			if test.independent && recovered.ContextTokens() != 5 {
				t.Fatalf("independent recovery recalibrated primary context: %d", recovered.ContextTokens())
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

func TestRecoveryAccruesIndependentKnownSourcesOnceWithoutChangingContext(t *testing.T) {
	path := filepath.Join(t.TempDir(), "independent-recovery.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC().Add(-time.Minute)
	if err := sqlite.NewSessionStore(db).Insert(t.Context(), testsupport.MustRestoreSession(session.Snapshot{ID: "ses_independent", CreatedAt: at, UpdatedAt: at})); err != nil {
		t.Fatal(err)
	}
	selection := testsupport.MustModelSelection("anthropic", "captured-model")

	goalValue, err := goal.New("ses_independent", "recover original consumption", selection, run.Capabilities{}, "lease_independent", at)
	if err != nil {
		t.Fatal(err)
	}
	unwritten, err := goal.Unwritten("ses_independent")
	if err != nil {
		t.Fatal(err)
	}
	goalReplacement, err := goal.NewReplacement(unwritten.Version(), goalValue)
	if err != nil {
		t.Fatal(err)
	}
	goalStore := sqlite.NewGoalStore(db)
	if applied, err := goalStore.Save(t.Context(), goalReplacement); err != nil || !applied {
		t.Fatalf("seed Goal: %v", err)
	}
	draft := run.Draft{RunID: "run_independent", SessionID: "ses_independent", SegmentID: "seg_independent", ModelSelection: selection, GoalIncarnationID: goalValue.IncarnationID(), CreatedAt: at}
	store := sqlite.NewRunStore(db)
	if err := store.Admit(t.Context(), draft); err != nil {
		t.Fatal(err)
	}
	journal := sqlite.NewModelInvocationStore(db)
	if err := journal.StartModelInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "original_model", at, run.ModelConversation); err != nil {
		t.Fatal(err)
	}
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("original request")))
	message := chat.NewAssistantMessage(chat.NewTextPart("known result"))
	for _, test := range []struct {
		id      string
		input   int64
		price   *float64
		failure string
		known   bool
	}{
		{"original_model", 5, new(0.5), "", true},
		{"independent_one", 9007199254740993, new(1.25), "outer failed", true},
		{"independent_two", 7, new(0.25), "", true},
		{"independent_unknown", 0, nil, "unconfirmed", false},
	} {
		inv := modelref.EndpointInvocation{ID: "endpoint_" + test.id, Kind: modelref.EndpointChat, Caller: capability.Invocation{ID: test.id, Origin: capability.Model, Parent: "original_operation", Target: capability.Target{Scope: "call", Reference: test.id}}, Provider: "anthropic", Model: "captured-model", SessionID: draft.SessionID, Request: request, StartedAt: at.Add(time.Second)}
		if err := journal.StartEndpointInvocation(t.Context(), inv, "original_model"); err != nil {
			t.Fatal(err)
		}
		outcome := modelref.EndpointOutcome{FinishedAt: at.Add(2 * time.Second), Failure: test.failure, CostUSD: test.price}
		if test.known {
			outcome.Response = &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, Metadata: &chat.ResponseMetadata{Model: "served-" + test.id, Usage: &chat.Usage{InputTokens: test.input, OutputTokens: 2}}}
		}
		if err := journal.SettleEndpointInvocation(t.Context(), inv, outcome); err != nil {
			t.Fatal(err)
		}
	}
	active, _, err := store.Run(t.Context(), draft.RunID)
	if err != nil {
		t.Fatal(err)
	}
	metrics, err := active.Metrics().AccrueModelCall("served-original_model", &chat.Usage{InputTokens: 5, OutputTokens: 2}, new(0.5))
	if err != nil {
		t.Fatal(err)
	}
	active, err = active.AdvanceProgress(metrics, 5, at.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateProgress(t.Context(), active); err != nil {
		t.Fatal(err)
	}
	if err := journal.CompleteModelInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "original_model", at, at.Add(3*time.Second), nil, &chat.Usage{InputTokens: 5, OutputTokens: 2}); err != nil {
		t.Fatal(err)
	}
	sources, err := journal.ListIndependentEndpoints(t.Context())
	if err != nil || len(sources) != 3 {
		t.Fatalf("independent source selection: %+v %v", sources, err)
	}

	if err := journal.ValidateIndependentEndpoint(t.Context(), sources[0].Clone()); err != nil {
		t.Fatal(err)
	}
	changed := sources[0].Clone()
	changed.Response.Metadata.Usage.InputTokens++
	if err := journal.ValidateIndependentEndpoint(t.Context(), changed); err == nil {
		t.Fatal("changed typed usage bypassed original JSON evidence")
	}
	changed = sources[0].Clone()
	changed.CostUSD = new(99.0)
	if err := journal.ValidateIndependentEndpoint(t.Context(), changed); err == nil {
		t.Fatal("changed price bypassed original evidence")
	}
	if _, err := db.ExecContext(t.Context(), `CREATE TRIGGER refuse_independent_recovery BEFORE UPDATE ON runs BEGIN SELECT RAISE(ABORT,'refused'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := endpointRecovery(t, db).Reconcile(t.Context()); err == nil {
		t.Fatal("failed Run transaction accepted consumption")
	}
	unchanged, _, err := store.Run(t.Context(), draft.RunID)
	if err != nil || !unchanged.Metrics().Equal(active.Metrics()) {
		t.Fatal("failed transaction partially billed independent sources")
	}
	if _, err := db.ExecContext(t.Context(), `DROP TRIGGER refuse_independent_recovery`); err != nil {
		t.Fatal(err)
	}
	if count, err := endpointRecovery(t, db).Reconcile(t.Context()); err != nil || count != 1 {
		t.Fatalf("independent recovery: %d %v", count, err)
	}
	recovered, _, err := store.Run(t.Context(), draft.RunID)
	if err != nil {
		t.Fatal(err)
	}
	usage, ok := recovered.Metrics().Usage()
	if !ok || usage.Total.InputTokens != 9007199254741005 || usage.Total.OutputTokens != 6 || usage.Total.CostUSD != nil || recovered.Metrics().Steps() != 4 || recovered.ContextTokens() != 5 {
		t.Fatalf("independent accrual: %+v steps=%d context=%d", usage, recovered.Metrics().Steps(), recovered.ContextTokens())
	}

	loadedGoal, err := goalStore.Get(t.Context(), draft.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	accounted, found := loadedGoal.Goal()
	if !found || accounted.Used().Runs != 1 || accounted.Used().Steps != 4 || accounted.Used().Cost.OptionalUSD() != nil {
		t.Fatalf("Goal lost original consumption/uncertainty: %+v", accounted.Used())
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if count, err := endpointRecovery(t, db).Reconcile(t.Context()); err != nil || count != 0 {
		t.Fatalf("restart billed a second time: %d %v", count, err)
	}
	next, _, err := sqlite.NewRunStore(db).Run(t.Context(), draft.RunID)
	if err != nil || !next.Metrics().Equal(recovered.Metrics()) {
		t.Fatal("restart changed original consumption")
	}
	row, found, err := sqlite.NewModelInvocationStore(db).EndpointForCaller(t.Context(), "independent_unknown")
	if err != nil || !found || row.Response != nil || row.CostUSD != nil {
		t.Fatalf("unknown invented a price/report: %+v %v", row, err)
	}
}
