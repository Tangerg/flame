package sqlite_test

import (
	"path/filepath"
	"testing"
	"time"

	"errors"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/scope/core/chat"
	"reflect"
)

func TestUnobservedEndpointReopensUnknownWithoutReplayingCallingIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flame.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("captured input")))
	invocation := modelref.EndpointInvocation{ID: "endpoint-original", Caller: capability.Invocation{ID: "model-operation", Origin: capability.Human, Parent: "human-command", Target: capability.Target{Scope: "call", Reference: "model-operation"}}, Provider: "anthropic", Model: "captured-model", Request: request, StartedAt: time.Now().UTC()}
	if err := sqlite.NewModelInvocationStore(db).StartEndpointInvocation(t.Context(), invocation); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := sqlite.NewModelInvocationStore(db)
	if err := store.ReconcileEndpointInvocations(t.Context(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	rows, err := store.PageEndpointInvocations(t.Context(), "model-operation", 0, "", 10)
	if err != nil || len(rows) != 1 || rows[0].State != "unknown" || rows[0].Response != nil || rows[0].Failure == "" {
		t.Fatalf("restart promoted unknown request: %+v, %v", rows, err)
	}
	invocation.ID = "endpoint-replacement"
	if err := store.StartEndpointInvocation(t.Context(), invocation); err == nil {
		t.Fatal("same calling operation started a replacement endpoint")
	}
}

func TestSessionEndpointDeletionIsAtomicAndRetainsUnrelatedEvidence(t *testing.T) {
	db, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "endpoints.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	insertTrajectorySession(t, t.Context(), db, "ses_endpoint")
	insertTrajectorySession(t, t.Context(), db, "ses_other")
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("original request")))
	store := sqlite.NewModelInvocationStore(db)
	invocation := modelref.EndpointInvocation{ID: "endpoint_session", Caller: capability.Invocation{ID: "utility_session", Origin: capability.Automatic, Target: capability.Target{Scope: "call", Reference: "utility_session"}}, Provider: "anthropic", Model: "captured-model", SessionID: "ses_endpoint", Request: request, StartedAt: time.Unix(100, 0).UTC()}
	for _, scope := range []struct{ id, session string }{{"session", "ses_endpoint"}, {"other", "ses_other"}, {"global", ""}} {
		value := invocation
		value.ID = "endpoint_" + scope.id
		value.Caller.ID = "utility_" + scope.id
		value.Caller.Target.Reference = value.Caller.ID
		value.SessionID = scope.session
		if err := store.StartEndpointInvocation(t.Context(), value); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(t.Context(), `CREATE TRIGGER refuse_session_delete BEFORE DELETE ON sessions BEGIN SELECT RAISE(ABORT,'refused'); END`); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.NewSessionStore(db).Delete(t.Context(), invocation.SessionID); err == nil {
		t.Fatal("failed Session deletion succeeded")
	}
	if rows, err := store.ListSessionEndpoints(t.Context(), invocation.SessionID); err != nil || len(rows) != 1 {
		t.Fatalf("failed deletion lost evidence: %v %v", rows, err)
	}
	if _, err := db.ExecContext(t.Context(), `DROP TRIGGER refuse_session_delete`); err != nil {
		t.Fatal(err)
	}
	if err := sqlite.NewSessionStore(db).Delete(t.Context(), invocation.SessionID); err != nil {
		t.Fatal(err)
	}
	if rows, err := store.PageEndpointInvocations(t.Context(), invocation.Caller.ID, 0, "", 10); err != nil || len(rows) != 0 {
		t.Fatalf("deleted Session retained endpoints: %v %v", rows, err)
	}
	for _, caller := range []string{"utility_other", "utility_global"} {
		if _, found, err := store.EndpointForCaller(t.Context(), caller); err != nil || !found {
			t.Fatalf("unrelated evidence lost: %s %v", caller, err)
		}
	}
	invocation.ID = "endpoint_after_delete"
	invocation.Caller.ID = "utility_after_delete"
	invocation.Caller.Target.Reference = invocation.Caller.ID
	if err := store.StartEndpointInvocation(t.Context(), invocation); err == nil {
		t.Fatal("new endpoint attached to deleted Session")
	}
	if _, err := sqlite.NewTrajectoryStore(db).Page(t.Context(), invocation.SessionID, true, nil, 100); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("deleted Session trajectory: %v", err)
	}
}

