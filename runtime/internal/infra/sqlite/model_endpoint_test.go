package sqlite_test

import (
	"path/filepath"
	"testing"
	"time"

	"errors"
	"github.com/Tangerg/flame/runtime/capability"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/infra/sqlite"
	"github.com/Tangerg/scope/core/chat"
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
