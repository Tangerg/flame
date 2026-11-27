package runtimebinding

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Tangerg/flame/apps/cli/internal/adapter/workbenchdb"
	sessionworkflow "github.com/Tangerg/flame/apps/cli/internal/application/agent/session"
	"github.com/Tangerg/flame/apps/cli/internal/application/mutation"
	"github.com/Tangerg/flame/apps/cli/internal/application/workbench"
	"github.com/Tangerg/flame/apps/cli/internal/domain/authoring"
	"github.com/Tangerg/flame/apps/cli/internal/domain/conversation"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
)

type interruptedRollbackReply struct {
	*Connection
	disconnect context.CancelFunc
}

func (r interruptedRollbackReply) RollbackSession(ctx context.Context, request conversation.RollbackSession) (conversation.RollbackResult, error) {
	if _, err := r.Connection.RollbackSession(ctx, request); err != nil {
		return conversation.RollbackResult{}, err
	}
	r.disconnect()
	return conversation.RollbackResult{}, conversation.ErrDisconnected
}

func TestRollbackRecoveryUsesTheOriginalReceiptAfterMoreRuns(t *testing.T) {
	configureIntegrationRuntime(t)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Stream bool `json:"stream"`
		}
		if err := json.UnmarshalRead(r.Body, &request); err != nil {
			http.Error(w, "invalid model request", http.StatusBadRequest)
			return
		}
		if err := writeColdTailCompletion(w, request.Stream, "Recorded.", nil); err != nil {
			t.Error(err)
		}
	}))
	t.Cleanup(provider.Close)
	t.Setenv("FLAME_PROVIDER", "deepseek")
	t.Setenv("FLAME_MODEL", coldTailModel)
	t.Setenv("FLAME_BASEURL", provider.URL)
	workspace := t.TempDir()
	connection := openIntegrationRuntime(t, workspace)
	ctx := t.Context()
	created, err := connection.CreateSession(ctx, conversation.CreateSession{Workspace: workspace})
	if err != nil {
		t.Fatal(err)
	}
	title := "Rollback receipt"
	if _, err := connection.UpdateSession(ctx, conversation.UpdateSession{SessionID: created.ID, ExpectedRevision: created.Revision, Title: &title}); err != nil {
		t.Fatal(err)
	}
	startRun := func(prompt string) string {
		t.Helper()
		stream, err := connection.StartRun(ctx, testStartRequest(created.ID, authoring.Message{Text: prompt}))
		if err != nil {
			t.Fatal(err)
		}
		for _, err := range stream.Events {
			if err != nil {
				t.Fatal(err)
			}
		}
		return stream.RunID
	}
	kept := startRun("Keep this prompt.")
	const droppedInput = "Recover this prompt from the original receipt."
	dropped := startRun(droppedInput)
	before, err := connection.GetSession(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := sessionworkflow.PreviewRollback(before, conversation.RollbackTarget{
		SessionID: created.ID, ToRunID: kept, Scope: protocol.RestoreHistory,
	})
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	openStore := func() *workbench.Store {
		t.Helper()
		persistence, err := workbenchdb.Open(directory)
		if err != nil {
			t.Fatal(err)
		}
		store, err := workbench.Open(persistence, workbench.Config{})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := store.Close(); err != nil {
				t.Error(err)
			}
		})
		return store
	}
	policy, err := mutation.NewReplayPolicy(connection.Profile().IdempotencyLimits(), time.Now)
	if err != nil {
		t.Fatal(err)
	}
	store := openStore()
	call, disconnect := context.WithCancel(ctx)
	defer disconnect()
	backoff := mutation.AcknowledgementBackoff()
	interrupted := interruptedRollbackReply{Connection: connection, disconnect: disconnect}
	const newerDraft = droppedInput + " Include the later evidence."
	if err := store.SaveDraft(created.ID, authoring.Message{Text: newerDraft}); err != nil {
		t.Fatal(err)
	}
	_, err = sessionworkflow.Rollback(call, interrupted, store, preview, policy, backoff)
	if !errors.Is(err, context.Canceled) && !errors.Is(err, conversation.ErrDisconnected) {
		t.Fatalf("interrupted rollback = %v; want a lost reply", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	committed, err := connection.GetSession(ctx, created.ID)
	if err != nil || len(committed.Runs) != 1 || committed.Runs[0].ID != kept ||
		committed.Session.Revision != before.Session.Revision {
		t.Fatalf("committed rollback = %+v, %v; want original surviving Run and unchanged Session revision", committed.Runs, err)
	}
	later := startRun("This later prompt must survive recovery.")
	reopened := openStore()
	if err := sessionworkflow.RecoverRollbacks(ctx, connection, reopened, policy, backoff); err != nil {
		t.Fatal(err)
	}
	restored, err := reopened.ActivateSessionDraft(created.ID, authoring.Message{})
	wantDraft := droppedInput + "\n\n" + newerDraft
	if err != nil || restored.Rollback == nil || restored.Rollback.DroppedCount != 1 || !restored.Rollback.Merged || restored.Draft.Text != wantDraft {
		t.Fatalf("recovered rollback draft = %+v, %v; want the original input of Run %q before the newer draft", restored, err, dropped)
	}
	repeated, err := reopened.ActivateSessionDraft(created.ID, authoring.Message{})
	if err != nil || repeated.Rollback != nil || repeated.Draft.Text != wantDraft {
		t.Fatalf("draft after consuming rollback = %+v, %v; want the same draft without another receipt", repeated, err)
	}
	current, err := connection.GetSession(ctx, created.ID)
	if err != nil || len(current.Runs) != 2 || current.Runs[0].ID != kept || current.Runs[1].ID != later {
		t.Fatalf("Runs after recovery = %+v, %v; want the surviving and later Runs", current.Runs, err)
	}
}