func TestEndpointReadRejectsCompetingUsageProjection(t *testing.T) {
	db, err := sqlite.Open(t.Context(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("original")))
	invocation := modelref.EndpointInvocation{ID: "endpoint_usage", Caller: capability.Invocation{ID: "caller_usage", Origin: capability.Human, Target: capability.Target{Scope: "call", Reference: "caller_usage"}}, Provider: "anthropic", Model: "captured-model", Request: request, StartedAt: time.Now().UTC()}
	store := sqlite.NewModelInvocationStore(db)
	if err := store.StartEndpointInvocation(t.Context(), invocation); err != nil {
		t.Fatal(err)
	}
	message := chat.NewAssistantMessage(chat.NewTextPart("known"))
	response := &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, Metadata: &chat.ResponseMetadata{Usage: &chat.Usage{InputTokens: 12, OutputTokens: 3}}}
	if err := store.SettleEndpointInvocation(t.Context(), invocation, modelref.EndpointOutcome{Response: response, FinishedAt: invocation.StartedAt.Add(time.Second), CostUSD: new(0.0)}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE model_invocations SET usage=json_set(usage,'$.inputTokens',13) WHERE call_id=?`, invocation.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.EndpointForCaller(t.Context(), invocation.Caller.ID); err == nil {
		t.Fatal("usage projection competed with original response")
	}
}

func TestPhysicalUsageReadsAllScopesOnceAndSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.db")
	db, err := sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	insertTrajectorySession(t, t.Context(), db, "ses_usage")
	insertTrajectorySession(t, t.Context(), db, "ses_other")
	draft := runDraft("run_usage", "ses_usage")
	if err := sqlite.NewRunStore(db).Admit(t.Context(), draft); err != nil {
		t.Fatal(err)
	}
	store := sqlite.NewModelInvocationStore(db)
	if err := store.StartModelInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "logical_usage", draft.CreatedAt); err != nil {
		t.Fatal(err)
	}
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("original")))
	for _, source := range []struct {
		id, caller, session string
		input               int64
		price               *float64
		unknown, failed     bool
	}{
		{id: "primary", caller: "logical_usage", input: 12, price: new(1.25)},
		{id: "auxiliary", caller: "utility_usage", session: "ses_usage", input: 4, price: new(0.0), failed: true},
		{id: "unattached", caller: "utility_global", unknown: true},
		{id: "other", caller: "utility_other", session: "ses_other", input: 2},
	} {
		invocation := modelref.EndpointInvocation{ID: "endpoint_" + source.id, Caller: capability.Invocation{ID: source.caller, Origin: capability.Automatic, Target: capability.Target{Scope: "call", Reference: source.caller}}, Provider: draft.ModelSelection.Provider(), Model: draft.ModelSelection.Model(), SessionID: source.session, Request: request, StartedAt: draft.CreatedAt}
		if err := store.StartEndpointInvocation(t.Context(), invocation); err != nil {
			t.Fatal(err)
		}
		outcome := modelref.EndpointOutcome{FinishedAt: draft.CreatedAt.Add(time.Second), CostUSD: source.price}
		if source.unknown {
			outcome.Failure = "unobserved result"
		} else {
			message := chat.NewAssistantMessage(chat.NewTextPart("known"))
			outcome.Response = &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, Metadata: &chat.ResponseMetadata{Model: "served-model", Usage: &chat.Usage{InputTokens: source.input, OutputTokens: 1}}}
			if source.failed {
				outcome.Failure = "outer failure"
			}
		}
		if err := store.SettleEndpointInvocation(t.Context(), invocation, outcome); err != nil {
			t.Fatal(err)
		}
	}
	reporter, err := sessions.NewUsageReporter(sessions.UsageDependencies{Source: store})
	if err != nil {
		t.Fatal(err)
	}
	before, err := reporter.Summary(t.Context(), sessions.AllTimeUsage())
	if err != nil {
		t.Fatal(err)
	}
	if before.Coverage.Calls != 4 || before.Coverage.ReportedCalls != 3 || before.Coverage.PricedCalls != 2 || before.Coverage.UnknownCalls != 1 || before.Total.InputTokens != 18 || before.Total.OutputTokens != 3 || before.Total.CostUSD != nil || before.RecordedCostUSD == nil || *before.RecordedCostUSD != 1.25 || before.Sessions != 2 || before.Runs != 1 {
		t.Fatalf("logical/physical or unattached usage changed: %+v", before)
	}
	own, err := reporter.Session(t.Context(), "ses_usage")
	if err != nil || own.Coverage.Calls != 2 || own.Total.InputTokens != 16 || own.Total.CostUSD == nil || *own.Total.CostUSD != 1.25 {
		t.Fatalf("auxiliary missing from Session report: %+v %v", own, err)
	}
	if _, err := reporter.Session(t.Context(), "ses_absent"); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("missing Session report: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = sqlite.Open(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	reporter, err = sessions.NewUsageReporter(sessions.UsageDependencies{Source: sqlite.NewModelInvocationStore(db)})
	if err != nil {
		t.Fatal(err)
	}
	after, err := reporter.Summary(t.Context(), sessions.AllTimeUsage())
	if err != nil || !reflect.DeepEqual(after, before) {
		t.Fatalf("restart changed recorded consumption: %v", err)
	}
	if _, err := db.ExecContext(t.Context(), `UPDATE model_invocations SET session_id='ses_other' WHERE call_id='endpoint_primary'`); err != nil {
		t.Fatal(err)
	}
	if value, err := reporter.Summary(t.Context(), sessions.AllTimeUsage()); err == nil || value.Coverage.Calls != 0 {
		t.Fatal("foreign ownership produced partial usage")
	}
}
