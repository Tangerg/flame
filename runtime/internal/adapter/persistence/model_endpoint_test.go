package persistence

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/qimu/portable"
	flameruns "github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/application/agent/sessions"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	rundomain "github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/embedding"
)

func TestUnobservedEndpointReopensUnknownWithoutReplayingCallingIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "flame.db")
	db, err := openDatabase(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("captured input")))
	invocation := modelref.EndpointInvocation{ID: "endpoint-original", Kind: modelref.EndpointChat, Caller: portable.Invocation{ID: "model-operation", Origin: extension.Human, Parent: "human-command", Target: portable.Target{Scope: extension.ScopeCall, Reference: "model-operation"}}, Provider: "anthropic", Model: "captured-model", Request: request, StartedAt: time.Now().UTC()}
	if err := newModelInvocationStore(db).StartEndpointInvocation(t.Context(), invocation, ""); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = openDatabase(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store := newModelInvocationStore(db)
	if err := store.ReconcileEndpointInvocations(t.Context(), time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	rows, err := store.PageEndpointInvocations(t.Context(), "model-operation", 0, "", 10)
	if err != nil || len(rows) != 1 || rows[0].State != "unknown" || rows[0].Response != nil || rows[0].Failure == "" {
		t.Fatalf("restart promoted unknown request: %+v, %v", rows, err)
	}
	invocation.ID = "endpoint-replacement"
	if err := store.StartEndpointInvocation(t.Context(), invocation, ""); err == nil {
		t.Fatal("same calling operation started a replacement endpoint")
	}
}

func TestSessionEndpointDeletionIsAtomicAndRetainsUnrelatedEvidence(t *testing.T) {
	db, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "endpoints.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	insertTrajectorySession(t, t.Context(), db, "ses_endpoint")
	insertTrajectorySession(t, t.Context(), db, "ses_other")
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("original request")))
	store := newModelInvocationStore(db)
	invocation := modelref.EndpointInvocation{ID: "endpoint_session", Kind: modelref.EndpointChat, Caller: portable.Invocation{ID: "utility_session", Origin: extension.Automatic, Target: portable.Target{Scope: extension.ScopeCall, Reference: "utility_session"}}, Provider: "anthropic", Model: "captured-model", SessionID: "ses_endpoint", Request: request, StartedAt: time.Unix(100, 0).UTC()}
	for _, scope := range []struct{ id, session string }{{"session", "ses_endpoint"}, {"other", "ses_other"}, {"global", ""}} {
		value := invocation
		value.ID = "endpoint_" + scope.id
		value.Caller.ID = "utility_" + scope.id
		value.Caller.Target.Reference = value.Caller.ID
		value.SessionID = scope.session
		if err := store.StartEndpointInvocation(t.Context(), value, ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(t.Context(), `CREATE TRIGGER refuse_session_delete BEFORE DELETE ON sessions BEGIN SELECT RAISE(ABORT,'refused'); END`); err != nil {
		t.Fatal(err)
	}
	if err := newSessionStore(db).Delete(t.Context(), invocation.SessionID); err == nil {
		t.Fatal("failed Session deletion succeeded")
	}
	if rows, err := store.ListSessionEndpoints(t.Context(), invocation.SessionID); err != nil || len(rows) != 1 {
		t.Fatalf("failed deletion lost evidence: %v %v", rows, err)
	}
	if _, err := db.ExecContext(t.Context(), `DROP TRIGGER refuse_session_delete`); err != nil {
		t.Fatal(err)
	}
	if err := newSessionStore(db).Delete(t.Context(), invocation.SessionID); err != nil {
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
	if err := store.StartEndpointInvocation(t.Context(), invocation, ""); err == nil {
		t.Fatal("new endpoint attached to deleted Session")
	}
	if _, err := newTrajectoryStore(db).Page(t.Context(), invocation.SessionID, true, nil, 100); !errors.Is(err, session.ErrNotFound) {
		t.Fatalf("deleted Session trajectory: %v", err)
	}
}

func TestEndpointReadRejectsCompetingUsageProjection(t *testing.T) {
	db, err := openDatabase(t.Context(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("original")))
	invocation := modelref.EndpointInvocation{ID: "endpoint_usage", Kind: modelref.EndpointChat, Caller: portable.Invocation{ID: "caller_usage", Origin: extension.Human, Target: portable.Target{Scope: extension.ScopeCall, Reference: "caller_usage"}}, Provider: "anthropic", Model: "captured-model", Request: request, StartedAt: time.Now().UTC()}
	store := newModelInvocationStore(db)
	if err := store.StartEndpointInvocation(t.Context(), invocation, ""); err != nil {
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
	db, err := openDatabase(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	insertTrajectorySession(t, t.Context(), db, "ses_usage")
	insertTrajectorySession(t, t.Context(), db, "ses_other")
	draft := runDraft("run_usage", "ses_usage")
	if err := newRunStore(db).Admit(t.Context(), draft); err != nil {
		t.Fatal(err)
	}
	store := newModelInvocationStore(db)
	if err := store.StartModelInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "logical_usage", draft.CreatedAt, rundomain.ModelConversation); err != nil {
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
		invocation := modelref.EndpointInvocation{ID: "endpoint_" + source.id, Kind: modelref.EndpointChat, Caller: portable.Invocation{ID: source.caller, Origin: extension.Automatic, Target: portable.Target{Scope: extension.ScopeCall, Reference: source.caller}}, Provider: draft.ModelSelection.Provider(), Model: draft.ModelSelection.Model(), SessionID: source.session, Request: request, StartedAt: draft.CreatedAt}
		owner := ""
		if source.id == "primary" {
			owner = "logical_usage"
			invocation.SessionID = draft.SessionID
		}
		if err := store.StartEndpointInvocation(t.Context(), invocation, owner); err != nil {
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
	clock := func() time.Time { return draft.CreatedAt.Add(2 * time.Second) }
	reporter, err := sessions.NewUsageReporter(sessions.UsageDependencies{Source: store, Now: clock})
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
	db, err = openDatabase(t.Context(), path)
	if err != nil {
		t.Fatal(err)
	}
	reporter, err = sessions.NewUsageReporter(sessions.UsageDependencies{Source: newModelInvocationStore(db), Now: clock})
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

func TestEndpointAssociatesOnlyExplicitActiveModelOrToolOwner(t *testing.T) {
	db, err := openDatabase(t.Context(), filepath.Join(t.TempDir(), "scoped-endpoints.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	insertTrajectorySession(t, t.Context(), db, "ses_owner")
	insertTrajectorySession(t, t.Context(), db, "ses_foreign")
	draft := runDraft("run_owner", "ses_owner")
	runs := newRunStore(db)
	if err := runs.Admit(t.Context(), draft); err != nil {
		t.Fatal(err)
	}
	journal := newModelInvocationStore(db)
	if err := journal.StartModelInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "logical_owner", draft.CreatedAt, rundomain.ModelConversation); err != nil {
		t.Fatal(err)
	}
	if err := journal.StartModelInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "reserved_logical", draft.CreatedAt, rundomain.ModelConversation); err != nil {
		t.Fatal(err)
	}

	if err := newToolInvocationStore(db).StartToolInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, "tool_owner", "item_owner", draft.CreatedAt, nil); err != nil {
		t.Fatal(err)
	}
	request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("original independent request")))
	source := func(id, sessionID, parent string) modelref.EndpointInvocation {
		return modelref.EndpointInvocation{ID: "endpoint_" + id, Kind: modelref.EndpointChat, Caller: portable.Invocation{ID: id, Origin: extension.Model, Parent: parent, Target: portable.Target{Scope: extension.ScopeCall, Reference: id}}, Provider: "anthropic", Model: "captured-model", SessionID: sessionID, Request: request, StartedAt: draft.CreatedAt.Add(time.Second)}
	}
	for _, test := range []struct {
		id, owner, parent string
		associated        bool
	}{
		{"child_model", "logical_owner", "unrelated-wire-parent", true},
		{"child_tool", "tool_owner", "unrelated-wire-parent", true},
		{"logical_owner", "logical_owner", "unrelated-wire-parent", true},
		{"unowned_child", "", "logical_owner", false},
	} {
		invocation := source(test.id, draft.SessionID, test.parent)
		if err := journal.StartEndpointInvocation(t.Context(), invocation, test.owner); err != nil {
			t.Fatal(err)
		}
		row, found, err := journal.EndpointForCaller(t.Context(), test.id)
		if err != nil || !found {
			t.Fatalf("missing admitted source: %+v %v", row, err)
		}
		if test.associated {
			if row.RunID != draft.RunID || row.SegmentID != draft.SegmentID || row.SessionID != draft.SessionID {
				t.Fatalf("independent source lost owner: %+v", row)
			}
		} else if row.RunID != "" || row.SegmentID != "" {
			t.Fatalf("wire ancestry invented a Run: %+v", row)
		}
	}
	for _, test := range []struct{ id, owner, session string }{
		{"foreign", "logical_owner", "ses_foreign"},
		{"unknown", "missing_owner", draft.SessionID},
		{"missing_session", "logical_owner", ""},
		{"reserved_logical", "", draft.SessionID},
		{"tool_owner", "logical_owner", draft.SessionID},
	} {
		if err := journal.StartEndpointInvocation(t.Context(), source(test.id, test.session, "logical_owner"), test.owner); err == nil {
			t.Fatalf("unadmitted owner %s accepted", test.id)
		}
		if _, found, err := journal.EndpointForCaller(t.Context(), test.id); err != nil || found {
			t.Fatalf("refused owner left provider evidence: %v %v", found, err)
		}
	}
	early := source("before_owner", draft.SessionID, "logical_owner")
	early.StartedAt = draft.CreatedAt.Add(-time.Second)
	if err := journal.StartEndpointInvocation(t.Context(), early, "logical_owner"); err == nil {
		t.Fatal("physical source preceded its admitted owner")
	}
	active, found, err := runs.Run(t.Context(), draft.RunID)
	if err != nil || !found {
		t.Fatal(err)
	}
	waiting, err := active.Suspend(draft.CreatedAt.Add(2 * time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := runs.Suspend(t.Context(), waiting, draft.SegmentID, flameruns.CommitID{}); err != nil {
		t.Fatal(err)
	}
	if err := journal.StartEndpointInvocation(t.Context(), source("after_wait", draft.SessionID, "logical_owner"), "logical_owner"); err == nil {
		t.Fatal("waiting Run accepted new provider work")
	}
	if err := resumeParkedRun(t.Context(), runs, waiting, "seg_reopened_owner", draft.CreatedAt.Add(3*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := journal.StartEndpointInvocation(t.Context(), source("after_resume", draft.SessionID, "logical_owner"), "logical_owner"); err == nil {
		t.Fatal("old source retargeted to resumed Segment")
	}

}

func TestEmbeddingEndpointRecordsKindUsageServedModelAndRawEvidence(t *testing.T) {
	db, err := openDatabase(t.Context(), ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	insertTrajectorySession(t, t.Context(), db, "ses_embed")
	store := newModelInvocationStore(db)
	request, err := embedding.NewRequest([]string{"first text", "second text"})
	if err != nil {
		t.Fatal(err)
	}
	request.Options.Model = "embed-model"
	invocation := modelref.EndpointInvocation{ID: "endpoint_embed", Kind: modelref.EndpointEmbedding, Caller: portable.Invocation{ID: "memory_search", Origin: extension.Automatic, Target: portable.Target{Scope: extension.ScopeCall, Reference: "memory_search"}}, Provider: "openai", Model: "embed-model", SessionID: "ses_embed", Embedding: request, StartedAt: time.Unix(100, 0).UTC()}
	if err := store.StartEndpointInvocation(t.Context(), invocation, ""); err != nil {
		t.Fatal(err)
	}
	first, _ := embedding.NewOutput([]float64{0.1, 0.2}, nil)
	second, _ := embedding.NewOutput([]float64{0.3, 0.4}, nil)
	response, err := embedding.NewResponse([]*embedding.Output{first, second}, &embedding.ResponseMetadata{Model: "embed-model-served", Usage: &embedding.Usage{InputTokens: 11}, CreatedAt: time.Unix(101, 0).UTC()})
	if err != nil {
		t.Fatal(err)
	}
	cost := 0.0005
	if err := store.SettleEndpointInvocation(t.Context(), invocation, modelref.EndpointOutcome{EmbeddingResponse: response, CostUSD: &cost, FinishedAt: invocation.StartedAt.Add(time.Second)}); err != nil {
		t.Fatal(err)
	}
	// A chat response on an embedding admission must be refused, not stored.
	if err := store.SettleEndpointInvocation(t.Context(), invocation, modelref.EndpointOutcome{Response: &chat.Response{}, FinishedAt: invocation.StartedAt.Add(time.Second)}); err == nil {
		t.Fatal("embedding endpoint accepted a chat response")
	}
	observed, found, err := store.EndpointForCaller(t.Context(), "memory_search")
	if err != nil || !found {
		t.Fatalf("embedding endpoint missing: %v", err)
	}
	if observed.Invocation.Kind != modelref.EndpointEmbedding || observed.State != "completed" || observed.ServedModel != "embed-model-served" {
		t.Fatalf("embedding projection changed: %+v", observed)
	}
	if observed.Usage == nil || observed.Usage.InputTokens != 11 || observed.Usage.OutputTokens != 0 {
		t.Fatalf("embedding usage is not input-only: %+v", observed.Usage)
	}
	if observed.CostUSD == nil || *observed.CostUSD != cost {
		t.Fatalf("embedding cost lost: %+v", observed.CostUSD)
	}
	if observed.Response != nil {
		t.Fatalf("embedding must not synthesize a chat response: %+v", observed.Response)
	}
	if !observed.HasResponse() || !strings.Contains(observed.ResponseJSON.String(), "0.3") || !strings.Contains(observed.RequestJSON.String(), "second text") {
		t.Fatalf("embedding raw evidence lost: req=%s resp=%s", observed.RequestJSON.String(), observed.ResponseJSON.String())
	}
	var folded *modelref.EndpointObservation
	if err := store.VisitEndpointInvocations(t.Context(), "ses_embed", time.Time{}, func(o modelref.EndpointObservation) error {
		captured := o
		folded = &captured
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if folded == nil || folded.Invocation.Kind != modelref.EndpointEmbedding || folded.Usage == nil || folded.Usage.InputTokens != 11 || folded.ServedModel != "embed-model-served" {
		t.Fatalf("usage source did not fold embedding evidence: %+v", folded)
	}
}
