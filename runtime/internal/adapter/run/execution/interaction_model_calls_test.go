package execution

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tangerg/flame/packages/go-sdk/extension"
	"github.com/Tangerg/flame/runtime/internal/adapter/executionctx"
	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	"github.com/Tangerg/flame/runtime/internal/domain/modelref"
	"github.com/Tangerg/flame/runtime/internal/domain/run"
	"github.com/Tangerg/flame/runtime/internal/domain/session"
	"github.com/Tangerg/flame/runtime/internal/testsupport"
	"github.com/Tangerg/flame/runtime/internal/testsupport/persistencefixture"
	"github.com/Tangerg/scope/core/chat"
)

func TestIndependentModelPublisherUsesOriginalSourceAndCapturedMemberAfterCancellation(t *testing.T) {
	for _, known := range []bool{true, false} {
		t.Run(map[bool]string{true: "known", false: "unreported"}[known], func(t *testing.T) {
			db, err := persistencefixture.Open(t, t.Context(), filepath.Dir(filepath.Join(t.TempDir(), "source.db")))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			at := time.Now().UTC().Add(-time.Second)
			if err := db.Sessions.Insert(t.Context(), testsupport.MustRestoreSession(session.Snapshot{ID: "ses_nested", CreatedAt: at, UpdatedAt: at})); err != nil {
				t.Fatal(err)
			}
			draft := testsupport.RunDraft(run.Draft{SessionID: "ses_nested", RunID: "run_nested", SegmentID: "seg_nested", CreatedAt: at})
			if err := db.Runs.Admit(t.Context(), draft); err != nil {
				t.Fatal(err)
			}
			journal := db.ModelInvocations
			active := &interactionSession{scope: runs.ExecutionScope{SessionID: draft.SessionID}, modelEvidence: journal, lifetime: newInteractionLifetime(t.Context())}
			detach, attached := active.state.attachObserver(t.Context())
			if !attached {
				t.Fatal("observation was not admitted")
			}
			defer detach()
			defer active.lifetime.stopExecution()
			defer active.lifetime.stopReconciling()
			defer active.lifetime.stopRelease()
			member := runs.ExecutorMember{MemberID: "actual-child", ParentID: "actual-root", SpawnCallID: "original-delegate"}
			ctx, cancel := context.WithCancel(active.modelCallContext(t.Context(), member, "original-owner"))
			defer cancel()
			publisher := executionctx.ModelCallsFromContext(ctx)
			selection := testsupport.MustModelSelection("openai", "independent-model")
			started := make(chan func(context.Context, error) error, 1)
			failure := make(chan error, 1)
			go func() {
				finish, err := publisher.Start(ctx, "independent-call", selection)
				failure <- err
				started <- finish
			}()
			first := <-active.lifetime.events
			start := first.Payload.(runs.ExecutionFactCommit)
			fact := start.Fact().(runs.ModelCallStarted)
			if first.Member != member || fact.Purpose != run.ModelIndependent || fact.CallID != "independent-call" {
				t.Fatalf("start = %+v %+v", first.Member, fact)
			}
			commitErr := journal.StartModelInvocation(t.Context(), draft.SessionID, draft.RunID, draft.SegmentID, fact.CallID, time.Now().UTC(), fact.Purpose)
			start.Complete(commitErr)
			if err := <-failure; err != nil {
				t.Fatal(err)
			}
			finish := <-started
			request, _ := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("original independent input")))
			source := modelref.EndpointInvocation{ID: "endpoint_independent", Kind: modelref.EndpointChat, Caller: extension.Invocation{ID: fact.CallID, Origin: extension.Model, Parent: "original-owner", Target: extension.Target{Scope: "call", Reference: fact.CallID}}, Provider: selection.Provider(), Model: selection.Model(), SessionID: draft.SessionID, Request: request, StartedAt: time.Now().UTC()}
			if err := journal.StartEndpointInvocation(t.Context(), source, fact.CallID); err != nil {
				t.Fatal(err)
			}
			if known {
				message := chat.NewAssistantMessage(chat.NewTextPart("auxiliary output"))
				response := &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, Metadata: &chat.ResponseMetadata{Model: "served-independent", Usage: &chat.Usage{InputTokens: 9007199254740993, OutputTokens: 2}}}
				if err := journal.SettleEndpointInvocation(t.Context(), source, modelref.EndpointOutcome{Response: response, CostUSD: new(.25), FinishedAt: time.Now().UTC()}); err != nil {
					t.Fatal(err)
				}
			}
			cancel()
			done := make(chan error, 1)
			go func() { done <- finish(ctx, context.Canceled) }()
			last := <-active.lifetime.events
			completion := last.Payload.(runs.ExecutionFactCommit)
			failed := completion.Fact().(runs.ModelCallFailed)
			if last.Member != member || failed.Accounting == nil || failed.Accounting.ContextTokens != 0 || failed.Accounting.Message != nil || failed.Observation != (runs.ModelObservation{}) {
				t.Fatalf("independent completion = %+v %+v", last.Member, failed)
			}
			if known {
				price, priced := failed.Accounting.Cost.USD()
				if failed.KnownResponse == nil || failed.Accounting.Model != "served-independent" || failed.Accounting.ReportedUsage.InputTokens != 9007199254740993 || !priced || price != .25 {
					t.Fatalf("original evidence = %+v", failed)
				}
			} else if failed.KnownResponse != nil || failed.Accounting.ReportedUsage != nil || failed.Accounting.Cost.OptionalUSD() != nil {
				t.Fatal("unknown admission fabricated usage or price")
			}
			completion.Complete(nil)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
		})
	}
}
